// Package logging provides the gateway's structured logger.
//
// Decision (closes #34): zap + lumberjack.v2.
//
// Comparison summary:
//
//	+----------------------+-------------------------+--------------------------------+
//	| Library              | Strengths               | Weaknesses                     |
//	+----------------------+-------------------------+--------------------------------+
//	| uber-go/zap          | mature, high perf,      | larger transitive deps         |
//	|                      | Console + JSON encoders | (zap, multierr, atomic)        |
//	| rs/zerolog           | smaller, JSON-first     | less ergonomic for human-      |
//	|                      |                         | readable file output           |
//	| stdlib log + custom  | zero deps               | reinvent leveled logging,      |
//	|                      |                         | structured fields, rotation    |
//	+----------------------+-------------------------+--------------------------------+
//
// Why zap won:
//   - docs/architecture.md §7 wants both human-readable and structured (JSON) output.
//     zap's Console + JSON encoders cover that without writing a wrapper.
//   - Pairs with lumberjack.v2 (de facto rotation library; FAT32-safe atomic rename)
//     to satisfy docs/architecture.md §4.4.4 (FAT32 max_size_mb).
//   - When #3 introduces /healthz / /admin / /logs we will likely want structured
//     fields anyway; pre-paying that cost here avoids a later rewrite.
//
// Fail-soft (docs/architecture.md §4.4.3): when the file sink can't be opened, we
// log to stdout only and continue. Once running, write errors from lumberjack are
// surfaced by zap on its internal error sink (stderr) but never stop the process.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Options configures the logger. Dir, MaxSizeMB, MaxBackups come from
// LoggingConfig in docs/architecture.md §3.4 / §4.4.4.
type Options struct {
	// Dir is the absolute directory for the log file (already resolved by
	// config.ResolvePaths). If empty, no file sink is attached.
	Dir string
	// MaxSizeMB is the rotation threshold. 0 falls back to 5 MB.
	MaxSizeMB int
	// MaxBackups is how many rotated files to keep. 0 falls back to 5.
	MaxBackups int
	// Level filters log entries. Zero value means InfoLevel.
	Level zapcore.Level
}

// Logger wraps *zap.Logger and the file sink so the caller can Close on shutdown.
type Logger struct {
	*zap.Logger
	closers []io.Closer
}

// New constructs a Logger that writes to stdout (console-formatted) and, when
// Dir is non-empty, also to <Dir>/gateway.log (JSON, rotated).
//
// New itself does not return an error for file-sink failures — those are
// downgraded to a warning on stdout per the fail-soft policy. New only fails
// if the in-memory zap construction itself fails (which on supported platforms
// it does not).
func New(opts Options) (*Logger, error) {
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.MessageKey = "msg"
	encCfg.LevelKey = "level"
	encCfg.CallerKey = "caller"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
	encCfg.EncodeCaller = zapcore.ShortCallerEncoder

	// Console (human-readable) sink to stdout.
	consoleEncoder := zapcore.NewConsoleEncoder(encCfg)
	cores := []zapcore.Core{
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), opts.Level),
	}

	var closers []io.Closer

	// File (JSON, rotated) sink.
	if opts.Dir != "" {
		if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr,
				"[logging] mkdir %s failed: %v (continuing with stdout only)\n",
				opts.Dir, err)
		} else {
			lj := &lumberjack.Logger{
				Filename:   filepath.Join(opts.Dir, "gateway.log"),
				MaxSize:    nonzero(opts.MaxSizeMB, 5),
				MaxBackups: nonzero(opts.MaxBackups, 5),
				LocalTime:  true,
			}
			jsonEncoder := zapcore.NewJSONEncoder(encCfg)
			cores = append(cores, zapcore.NewCore(jsonEncoder, zapcore.AddSync(lj), opts.Level))
			closers = append(closers, lj)
		}
	}

	core := zapcore.NewTee(cores...)
	base := zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	return &Logger{Logger: base, closers: closers}, nil
}

// Close flushes buffered entries and closes the file sink (if any).
// It is safe to call multiple times.
func (l *Logger) Close() error {
	if l == nil || l.Logger == nil {
		return nil
	}
	// Sync errors on stdout/stderr can be expected on Windows; ignore.
	_ = l.Sync()
	for _, c := range l.closers {
		_ = c.Close()
	}
	return nil
}

func nonzero(v, dflt int) int {
	if v <= 0 {
		return dflt
	}
	return v
}
