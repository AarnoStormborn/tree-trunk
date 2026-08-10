package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestColorizeDiff(t *testing.T) {
	old := lipgloss.DefaultRenderer()
	r := lipgloss.NewRenderer(os.Stdout)
	r.SetColorProfile(termenv.ANSI256)
	lipgloss.SetDefaultRenderer(r)
	t.Cleanup(func() {
		lipgloss.SetDefaultRenderer(old)
		initStyles(DefaultTheme(), nil)
	})
	initStyles(DefaultTheme(), nil)

	diff := "diff --git a/f b/f\n@@ -1 +1,2 @@\n context\n+added\n-removed\n"
	out := colorizeDiff(diff)

	// Additions green (34), deletions red (196), hunk cyan (44).
	if !strings.Contains(out, "\x1b[38;5;34m+added") {
		t.Errorf("added line not green:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;196m-removed") {
		t.Errorf("removed line not red:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;44m@@") {
		t.Errorf("hunk header not cyan:\n%q", out)
	}
	// Context lines stay uncolored.
	if strings.Contains(out, "\x1b[38;5;34m context") {
		t.Errorf("context line should not be green")
	}
}

func TestColorizeDiffEmpty(t *testing.T) {
	if colorizeDiff("") != "" {
		t.Fatal("empty diff should stay empty")
	}
}
