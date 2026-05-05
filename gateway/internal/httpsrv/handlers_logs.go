package httpsrv

import (
	"bytes"
	"net/http"
	"os"
	"strconv"
)

// handleLogs returns the last N lines of the gateway's log file as
// application/x-ndjson.
//
// LAN-only deployment: no auth. Disabled when LogPath is empty.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.LogPath == "" {
		http.Error(w, "logs not configured", http.StatusServiceUnavailable)
		return
	}

	n := s.cfg.MaxLogTailLines
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 5000 {
			n = parsed
		}
	}

	data, err := os.ReadFile(s.cfg.LogPath)
	if err != nil {
		// Don't leak the path; the surrounding `gateway.log` (lumberjack)
		// may not exist yet on a fresh start.
		http.Error(w, "log unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(tailLines(data, n))
}

// tailLines returns at most n trailing lines of data, preserving the
// trailing newline if the original had one.
func tailLines(data []byte, n int) []byte {
	if n <= 0 || len(data) == 0 {
		return nil
	}
	count := 0
	// Walk backwards counting '\n'. We allow either \n or no trailing \n.
	endsWithNewline := data[len(data)-1] == '\n'
	scanEnd := len(data)
	if endsWithNewline {
		scanEnd--
	}
	i := scanEnd
	for ; i > 0; i-- {
		if data[i-1] == '\n' {
			count++
			if count == n {
				break
			}
		}
	}
	if endsWithNewline {
		return append([]byte{}, data[i:]...)
	}
	tail := data[i:]
	out := make([]byte, 0, len(tail)+1)
	out = append(out, tail...)
	if !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	return out
}
