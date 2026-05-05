// Command gateway is the AMB Lap Timer gateway.
//
// Scope (docs/roadmap.md §3 #1 / #3):
//   - TCP client to upstream AMB, with auto-reconnect (exponential + jitter)
//   - --record:  write received bytes to <file> and timestamps to <file>.timing.csv
//   - --mock:    run with an in-memory mock source (no network)
//   - --replay:  play back a captured .bin (+ .timing.csv) instead of the live upstream
//   - WebSocket fan-out over /ws (binary frames, byte pipe)
//   - SPA hosting at /, /assets/* (embedded via go:embed)
//   - /healthz, /admin (stub), /logs
//   - Graceful shutdown on Ctrl+C / SIGTERM
//
// Out of scope here (handled later):
//   - /admin real WebUI    -> #8
//   - WS text-frame status -> #28
//   - Replay fast/instant detailed semantics -> field-test β
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/config"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/httpsrv"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/hub"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/logging"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/recorder"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/source"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/source/mock"
	realsrc "github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/source/real"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/source/replay"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/upstream"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/webassets"
)

// version is overridden by -ldflags="-X main.version=..." at release build time.
var version = "dev"

type cliFlags struct {
	configPath  string
	upstream    string
	recordPath  string
	replayPath  string
	mockMode    bool
	listen      string
	showVersion bool
}

func main() {
	var fl cliFlags
	flag.StringVar(&fl.configPath, "config", "", "config.json path (default: <exeDir>/config.json)")
	flag.StringVar(&fl.upstream, "upstream", "", "override upstream host:port (e.g. 192.168.1.21:5403)")
	flag.StringVar(&fl.recordPath, "record", "", "record received bytes to <file> (and <file>.timing.csv)")
	flag.StringVar(&fl.replayPath, "replay", "", "play back a captured .bin (+ .timing.csv) instead of live upstream")
	flag.BoolVar(&fl.mockMode, "mock", false, "use built-in mock source (no upstream / no replay)")
	flag.StringVar(&fl.listen, "listen", "", "override config.listen (e.g. :8080)")
	flag.BoolVar(&fl.showVersion, "version", false, "print version and exit")
	flag.Parse()

	if fl.showVersion {
		fmt.Println(version)
		return
	}

	if err := run(fl); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// run is the long body of main(); kept separate so it can return an error
// rather than os.Exit. Each step is tagged with the corresponding section
// of docs/architecture.md for traceability.
func run(fl cliFlags) error {
	if err := validateSourceFlags(fl); err != nil {
		return err
	}

	baseDir, err := exeDir()
	if err != nil {
		return fmt.Errorf("locate exe dir: %w", err)
	}
	if fl.configPath == "" {
		fl.configPath = filepath.Join(baseDir, "config.json")
	}
	if err := ensureConfigFile(fl.configPath, baseDir); err != nil {
		fmt.Fprintf(os.Stderr, "config init warning: %v\n", err)
	}
	cfg, err := loadConfig(fl.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg = cfg.ResolvePaths(baseDir)

	if fl.upstream != "" {
		host, port, err := splitHostPort(fl.upstream)
		if err != nil {
			return fmt.Errorf("--upstream: %w", err)
		}
		cfg.Upstream.Host = host
		cfg.Upstream.Port = port
	}
	if fl.listen != "" {
		cfg.Listen = fl.listen
	}

	if err := os.MkdirAll(cfg.Logging.Dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir logs warning: %v\n", err)
	}
	if err := os.MkdirAll(cfg.Records.Dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir records warning: %v\n", err)
	}

	log, err := logging.New(logging.Options{
		Dir:        cfg.Logging.Dir,
		MaxSizeMB:  cfg.Logging.MaxSizeMB,
		MaxBackups: cfg.Logging.MaxBackups,
	})
	if err != nil {
		return fmt.Errorf("logger init: %w", err)
	}
	defer log.Close()

	upstreamAddr := net.JoinHostPort(cfg.Upstream.Host, strconv.Itoa(cfg.Upstream.Port))
	log.Info("gateway starting",
		zap.String("version", version),
		zap.String("baseDir", baseDir),
		zap.String("config", fl.configPath),
		zap.Bool("mock", fl.mockMode),
		zap.String("replay", fl.replayPath),
		zap.String("upstream", upstreamAddr),
		zap.String("record", fl.recordPath),
		zap.String("listen", cfg.Listen),
		zap.String("logs", cfg.Logging.Dir),
		zap.String("records", cfg.Records.Dir))

	// Hub for WS fan-out.
	h := hub.New(log.Logger,
		cfg.Server.MaxClients,
		cfg.Server.ClientBufferLen)
	defer h.Close()

	// HTTP server with embedded SPA.
	webFS, err := webassets.FS()
	if err != nil {
		return fmt.Errorf("webassets: %w", err)
	}
	httpServer := httpsrv.New(httpsrv.Config{
		Addr:    cfg.Listen,
		Version: version,
		WebFS:   webFS,
		LogPath: filepath.Join(cfg.Logging.Dir, "gateway.log"),
	}, h, log.Logger)

	// Source.
	src, initialState, err := openSource(fl, cfg, upstreamAddr, log.Logger)
	if err != nil {
		return err
	}
	defer src.Close()
	httpServer.SetUpstreamState(initialState)

	// Recorder, optional.
	var rec *recorder.Recorder
	if fl.recordPath != "" {
		recPath := fl.recordPath
		if !filepath.IsAbs(recPath) {
			recPath = filepath.Join(baseDir, recPath)
		}
		if err := os.MkdirAll(filepath.Dir(recPath), 0o755); err != nil {
			log.Warn("mkdir record parent failed", zap.Error(err))
		}
		rec, err = recorder.New(recPath, log.Logger)
		if err != nil {
			return fmt.Errorf("recorder init: %w", err)
		}
		defer rec.Close()
		log.Info("recording", zap.String("bin", recPath),
			zap.String("timing_csv", recPath+".timing.csv"))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Spin up the HTTP server.
	httpDone := make(chan error, 1)
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpDone <- err
			return
		}
		httpDone <- nil
	}()

	// Drive the upstream loop. On signal cancellation it returns nil and
	// we proceed to shutdown. On replay-EOF it also returns nil but ctx
	// is still alive — in that case we keep the HTTP server running so
	// already-connected clients stay up (Issue #52: "hub stays alive on
	// replay finish"). Final shutdown is driven by ctx cancellation.
	loopErr := readLoop(ctx, src, rec, h, httpServer, initialState, log.Logger)
	if loopErr == nil && ctx.Err() == nil {
		log.Info("source finished; HTTP server remains up — Ctrl+C to stop")
		<-ctx.Done()
	}

	// Now actually shut down.
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown error", zap.Error(err))
	}

	// Wait for the http goroutine to exit. Any non-Closed error is fatal.
	select {
	case herr := <-httpDone:
		if herr != nil && loopErr == nil {
			return herr
		}
	case <-time.After(2 * time.Second):
		log.Warn("http server did not exit within 2s")
	}

	return loopErr
}

