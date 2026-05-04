package recorder

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRecorder_BasicWrite(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "out.bin")
	rec, err := New(binPath, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec.Write([]byte("hello"))
	rec.Write([]byte("world"))
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	gotBin, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read bin: %v", err)
	}
	if !bytes.Equal(gotBin, []byte("helloworld")) {
		t.Errorf("bin: got %q want helloworld", gotBin)
	}

	gotCSV, err := os.ReadFile(binPath + ".timing.csv")
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	csv := string(gotCSV)
	if !strings.HasPrefix(csv, "offset_ms,length_bytes\n") {
		t.Errorf("missing header; got: %q", csv)
	}
	lines := strings.Split(strings.TrimRight(csv, "\n"), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), csv)
	}
	// Row 1: ",5"; row 2: ",5"
	if !strings.HasSuffix(lines[1], ",5") {
		t.Errorf("row 1 length wrong: %q", lines[1])
	}
	if !strings.HasSuffix(lines[2], ",5") {
		t.Errorf("row 2 length wrong: %q", lines[2])
	}
}

func TestRecorder_Close_Idempotent(t *testing.T) {
	dir := t.TempDir()
	rec, err := New(filepath.Join(dir, "x.bin"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestRecorder_New_OpenFailure_Errors(t *testing.T) {
	// Path with a non-existent intermediate directory must fail.
	_, err := New(filepath.Join(t.TempDir(), "no-such-dir", "x.bin"), zap.NewNop())
	if err == nil {
		t.Fatal("expected error for unwritable path")
	}
}

// failingWriter returns errFailingWriter on every Write.
type failingWriter struct{ closed bool }

var errFailingWriter = errors.New("simulated write failure")

func (w *failingWriter) Write(p []byte) (int, error) { return 0, errFailingWriter }
func (w *failingWriter) Close() error                { w.closed = true; return nil }

// growBuffer is a Buffer that also implements Close.
type growBuffer struct{ bytes.Buffer }

func (g *growBuffer) Close() error { return nil }

func TestRecorder_FailSoft_BinWriteFailure_NoPanic_CountsFailures(t *testing.T) {
	bin := &failingWriter{}
	timing := &growBuffer{}
	rec, err := newWithWriters(bin, timing, zap.NewNop(), func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatalf("newWithWriters: %v", err)
	}

	// Header should be present in the csv buffer.
	if !strings.Contains(timing.String(), "offset_ms,length_bytes") {
		t.Errorf("header missing; csv=%q", timing.String())
	}

	rec.Write([]byte("data1"))
	rec.Write([]byte("data2"))
	if got := rec.Failures(); got != 2 {
		t.Errorf("expected 2 failures, got %d", got)
	}
	// Recorder is still usable (Close should not panic, returns nil).
	if err := rec.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestRecorder_TimingOffset_Monotonic(t *testing.T) {
	bin := &growBuffer{}
	timing := &growBuffer{}
	tick := time.Unix(0, 0)
	now := func() time.Time {
		t := tick
		tick = tick.Add(100 * time.Millisecond)
		return t
	}
	rec, err := newWithWriters(bin, timing, zap.NewNop(), now)
	if err != nil {
		t.Fatal(err)
	}
	rec.Write([]byte("a"))
	rec.Write([]byte("bb"))
	rec.Write([]byte("ccc"))
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	csv := timing.String()
	// `now` is called once at New() (returns 0ms, advances to 100ms) and once per
	// Write (returning 100ms, 200ms, 300ms). offsets = call - started = 100/200/300.
	want := "offset_ms,length_bytes\n100,1\n200,2\n300,3\n"
	if csv != want {
		t.Errorf("csv mismatch:\n got %q\nwant %q", csv, want)
	}
}

// Compile-time check: Recorder.Write must accept io.Reader-like flow without
// the caller needing to think about errors (fail-soft signature).
var _ = func() {
	var r *Recorder
	var _ func([]byte) = r.Write
}
