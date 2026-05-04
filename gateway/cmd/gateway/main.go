// Command gateway is the AMB Lap Timer gateway-recorder MVP (Issue #1).
//
// Scope (docs/roadmap.md §3 #1):
//   - TCP client to upstream AMB, with auto-reconnect (exponential + jitter)
//   - --record:  write received bytes to <file> and timestamps to <file>.timing.csv
//   - --mock:    run with an in-memory mock source (no network)
//   - Graceful shutdown on Ctrl+C / SIGTERM
//
// Out of scope here (handled in #3+):
//   - WebSocket fan-out, SPA hosting, /admin, /healthz, --replay
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/config"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/logging"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/recorder"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/source"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/source/mock"
	realsrc "github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/source/real"
	"github.com/hama-jp/AMB_RC_Lap_Timer/gateway/internal/upstream"
)

// version is overridden by -ldflags="-X main.version=..." at release build time.
var version = "dev"

type cliFlags struct {
	configPath  string
	upstream    string
	recordPath  string
	mockMode    bool
	showVersion bool
}

func main() {
	var fl cliFlags
	flag.StringVar(&fl.configPath, "config", "", "config.json path (default: <exeDir>/config.json)")
	flag.StringVar(&fl.upstream, "upstream", "", "override upstream host:port (e.g. 192.168.1.21:5403)")
	flag.StringVar(&fl.recordPath, "record", "", "record received bytes to <file> (and <file>.timing.csv)")
	flag.BoolVar(&fl.mockMode, "mock", false, "use built-in mock source (ignores --upstream)")
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

func run(fl cliFlags) error {
	baseDir, err := exeDir()
	if err != nil {
		return fmt.Errorf("locate exe dir: %w", err)
	}

	if fl.configPath == "" {
		fl.configPath = filepath.Join(baseDir, "config.json")
	}

	// docs/architecture.md §4.4.6: bootstrap config.json from the example
	// when missing. Failures are warnings, not fatal — the gateway falls
	// back to defaults if no file exists at all.
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

	// docs/architecture.md §4.4.6: ensure logs/records dirs exist (fail-soft).
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

	addr := net.JoinHostPort(cfg.Upstream.Host, strconv.Itoa(cfg.Upstream.Port))
	log.Info("gateway starting",
		zap.String("version", version),
		zap.String("baseDir", baseDir),
		zap.String("config", fl.configPath),
		zap.Bool("mock", fl.mockMode),
		zap.String("upstream", addr),
		zap.String("record", fl.recordPath),
		zap.String("logs", cfg.Logging.Dir),
		zap.String("records", cfg.Records.Dir))

	// Open the source.
	var src source.Source
	if fl.mockMode {
		src = mock.New()
		log.Info("source: mock (--mock)")
	} else {
		bo := upstream.NewBackoff(
			time.Duration(cfg.Upstream.Reconnect.InitialMs)*time.Millisecond,
			time.Duration(cfg.Upstream.Reconnect.MaxMs)*time.Millisecond,
			cfg.Upstream.Reconnect.JitterRatio,
		)
		src = realsrc.New(addr, bo, log.Logger)
		log.Info("source: real (TCP)", zap.String("addr", addr))
	}
	defer src.Close()

	// Open the recorder if --record was given.
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

	// Signal-driven shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return readLoop(ctx, src, rec, log.Logger)
}

func readLoop(ctx context.Context, src source.Source, rec *recorder.Recorder, log *zap.Logger) error {
	for {
		chunk, err := src.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Info("shutdown requested")
				return nil
			}
			if errors.Is(err, io.EOF) {
				log.Info("source EOF, exiting")
				return nil
			}
			log.Warn("source read error", zap.Error(err))
			// Sources are expected to recover internally; if Read returns a
			// fresh error here we still loop until ctx is cancelled.
			continue
		}
		if rec != nil {
			rec.Write(chunk)
		}
	}
}

// exeDir returns the directory containing the running executable.
// `go run` produces a temp executable; in that case the returned path is the
// temp dir, which is acceptable for development.
func exeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// On Windows symlinks are uncommon for installed apps; fall back.
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
		// No example to copy from; let the caller fall back to defaults.
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
		// No config file at all — use defaults.
		return config.Defaults(), nil
	}
	return cfg, err
}

// splitHostPort accepts "host:port" (IPv4 / hostname) and returns the parts.
// IPv6 with brackets ("[::1]:5403") is also handled by net.SplitHostPort.
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
