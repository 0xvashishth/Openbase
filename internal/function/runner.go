// Package function executes user-authored functions (SCHEMA.md `functions`
// table) in an isolated, time-bounded child process. Only the Node.js runtime
// is implemented so far; Python can be added behind the same Runner interface.
package function

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Runner executes a user function's source with an event payload.
type Runner interface {
	// Run invokes the handler with event and returns its JSON result. It must
	// enforce a timeout and return an error with context on failure.
	Run(ctx context.Context, source string, runtime string, event map[string]any) (map[string]any, error)
}

// Timeout is the maximum wall-clock duration a single function run may take.
var Timeout = 5 * time.Second

// MaxMemoryMB is the V8 heap limit applied to function subprocesses.
const MaxMemoryMB = 128

// nodeSandbox executes Node.js functions via a small wrapper that requires the
// user module, calls exports.handler(event), and prints the JSON result.
type nodeSandbox struct{}

// New returns a Runner supporting the implemented runtimes.
func New() Runner { return nodeSandbox{} }

// wrapper is the glue that loads a user module and invokes its handler. The
// event is passed via stdin (JSON) and the result is printed to stdout — one
// JSON object on the last line.
const wrapper = `
const readline = require('readline');
const rl = readline.createInterface({ input: process.stdin });
let input = '';
rl.on('line', (l) => { input += l; });
rl.on('close', async () => {
  let event;
  try { event = JSON.parse(input || '{}'); }
  catch (e) { process.stderr.write('bad event json: ' + e.message); process.exit(1); }
  try {
    const mod = require(process.env.OB_FUNCTION_PATH);
    if (typeof mod.handler !== 'function') {
      process.stderr.write('module must export a handler function');
      process.exit(1);
    }
    const result = await mod.handler(event);
    process.stdout.write(JSON.stringify(result === undefined ? { ok: true } : result));
  } catch (e) {
    process.stderr.write((e && e.stack ? e.stack : String(e)));
    process.exit(1);
  }
});
`

func (nodeSandbox) Run(ctx context.Context, source, runtime string, event map[string]any) (map[string]any, error) {
	if runtime != "node" {
		return nil, fmt.Errorf("function: runtime %q not implemented, only node", runtime)
	}
	dir, err := os.MkdirTemp("", "ob-fn-")
	if err != nil {
		return nil, fmt.Errorf("function: mktemp: %w", err)
	}
	defer os.RemoveAll(dir)

	// Write the user module and the wrapper alongside it.
	modPath := filepath.Join(dir, "handler.js")
	if err := os.WriteFile(modPath, []byte(source), 0o600); err != nil {
		return nil, fmt.Errorf("function: write module: %w", err)
	}
	wrapPath := filepath.Join(dir, "wrapper.js")
	if err := os.WriteFile(wrapPath, []byte(wrapper), 0o600); err != nil {
		return nil, fmt.Errorf("function: write wrapper: %w", err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("function: marshal event: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "node",
		"--max-old-space-size="+fmt.Sprint(MaxMemoryMB),
		"--disallow-code-generation-from-strings",
		wrapPath,
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "OB_FUNCTION_PATH="+modPath)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("function: timed out after %s", Timeout)
		}
		detail := stderr.String()
		if detail != "" {
			return nil, fmt.Errorf("function: run failed: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("function: run failed: %w", err)
	}

	// The wrapper prints one JSON line; tolerate surrounding whitespace.
	var result map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return nil, fmt.Errorf("function: result not valid JSON: %w: %s", err, stdout.String())
	}
	return result, nil
}
