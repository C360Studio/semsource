//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// quickstartStep pairs one marked doc block (by position) with what must be
// true after it runs. The step-count coupling is deliberate (design D2): a
// new documented block fails the count check loudly, forcing someone to
// decide what must be true after it.
type quickstartStep struct {
	// name labels the step in failure output.
	name string
	// background starts the block and leaves it running (the engine start
	// step). The process is interrupted at track teardown.
	background bool
	// retryFor re-runs the block until it exits 0 (poll-style steps such as
	// the readiness curl, which a user also re-runs until ready). Zero means
	// one attempt.
	retryFor time.Duration
	// retryUntil, when set, additionally re-runs the block until its output
	// satisfies the predicate (e.g. the status payload finally says ready).
	// The verbatim documented command is what gets re-run — the test never
	// substitutes its own query.
	retryUntil func(output string) bool
	// assert runs after the block succeeds. Nil means the block's exit code
	// is the whole contract.
	assert func(t *testing.T, env *quickstartEnv)
}

// quickstartEnv is the execution context a track runs in: an isolated scratch
// HOME/workdir, the harness NATS, a free HTTP port, and the built binary on
// PATH. Doc blocks execute verbatim; the environment supplies only what the
// document tells a real user to have (a NATS URL, a port).
type quickstartEnv struct {
	workDir  string
	httpPort int
	natsURL  string
	binDir   string
	env      []string
	cwdFile  string
	// lastOut is the most recent block attempt's combined stdout+stderr —
	// what a step assertion inspects.
	lastOut string
	// background process bookkeeping for teardown.
	bg []*exec.Cmd
}

// lastOutput returns the most recent step's captured output.
func (e *quickstartEnv) lastOutput() string {
	return e.lastOut
}

// cwd returns the persisted working directory blocks currently run in.
func (e *quickstartEnv) cwd() string {
	data, err := os.ReadFile(e.cwdFile)
	if err != nil {
		return e.workDir
	}
	return strings.TrimSpace(string(data))
}

// newQuickstartEnv builds the isolated environment for one track.
func newQuickstartEnv(t *testing.T, natsURL, binPath string) *quickstartEnv {
	t.Helper()
	workDir := t.TempDir()
	httpPort := freePort(t)
	wsPort := freePort(t)
	binDir := filepath.Dir(binPath)

	cwdFile := filepath.Join(workDir, ".quickstart-cwd")
	if err := os.WriteFile(cwdFile, []byte(workDir+"\n"), 0o644); err != nil {
		t.Fatalf("seed cwd file: %v", err)
	}

	env := append(os.Environ(),
		// HOME under the scratch dir isolates ~/.semsource/repos (the git
		// workspace) and any git config from the developer's real home.
		"HOME="+workDir,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"NATS_URL="+natsURL,
		fmt.Sprintf("SEMSOURCE_HTTP_PORT=%d", httpPort),
		fmt.Sprintf("SEMSOURCE_WS_BIND=127.0.0.1:%d", wsPort),
		"GIT_TERMINAL_PROMPT=0",
	)

	return &quickstartEnv{
		workDir:  workDir,
		httpPort: httpPort,
		natsURL:  natsURL,
		binDir:   binDir,
		env:      env,
		cwdFile:  cwdFile,
	}
}

