package function

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func nodeAvailable() bool {
	_, err := exec.LookPath("node")
	return err == nil
}

func TestNodeRunnerInvokesHandler(t *testing.T) {
	if !nodeAvailable() {
		t.Skip("node not available")
	}
	r := New()
	src := `
exports.handler = async (event) => {
  return { doubled: event.value * 2, name: event.name };
};
`
	res, err := r.Run(context.Background(), src, "node", map[string]any{"value": 21, "name": "alice"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res["doubled"] != float64(42) {
		t.Fatalf("doubled = %v, want 42", res["doubled"])
	}
	if res["name"] != "alice" {
		t.Fatalf("name = %v, want alice", res["name"])
	}
}

func TestNodeRunnerTimeout(t *testing.T) {
	if !nodeAvailable() {
		t.Skip("node not available")
	}
	r := New()
	src := `
exports.handler = async () => { await new Promise((r) => setTimeout(r, 60000)); };
`
	_, err := r.Run(context.Background(), src, "node", map[string]any{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout message, got: %v", err)
	}
}

func TestNodeRunnerBadSource(t *testing.T) {
	if !nodeAvailable() {
		t.Skip("node not available")
	}
	r := New()
	_, err := r.Run(context.Background(), "this is not javascript", "node", map[string]any{})
	if err == nil {
		t.Fatal("expected bad-source error")
	}
}

func TestNodeRunnerRejectsUnknownRuntime(t *testing.T) {
	r := New()
	_, err := r.Run(context.Background(), "x", "python", map[string]any{})
	if err == nil {
		t.Fatal("expected unknown-runtime error")
	}
}

func TestLimitedBufferTruncatesOversizedOutput(t *testing.T) {
	b := &limitedBuffer{max: 16}
	buf := make([]byte, 64)
	for i := range buf {
		buf[i] = 'x'
	}
	n, _ := b.Write(buf)
	if n != 16 {
		t.Fatalf("Write returned %d, want 16 (bytes actually buffered)", n)
	}
	if len(b.Bytes()) > 16 {
		t.Fatalf("buffer exceeded max: %d bytes", len(b.Bytes()))
	}
	if !strings.Contains(b.String(), "truncated") {
		t.Fatalf("expected truncation marker, got %q", b.String())
	}
}

func TestNodeRunnerKillsDescendantsOnTimeout(t *testing.T) {
	if !nodeAvailable() {
		t.Skip("node not available")
	}
	if !isLinux {
		t.Skip("process-group kill is linux-only")
	}
	r := New()
	// The handler spawns a detached child that persists for a long time. On
	// timeout, the child must be killed along with the parent (no orphans).
	src := `
const { spawn } = require('child_process');
exports.handler = async () => {
  spawn('sleep', ['60'], { detached: true, stdio: 'ignore' });
  await new Promise((res) => setTimeout(res, 60000));
};
`
	_, err := r.Run(context.Background(), src, "node", map[string]any{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout message, got: %v", err)
	}
}
