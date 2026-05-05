package replay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

// writeFixture writes a .bin and a matching .timing.csv into dir and returns
// the bin path.
func writeFixture(t *testing.T, dir, name string, chunks [][]byte, offsetsMs []int64) string {
	t.Helper()
	if len(chunks) != len(offsetsMs) {
		t.Fatalf("len mismatch: %d chunks vs %d offsets", len(chunks), len(offsetsMs))
	}
	binPath := filepath.Join(dir, name+".bin")
	binF, err := os.Create(binPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chunks {
		_, _ = binF.Write(c)
	}
	binF.Close()

	csvPath := binPath + ".timing.csv"
	csvF, err := os.Create(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	csvF.WriteString("offset_ms,length_bytes\n")
	for i, c := range chunks {
		if _, err := csvF.WriteString(formatRow(offsetsMs[i], len(c))); err != nil {
			t.Fatal(err)
		}
	}
	csvF.Close()
	return binPath
}

func formatRow(off int64, ln int) string {
	return itoa(off) + "," + itoa(int64(ln)) + "\n"
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	negative := v < 0
	if negative {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestRead_RealtimeMode_HonorsTimingOffsets(t *testing.T) {
	dir := t.TempDir()
	bin := writeFixture(t, dir, "session", [][]byte{
		[]byte("alpha"),
		[]byte("beta"),
		[]byte("gamma"),
	}, []int64{0, 100, 250})

	src, err := New(bin, "realtime", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	// Inject a fake clock and capture sleep durations.
	now := time.Unix(1_700_000_000, 0)
	src.now = func() time.Time { return now }

	var slept []time.Duration
	src.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		now = now.Add(d)
		return nil
	}

	for i, want := range []string{"alpha", "beta", "gamma"} {
		got, err := src.Read(context.Background())
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if string(got) != want {
			t.Errorf("chunk %d: got %q want %q", i, got, want)
		}
	}
	if _, err := src.Read(context.Background()); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF after all rows, got %v", err)
	}

	// Expected sleeps: 0 → 0, 100 → 100ms, 250 → 150ms (250-100).
	want := []time.Duration{100 * time.Millisecond, 150 * time.Millisecond}
	if len(slept) != len(want) {
		t.Fatalf("sleep count: got %d want %d (%v)", len(slept), len(want), slept)
	}
	for i, w := range want {
		if slept[i] != w {
			t.Errorf("sleep %d: got %v want %v", i, slept[i], w)
		}
	}
}

func TestRead_FastMode_DoesNotSleep(t *testing.T) {
	dir := t.TempDir()
	bin := writeFixture(t, dir, "fast", [][]byte{
		[]byte("aa"), []byte("bb"),
	}, []int64{0, 1000})

	src, err := New(bin, "fast", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	src.sleep = func(_ context.Context, d time.Duration) error {
		t.Fatalf("fast mode should not sleep, got %v", d)
		return nil
	}

	for i, want := range []string{"aa", "bb"} {
		got, err := src.Read(context.Background())
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if string(got) != want {
			t.Errorf("chunk %d: got %q want %q", i, got, want)
		}
	}
}

func TestRead_NoTimingCSV_FallsBackToInstantWholeFile(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "bare.bin")
	want := []byte("entire-bin-as-one-chunk")
	if err := os.WriteFile(binPath, want, 0o644); err != nil {
		t.Fatal(err)
	}
	// Note: NO timing.csv next to it.

	src, err := New(binPath, "realtime", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if src.speed != SpeedInstant {
		t.Errorf("speed: got %q want instant (fallback)", src.speed)
	}

	got, err := src.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
	if _, err := src.Read(context.Background()); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF on second read, got %v", err)
	}
}

func TestRead_BinShorterThanTimingClaim_TruncatesAndFinishes(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "short.bin")
	if err := os.WriteFile(binPath, []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	csvPath := binPath + ".timing.csv"
	// timing.csv promises 5 bytes, bin has only 2.
	if err := os.WriteFile(csvPath, []byte("offset_ms,length_bytes\n0,5\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src, err := New(binPath, "fast", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	got, err := src.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "xx" {
		t.Errorf("got %q want xx", got)
	}
	if _, err := src.Read(context.Background()); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF after truncation, got %v", err)
	}
}

func TestRead_ContextCanceled_DuringSleep_ReturnsCtxErr(t *testing.T) {
	dir := t.TempDir()
	bin := writeFixture(t, dir, "long", [][]byte{[]byte("hi")}, []int64{60_000})

	src, err := New(bin, "realtime", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	// Pretend "now" is just after epoch so the 60s offset triggers a sleep.
	src.now = func() time.Time { return time.Unix(0, 0) }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = src.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestNew_MissingBin_ReturnsError(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "nope.bin"), "realtime", zap.NewNop())
	if err == nil {
		t.Fatal("expected error for missing bin")
	}
}

func TestRead_AfterClose_ReturnsEOF(t *testing.T) {
	dir := t.TempDir()
	bin := writeFixture(t, dir, "x", [][]byte{[]byte("a")}, []int64{0})
	src, err := New(bin, "instant", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	// Drain.
	if _, err := src.Read(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Read(context.Background()); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF, got %v", err)
	}
	src.Close()
	src.Close() // idempotent
}