// runQuickstartTrack extracts the track's blocks from docs/QUICKSTART.md and
// executes them verbatim, in document order, invoking each step's assertion
// as it completes. The document is the single source of the commands; the
// test holds only the assertions.
func runQuickstartTrack(t *testing.T, track string, natsURL string, steps []quickstartStep) {
	t.Helper()

	docPath := filepath.Join(repoRoot(t), "docs", "QUICKSTART.md")
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read quickstart document: %v", err)
	}
	blocks, err := ExtractQuickstartBlocks(string(raw), track)
	if err != nil {
		t.Fatalf("extract %s track blocks: %v", track, err)
	}
	if len(blocks) != len(steps) {
		var got []string
		for i, b := range blocks {
			got = append(got, fmt.Sprintf("  block %d: %s (line %d)", i+1, b.Heading, b.Line))
		}
		t.Fatalf("docs/QUICKSTART.md %s track has %d marked blocks but the test defines %d steps.\n"+
			"A documented step changed — decide what must be true after it and update the step table.\n%s",
			track, len(blocks), len(steps), strings.Join(got, "\n"))
	}

	binPath := buildBinary(t)
	env := newQuickstartEnv(t, natsURL, binPath)
	defer env.teardown(t)

	for i, block := range blocks {
		step := steps[i]
		label := fmt.Sprintf("quickstart[%s] step %d/%d %q (doc: %q, line %d)",
			track, i+1, len(steps), step.name, block.Heading, block.Line)
		t.Logf("%s:\n%s", label, block.Script)

		if step.background {
			env.startBackground(t, label, block.Script)
		} else {
			env.runBlock(t, label, block.Script, step.retryFor, step.retryUntil)
		}
		if step.assert != nil {
			step.assert(t, env)
			if t.Failed() {
				t.Fatalf("%s: assertion failed (see errors above)", label)
			}
		}
	}
}

// wrapScript wraps a doc block so consecutive blocks behave like one user
// shell session: start from the persisted cwd, fail fast like a user watching
// exit codes, and persist the cwd the block ends in.
func (e *quickstartEnv) wrapScript(script string, persistCwd bool) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	b.WriteString(fmt.Sprintf("cd \"$(cat %q)\"\n", e.cwdFile))
	b.WriteString(script)
	b.WriteString("\n")
	if persistCwd {
		b.WriteString(fmt.Sprintf("pwd > %q\n", e.cwdFile))
	}
	return b.String()
}

// runBlock executes one foreground block, optionally retrying until it exits
// zero within retryFor (the poll-until-ready idiom the document describes in
// prose). The block's combined output is captured for assertions.
func (e *quickstartEnv) runBlock(t *testing.T, label, script string, retryFor time.Duration, retryUntil func(string) bool) {
	t.Helper()
	deadline := time.Now().Add(retryFor)
	for {
		cmd := exec.Command("bash", "-c", e.wrapScript(script, true))
		cmd.Env = e.env
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		e.setLastOutput(out.String())
		if err == nil && (retryUntil == nil || retryUntil(out.String())) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: block failed: err=%v (retryUntil satisfied: %v)\noutput:\n%s",
				label, err, err == nil && retryUntil != nil && retryUntil(out.String()), out.String())
		}
		time.Sleep(2 * time.Second)
	}
}

// startBackground launches a long-running block (the engine) and keeps the
// process for teardown. Output streams to the test log.
func (e *quickstartEnv) startBackground(t *testing.T, label, script string) {
	t.Helper()
	cmd := exec.Command("bash", "-c", e.wrapScript(script, false))
	cmd.Env = e.env
	// Own process group: the block runs the engine as a child of bash, and a
	// signal to bash alone would orphan it holding NATS and the HTTP port.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s: stdout pipe: %v", label, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("%s: stderr pipe: %v", label, err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", label, err)
	}
	logPipe(t, label+" stdout", stdout)
	logPipe(t, label+" stderr", stderr)
	e.bg = append(e.bg, cmd)
	e.setLastOutput("")
}

// setLastOutput records a step's captured output.
func (e *quickstartEnv) setLastOutput(out string) {
	e.lastOut = out
}

// teardown interrupts background processes and waits briefly for a clean
// exit, escalating to kill.
func (e *quickstartEnv) teardown(t *testing.T) {
	t.Helper()
	for _, cmd := range e.bg {
		if cmd.Process == nil {
			continue
		}
		// Signal the whole process group (see startBackground).
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		done := make(chan error, 1)
		go func(c *exec.Cmd) { done <- c.Wait() }(cmd)
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
	}
}
