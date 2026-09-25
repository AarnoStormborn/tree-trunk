package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written. Used to keep the JSON envelopes out of the test log.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{"repo_not_found", 3},
		{"worktree_not_found", 3},
		{"worktree_dirty", 4},
		{"worktree_locked", 5},
		{"branch_exists", 1},
		{"branch_checked_out_elsewhere", 1},
		{"git_error", 1},
		{"", 1},
	}
	for _, c := range cases {
		if got := exitCodeFor(&wtError{Code: c.code}); got != c.want {
			t.Errorf("exitCodeFor(%q) = %d, want %d", c.code, got, c.want)
		}
	}
}

// A failed wt op must print the JSON envelope AND return an exit code, so
// agents can branch on process status alone (docs/design/10-agent-cli.md).
func TestWTErrorReturnsExitCoder(t *testing.T) {
	var got int
	out := captureStdout(t, func() {
		err := runWTCommand([]string{"delete", "no-such-repo-xyz", "feat/x", "--no-scan"}, "test")
		var ec exitCoder
		if !errors.As(err, &ec) {
			t.Fatalf("wt delete on unknown repo: got %v, want an exitCoder", err)
		}
		got = ec.ExitCode()
	})
	if got != 3 {
		t.Errorf("unknown-repo exit code = %d, want 3", got)
	}
	if !strings.Contains(out, `"repo_not_found"`) || !strings.Contains(out, `"ok": false`) {
		t.Errorf("envelope missing structured error: %s", out)
	}
}

// Usage mistakes are exit 1 (plain error → main's default).
func TestWTUsageErrorIsNotCoded(t *testing.T) {
	var errIsCoded bool
	out := captureStdout(t, func() {
		err := runWTCommand([]string{"nonsense-op"}, "test")
		var ec exitCoder
		errIsCoded = errors.As(err, &ec)
	})
	if out != "" {
		t.Errorf("usage error wrote to stdout: %q", out)
	}
	if errIsCoded {
		t.Error("usage error should use the default exit code 1, not an exitCoder")
	}
}

func TestWTHelpExitsZero(t *testing.T) {
	out := captureStdout(t, func() {
		if err := runWTCommand([]string{"--help"}, "test"); err != nil {
			t.Fatalf("wt --help returned %v, want nil (exit 0)", err)
		}
	})
	if !strings.Contains(out, "Exit codes") {
		t.Errorf("wt --help did not print usage: %q", out)
	}
}

// --help must not be reported as a failure by the subcommands.
func TestSubcommandHelpIsNotAnError(t *testing.T) {
	err := run([]string{"query", "--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("query --help = %v, want flag.ErrHelp (main exits 0)", err)
	}
}

func TestTopLevelHelpIsNotAnError(t *testing.T) {
	if err := run([]string{"-h"}); err != nil {
		t.Fatalf("tree-trunk -h = %v, want nil", err)
	}
}

func TestDescribeIncludesExitCodes(t *testing.T) {
	out := captureStdout(t, func() {
		if err := printDescribe("test"); err != nil {
			t.Fatalf("describe: %v", err)
		}
	})
	var schema struct {
		ExitCodes map[string]string `json:"exit_codes"`
	}
	if err := json.Unmarshal([]byte(out), &schema); err != nil {
		t.Fatalf("describe output is not JSON: %v", err)
	}
	for _, code := range []string{"0", "1", "3", "4", "5"} {
		if schema.ExitCodes[code] == "" {
			t.Errorf("describe schema missing exit code %s", code)
		}
	}
}
