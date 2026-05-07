// /admin/api/config implementation: GET returns the current config snapshot,
// POST validates a candidate config, atomically rewrites config.json,
// applies the live-changeable subset to running components, and writes a
// row to logs/admin-audit.log. The handler is mounted behind
// requireAdminAuth in registerRoutes.
//
// Apply timing classification follows docs/architecture.md §3.5.5:
//
//	immediate / next reconnect / next start  → "applied"
//	bind / log file path                     → "requires_restart"
//
// The classifier is deliberately static (it does not depend on whether a
// hook actually mutated runtime state) so the response is predictable: a
// field counts as "applied" the moment it is on disk, even if no live
// component cared about it. Operators can read this as "your edit is
// durable; here are the fields that also need a restart to take effect."

package httpsrv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/config"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/logging"
)

// ApplyHooks gathers the optional live-apply callbacks main.go provides.
// All fields are nil-safe: a missing hook is treated as "config is on
// disk; restart picks it up" for the response classification.
type ApplyHooks struct {
	// Upstream is invoked when upstream.host or upstream.port changed.
	// Implementation should rotate the dial target and force a reconnect.
	Upstream func(host string, port int)
	// Reconnect is invoked when any upstream.reconnect.* field changed.
	Reconnect func(initialMs, maxMs int, jitterRatio float64)
	// HubLimits is invoked when server.max_clients or
	// server.client_buffer_len changed. New values affect subsequent
	// Add calls only.
	HubLimits func(maxClients, bufferLen int)
}

// configState is everything the admin handler needs at request time.
// Server keeps one *atomic.Pointer[configState]; main.go calls
// SetAdminConfigState once at startup. Hot-reading configState through an
// atomic pointer means the handler does not need to hold any lock during
// the JSON encode/decode work.
//
// Raw vs Resolved (Issue #101): the handler must round-trip the config
// without mutating relative paths into absolute ones. Raw is what came off
// disk (and what GET returns / POST writes back); Resolved is the same
// shape with `logging.dir` / `records.dir` etc. expanded against the EXE
// directory and is reserved for future hooks that need real filesystem
// targets at runtime. Today no hook reads paths so Resolved is unused at
// request time, but the field exists so the handler can grow into runtime
// log-rotation reconfiguration etc. without another signature change.
type configState struct {
	// Raw is the as-loaded config: relative paths stay relative. POST
	// writes Raw to disk and the SPA's GET reads it back, so a USB
	// distribution survives a /admin save without C:\... paths leaking
	// into config.json (docs/architecture.md §4.4 portable operation).
	Raw config.Config
	// Resolved has Raw.ResolvePaths(baseDir) applied. Reserved for live
	// path-aware hooks; today the handler does not consult it.
	Resolved config.Config
	// Path is the absolute path to config.json. POST writes to
	// Path + ".tmp.<unix-nano>" then renames.
	Path string
	// Hooks are the live-apply callbacks (any may be nil).
	Hooks ApplyHooks
}

// SetAdminConfigState wires the running config + apply hooks into the
// /admin/api/config handlers. Must be called once at startup; subsequent
// calls atomically swap the pointer (main.go does not need this, but tests
// re-use one Server across cases).
//
// raw is the as-loaded config (relative paths preserved); resolved is the
// same with ResolvePaths applied. The handler returns / persists raw and
// holds resolved for future hooks.
func (s *Server) SetAdminConfigState(raw, resolved config.Config, path string, hooks ApplyHooks) {
	s.adminConfig.Store(&configState{
		Raw:      raw,
		Resolved: resolved,
		Path:     path,
		Hooks:    hooks,
	})
}

// loadAdminConfigState returns the current snapshot or nil if main.go has
// not yet wired one. Handlers fail safely with 503 in that window.
func (s *Server) loadAdminConfigState() *configState {
	return s.adminConfig.Load()
}

func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleAdminConfigGet(w, r)
	case http.MethodPost:
		s.handleAdminConfigPost(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdminConfigGet(w http.ResponseWriter, _ *http.Request) {
	st := s.loadAdminConfigState()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "admin config not yet initialized",
		})
		return
	}
	writeJSON(w, http.StatusOK, st.Raw)
}

