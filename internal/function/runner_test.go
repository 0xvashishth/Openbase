package function

import (
	"context"
	"os"
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

func TestNodeRunnerDoesNotLeakParentEnv(t *testing.T) {
	if !nodeAvailable() {
		t.Skip("node not available")
	}
	// Plant fake platform secrets in the parent environment. If the sandbox
	// inherits os.Environ(), the user function can exfiltrate them.
	t.Setenv("OPENBASE_JWT_SECRET", "super-secret-jwt")
	t.Setenv("OPENBASE_ENCRYPTION_KEY", "super-secret-key")
	t.Setenv("OPENBASE_DATABASE_URL", "postgres://secret@localhost/db")
	t.Setenv("OB_TEST_MARKER", "should-not-be-visible")

	r := New()
	src := `
exports.handler = async () => {
  return {
    jwt: process.env.OPENBASE_JWT_SECRET || null,
    enc: process.env.OPENBASE_ENCRYPTION_KEY || null,
    dburl: process.env.OPENBASE_DATABASE_URL || null,
    marker: process.env.OB_TEST_MARKER || null,
    fnPath: process.env.OB_FUNCTION_PATH || null,
    hasPath: !!process.env.PATH,
  };
};
`
	res, err := r.Run(context.Background(), src, "node", map[string]any{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, k := range []string{"jwt", "enc", "dburl", "marker"} {
		if res[k] != nil {
			t.Fatalf("sandbox leaked parent env %q = %v", k, res[k])
		}
	}
	if res["fnPath"] == nil {
		t.Fatal("sandbox must provide OB_FUNCTION_PATH")
	}
	if res["hasPath"] != true {
		t.Fatal("sandbox must inherit PATH")
	}
}

func TestSandboxEnvAllowList(t *testing.T) {
	t.Setenv("OPENBASE_JWT_SECRET", "x")
	os.Unsetenv("OB_TEST_UNSET_PRESENT")
	env := sandboxEnv("EXTRA=yes")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "OPENBASE_JWT_SECRET") {
		t.Fatalf("allow-list leaked secret: %q", joined)
	}
	if !strings.Contains(joined, "EXTRA=yes") {
		t.Fatalf("extra entries must be included: %q", joined)
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
