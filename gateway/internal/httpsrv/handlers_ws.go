package httpsrv

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/hub"
)

// wsWriteTimeout is the per-frame write deadline. A WebSocket Write that
// exceeds this likely indicates a TCP-level stall; we drop the client.
const wsWriteTimeout = 10 * time.Second

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// nhooyr's default already rejects cross-origin Upgrade requests
		// when the Origin header doesn't match Host. LAN-only deployment
		// keeps this strict — there's no need for OriginPatterns.
		InsecureSkipVerify: false,
	})
	if err != nil {
		s.log.Warn("ws upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close(websocket.StatusInternalError, "internal error")

	client, err := s.hub.Add()
	if err != nil {
		switch {
		case errors.Is(err, hub.ErrTooManyClients):
			conn.Close(websocket.StatusTryAgainLater, "too many clients")
		case errors.Is(err, hub.ErrHubClosed):
			conn.Close(websocket.StatusGoingAway, "shutting down")
		default:
			conn.Close(websocket.StatusInternalError, "hub error")
		}
		return
	}
	defer s.hub.Remove(client)
	s.log.Info("ws client connected",
		zap.String("remote", r.RemoteAddr), zap.Int("clients", s.hub.Count()))

	// We don't process incoming WS messages yet (#28 is the proper place
	// for that). However the lib expects something to read on the conn so
	// pings / control frames are processed; this goroutine drains and
	// terminates when the client disconnects.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				cancel()
				return
			}
		}
	}()

	for {
		select {
		case msg, ok := <-client.Recv():
			if !ok {
				return
			}
			wctx, wcancel := context.WithTimeout(ctx, wsWriteTimeout)
			err := conn.Write(wctx, websocket.MessageBinary, msg)
			wcancel()
			if err != nil {
				s.log.Debug("ws write failed; closing client",
					zap.String("remote", r.RemoteAddr), zap.Error(err))
				return
			}
		case <-client.Done():
			return
		case <-ctx.Done():
			return
		}
	}
}
