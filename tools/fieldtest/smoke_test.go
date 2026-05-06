//go:build smoke

// Stand-alone smoke check: spawn tcp-emitter on a random port, connect to it,
// and assert we receive at least a few wire-encoded P3 frames within a few
// seconds. Run via: go test -tags=smoke ./...
package fieldtest

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestEmitterSmoke(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./tcp-emitter",
		"--port", strconv.Itoa(port),
		"--interval-ms", "200",
		"--ponders", "1,2",
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	deadline := time.Now().Add(5 * time.Second)
	var conn net.Conn
	for time.Now().Before(deadline) {
		conn, err = net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n == 0 {
		t.Fatal("zero bytes")
	}
	if buf[0] != 0x8E {
		t.Errorf("expected SOR 0x8E at offset 0, got 0x%02X", buf[0])
	}
	sawEOR := false
	for i := 0; i < n; i++ {
		if buf[i] == 0x8F {
			sawEOR = true
			break
		}
	}
	if !sawEOR {
		t.Errorf("no EOR (0x8F) seen in %d bytes", n)
	}
	t.Logf("got %d bytes; head=%x", n, buf[:min(n, 32)])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