// adminConfigPostResp is the success response shape (200). Errors use the
// standard {"errors":[...]} list and never reach this struct.
type adminConfigPostResp struct {
	Applied          []string      `json:"applied"`
	RequiresRestart  []string      `json:"requires_restart"`
	Config           config.Config `json:"config"`
	ChangedFieldList []string      `json:"changed_fields"`
}

func (s *Server) handleAdminConfigPost(w http.ResponseWriter, r *http.Request) {
	st := s.loadAdminConfigState()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "admin config not yet initialized",
		})
		return
	}

	// Strict decode so the UI gets caught early on stale field names — far
	// better than silently dropping a field the operator intended to set.
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var newCfg config.Config
	if err := dec.Decode(&newCfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"errors": []config.ValidationError{
				{Path: "", Message: fmt.Sprintf("invalid JSON: %v", err)},
			},
		})
		return
	}
	if errs := config.Validate(newCfg); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": errs})
		return
	}

	// What actually changed? Atomic write happens regardless (so admin can
	// re-save the same values to bump mtime), but the "applied" /
	// "requires_restart" buckets list only the diff.
	//
	// Diff against Raw — the request body shares Raw's relative-path shape
	// because the SPA fetched it from a previous GET that returned Raw.
	changed := diffConfig(st.Raw, newCfg)

	if err := writeConfigAtomic(st.Path, newCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("write config: %v", err),
		})
		return
	}

	// Live applies — only for fields that have a hook AND actually changed.
	// Hooks read non-path fields (host, port, reconnect, hub limits) so
	// passing Raw is semantically identical to passing Resolved here.
	applyHooks(st.Hooks, st.Raw, newCfg, changed)

	// Swap the in-memory snapshot LAST so concurrent GETs never observe a
	// state where disk and memory disagree. Resolved is preserved as-is —
	// none of today's hooks rotate paths at runtime; if that lands later,
	// re-apply ResolvePaths here against the original baseDir.
	s.adminConfig.Store(&configState{
		Raw:      newCfg,
		Resolved: st.Resolved,
		Path:     st.Path,
		Hooks:    st.Hooks,
	})

	// Audit log: pass through the per-field old→new pairs the diff produced.
	auditFields := make(map[string]logging.ChangeValue, len(changed))
	for _, p := range changed {
		auditFields[p] = changeValueFor(p, st.Raw, newCfg)
	}
	s.auth.audit.LogChange(clientIP(r), auditFields)

	applied, restart := classifyChanged(changed)
	resp := adminConfigPostResp{
		Applied:          applied,
		RequiresRestart:  restart,
		Config:           newCfg,
		ChangedFieldList: changed,
	}
	writeJSON(w, http.StatusOK, resp)
}

// diffConfig returns the sorted list of dotted paths whose values differ
// between old and new. The path set must match what classifyChanged knows
// about; an unrecognized path is bucketed as "applied" (safe default —
// it's at least on disk).
func diffConfig(oldC, newC config.Config) []string {
	var out []string
	if oldC.Listen != newC.Listen {
		out = append(out, "listen")
	}
	if oldC.Upstream.Host != newC.Upstream.Host {
		out = append(out, "upstream.host")
	}
	if oldC.Upstream.Port != newC.Upstream.Port {
		out = append(out, "upstream.port")
	}
	if oldC.Upstream.Reconnect.InitialMs != newC.Upstream.Reconnect.InitialMs {
		out = append(out, "upstream.reconnect.initial_ms")
	}
	if oldC.Upstream.Reconnect.MaxMs != newC.Upstream.Reconnect.MaxMs {
		out = append(out, "upstream.reconnect.max_ms")
	}
	if oldC.Upstream.Reconnect.JitterRatio != newC.Upstream.Reconnect.JitterRatio {
		out = append(out, "upstream.reconnect.jitter_ratio")
	}
	if oldC.Logging.Dir != newC.Logging.Dir {
		out = append(out, "logging.dir")
	}
	if oldC.Logging.MaxSizeMB != newC.Logging.MaxSizeMB {
		out = append(out, "logging.max_size_mb")
	}
	if oldC.Logging.MaxBackups != newC.Logging.MaxBackups {
		out = append(out, "logging.max_backups")
	}
	if oldC.Records.Dir != newC.Records.Dir {
		out = append(out, "records.dir")
	}
	if oldC.Replay.Speed != newC.Replay.Speed {
		out = append(out, "replay.speed")
	}
	if oldC.Server.MaxClients != newC.Server.MaxClients {
		out = append(out, "server.max_clients")
	}
	if oldC.Server.ClientBufferLen != newC.Server.ClientBufferLen {
		out = append(out, "server.client_buffer_len")
	}
	sort.Strings(out)
	return out
}

