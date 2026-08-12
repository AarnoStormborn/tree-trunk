package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func withColor(t *testing.T) {
	t.Helper()
	old := lipgloss.DefaultRenderer()
	r := lipgloss.NewRenderer(os.Stdout)
	r.SetColorProfile(termenv.ANSI256)
	lipgloss.SetDefaultRenderer(r)
	t.Cleanup(func() {
		lipgloss.SetDefaultRenderer(old)
		initStyles(DefaultTheme(), nil)
	})
	initStyles(DefaultTheme(), nil)
}

func TestRenderDiffStructure(t *testing.T) {
	withColor(t)
	diff := "diff --git a/f b/f\n@@ -1,3 +1,4 @@\n ctx\n-old line\n+new line\n ctx2\n+added\n"
	out := renderDiff(diff)
	plain := stripANSI(out)

	// Line-number gutters: context has both, removed has old, added has new.
	if !strings.Contains(plain, "   1    1  ctx") {
		t.Errorf("context gutter missing:\n%s", plain)
	}
	if !strings.Contains(plain, "   2      -old line") {
		t.Errorf("removed gutter missing:\n%s", plain)
	}
	if !strings.Contains(plain, "        2 +new line") {
		t.Errorf("added gutter missing:\n%s", plain)
	}
	// Backgrounds present.
	if !strings.Contains(out, "48;5;52") {
		t.Error("removed-line background missing")
	}
	if !strings.Contains(out, "48;5;22") {
		t.Error("added-line background missing")
	}
}

func TestRenderDiffWordLevel(t *testing.T) {
	withColor(t)
	diff := "@@ -1 +1 @@\n-the quick brown fox\n+the quick red fox\n"
	out := renderDiff(diff)
	// Word backgrounds highlight only the changed token.
	if !strings.Contains(out, "48;5;88") {
		t.Errorf("removed word highlight missing:\n%q", out)
	}
	if !strings.Contains(out, "48;5;28") {
		t.Errorf("added word highlight missing:\n%q", out)
	}
	// The unchanged words stay on the base line background, not the word bg.
	plain := stripANSI(out)
	if !strings.Contains(plain, "the quick") || !strings.Contains(plain, "fox") {
		t.Errorf("content lost:\n%s", plain)
	}
}

func TestRenderDiffHunkNumbers(t *testing.T) {
	oldN, newN := parseHunkHeader("@@ -12,7 +20,9 @@ func x()")
	if oldN != 11 || newN != 19 {
		t.Fatalf("parseHunkHeader = %d,%d, want 11,19", oldN, newN)
	}
}

func TestRenderDiffEmpty(t *testing.T) {
	if renderDiff("") != "" {
		t.Fatal("empty diff should stay empty")
	}
}

func TestWordDiffableGuards(t *testing.T) {
	if wordDiffable("-", "+") {
		t.Error("too-short lines should not word-diff")
	}
	big := "-" + strings.Repeat("x", 500)
	if wordDiffable(big, "+short") {
		t.Error("huge lines should skip word-diff")
	}
	if !wordDiffable("-hello world", "+hello there") {
		t.Error("normal lines should word-diff")
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
