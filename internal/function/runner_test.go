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