// classifyChanged splits the diff into applied (any change taking effect
// without a restart, including "next reconnect" / "next start") and
// requires_restart.
func classifyChanged(changed []string) (applied, restart []string) {
	for _, p := range changed {
		switch p {
		case "listen",
			"logging.dir",
			"logging.max_size_mb",
			"logging.max_backups":
			restart = append(restart, p)
		default:
			applied = append(applied, p)
		}
	}
	return applied, restart
}

func applyHooks(h ApplyHooks, oldC, newC config.Config, changed []string) {
	idx := make(map[string]struct{}, len(changed))
	for _, p := range changed {
		idx[p] = struct{}{}
	}
	if _, hostChanged := idx["upstream.host"]; hostChanged || hasKey(idx, "upstream.port") {
		if h.Upstream != nil {
			h.Upstream(newC.Upstream.Host, newC.Upstream.Port)
		}
	}
	if hasKey(idx, "upstream.reconnect.initial_ms") ||
		hasKey(idx, "upstream.reconnect.max_ms") ||
		hasKey(idx, "upstream.reconnect.jitter_ratio") {
		if h.Reconnect != nil {
			h.Reconnect(
				newC.Upstream.Reconnect.InitialMs,
				newC.Upstream.Reconnect.MaxMs,
				newC.Upstream.Reconnect.JitterRatio,
			)
		}
	}
	if hasKey(idx, "server.max_clients") || hasKey(idx, "server.client_buffer_len") {
		if h.HubLimits != nil {
			h.HubLimits(newC.Server.MaxClients, newC.Server.ClientBufferLen)
		}
	}
	_ = oldC // reserved for hooks that need to know what they're replacing
}

func hasKey(m map[string]struct{}, k string) bool {
	_, ok := m[k]
	return ok
}

// writeConfigAtomic serializes cfg as pretty JSON and renames it into place
// (FAT32-safe rename per docs/architecture.md §4.4.4). The temp file is
// scoped to the same directory so rename never has to cross a filesystem
// boundary.
func writeConfigAtomic(path string, cfg config.Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// changeValueFor formats a single field's old→new pair for the audit log.
// Reflection here is unfortunate but contained — Config has 13 leaf fields,
// they're all small, and the alternative is a large case statement that
// duplicates every entry in diffConfig.
func changeValueFor(path string, oldC, newC config.Config) logging.ChangeValue {
	o := readPath(oldC, path)
	n := readPath(newC, path)
	return logging.ChangeValue{
		Old: fmt.Sprintf("%v", o),
		New: fmt.Sprintf("%v", n),
	}
}

func readPath(c config.Config, path string) any {
	switch path {
	case "listen":
		return c.Listen
	case "upstream.host":
		return c.Upstream.Host
	case "upstream.port":
		return c.Upstream.Port
	case "upstream.reconnect.initial_ms":
		return c.Upstream.Reconnect.InitialMs
	case "upstream.reconnect.max_ms":
		return c.Upstream.Reconnect.MaxMs
	case "upstream.reconnect.jitter_ratio":
		return c.Upstream.Reconnect.JitterRatio
	case "logging.dir":
		return c.Logging.Dir
	case "logging.max_size_mb":
		return c.Logging.MaxSizeMB
	case "logging.max_backups":
		return c.Logging.MaxBackups
	case "records.dir":
		return c.Records.Dir
	case "replay.speed":
		return c.Replay.Speed
	case "server.max_clients":
		return c.Server.MaxClients
	case "server.client_buffer_len":
		return c.Server.ClientBufferLen
	}
	return nil
}