// validateSourceFlags enforces the documented exclusivity rules.
//
//	--mock and --replay are mutually exclusive (both replace the upstream
//	source). --record is also exclusive with both, per the Issue #3
//	completion criteria — recording while mocking/replaying re-records
//	the same data and is more confusing than useful.
func validateSourceFlags(fl cliFlags) error {
	exclusive := []string{}
	if fl.mockMode {
		exclusive = append(exclusive, "--mock")
	}
	if fl.replayPath != "" {
		exclusive = append(exclusive, "--replay")
	}
	if fl.recordPath != "" {
		exclusive = append(exclusive, "--record")
	}
	if len(exclusive) > 1 {
		return fmt.Errorf("flags %v are mutually exclusive — pick exactly one", exclusive)
	}
	return nil
}

// openSource constructs the right source.Source based on the flags.
// Returns the source plus the initial UpstreamState to publish to /healthz.
func openSource(fl cliFlags, cfg config.Config, upstreamAddr string, log *zap.Logger) (source.Source, httpsrv.UpstreamState, error) {
	switch {
	case fl.mockMode:
		log.Info("source: mock (--mock)")
		return mock.New(), httpsrv.UpstreamMock, nil
	case fl.replayPath != "":
		log.Info("source: replay", zap.String("path", fl.replayPath),
			zap.String("speed", cfg.Replay.Speed))
		s, err := replay.New(fl.replayPath, cfg.Replay.Speed, log)
		if err != nil {
			return nil, "", fmt.Errorf("--replay: %w", err)
		}
		return s, httpsrv.UpstreamReplay, nil
	default:
		bo := upstream.NewBackoff(
			time.Duration(cfg.Upstream.Reconnect.InitialMs)*time.Millisecond,
			time.Duration(cfg.Upstream.Reconnect.MaxMs)*time.Millisecond,
			cfg.Upstream.Reconnect.JitterRatio,
		)
		log.Info("source: real (TCP)", zap.String("addr", upstreamAddr))
		return realsrc.New(upstreamAddr, bo, log), httpsrv.UpstreamConnecting, nil
	}
}

func readLoop(
	ctx context.Context,
	src source.Source,
	rec *recorder.Recorder,
	h *hub.Hub,
	srv *httpsrv.Server,
	initialState httpsrv.UpstreamState,
	log *zap.Logger,
) error {
	var firstChunkOnce sync.Once
	for {
		chunk, err := src.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Info("shutdown requested")
				return nil
			}
			if errors.Is(err, io.EOF) {
				log.Info("replay finished")
				srv.SetUpstreamState(httpsrv.UpstreamFinished)
				return nil
			}
			log.Warn("source read error", zap.Error(err))
			continue
		}
		// First successful read flips real upstream from "connecting"
		// to "connected". Mock / replay keep their own label.
		firstChunkOnce.Do(func() {
			if initialState == httpsrv.UpstreamConnecting {
				srv.SetUpstreamState(httpsrv.UpstreamConnected)
			}
		})
		if rec != nil {
			rec.Write(chunk)
		}
		h.Broadcast(chunk)
	}
}

// exeDir returns the directory containing the running executable.
func exeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return filepath.Dir(resolved), nil
}

// ensureConfigFile copies <baseDir>/config.example.json to path if path does
// not exist. If neither exists, no error is returned and the caller falls
// back to config.Defaults().
func ensureConfigFile(path, baseDir string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	example := filepath.Join(baseDir, "config.example.json")
	if _, err := os.Stat(example); err != nil {
		return nil
	}
	src, err := os.Open(example)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(path)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func loadConfig(path string) (config.Config, error) {
	cfg, err := config.LoadFile(path)
	if err != nil && os.IsNotExist(err) {
		return config.Defaults(), nil
	}
	return cfg, err
}

func splitHostPort(s string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return "", 0, fmt.Errorf("expected host:port, got %q: %w", s, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return host, port, nil
}
