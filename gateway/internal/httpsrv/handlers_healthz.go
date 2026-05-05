package httpsrv

import (
	"net/http"
	"time"
)

// healthzPayload is the documented JSON shape (docs/architecture.md §3.1).
type healthzPayload struct {
	Upstream  string `json:"upstream"`
	Clients   int    `json:"clients"`
	UptimeSec int64  `json:"uptime_sec"`
	Version   string `json:"version"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state, _ := s.upstream.Load().(UpstreamState)
	payload := healthzPayload{
		Upstream:  string(state),
		Clients:   s.hub.Count(),
		UptimeSec: int64(time.Since(s.started).Seconds()),
		Version:   s.cfg.Version,
	}
	writeJSON(w, http.StatusOK, payload)
}
