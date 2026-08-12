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

func TestRenderSideBySide(t *testing.T) {
	withColor(t)
	diff := "@@ -1,3 +1,4 @@\n ctx\n-old line\n+new line\n ctx2\n+added\n@@ -20,1 +21,1 @@\n-tail\n+TAIL\n"
	out := renderDiffSideBySide(diff, 120)
	plain := stripANSI(out)

	// Column divider present on content rows.
	if !strings.Contains(plain, "│") {
		t.Errorf("column divider missing:\n%s", plain)
	}
	// Both old and new content appear.
	if !strings.Contains(plain, "old line") || !strings.Contains(plain, "new line") {
		t.Errorf("paired content missing:\n%s", plain)
	}
	// Hunk separator rule between the two hunks.
	if !strings.Contains(plain, strings.Repeat("─", 20)) {
		t.Errorf("hunk separator rule missing:\n%s", plain)
	}
	// Backgrounds (del red 52, add green 22) present.
	if !strings.Contains(out, "48;5;52") || !strings.Contains(out, "48;5;22") {
		t.Error("add/remove backgrounds missing in split view")
	}
}

func TestSideBySideNarrowFallsBack(t *testing.T) {
	withColor(t)
	// Too narrow → falls back to the unified renderer (no column divider row).
	out := renderDiffSideBySide("@@ -1 +1 @@\n-a\n+b\n", 20)
	// Unified renderer output has the gutter but not the " │ " column divider.
	if strings.Contains(stripANSI(out), " │ ") {
		t.Error("narrow width should fall back to unified, not split")
	}
}

func TestRenderDiffMultiFile(t *testing.T) {
	withColor(t)
	diff := "diff --git a/alpha.go b/alpha.go\n" +
		"index 1..2 100644\n--- a/alpha.go\n+++ b/alpha.go\n" +
		"@@ -1 +1 @@\n-old\n+new\n" +
		"diff --git a/infra.tf b/infra.tf\n" +
		"new file mode 100644\nindex 0..3\n--- /dev/null\n+++ b/infra.tf\n" +
		"@@ -0,0 +1 @@\n+created\n"
	plain := stripANSI(renderDiff(diff))

	// Clean file bars for both files.
	if !strings.Contains(plain, " alpha.go") || !strings.Contains(plain, " infra.tf") {
		t.Errorf("file bars missing:\n%s", plain)
	}
	// New-file status tag.
	if !strings.Contains(plain, "new file") {
		t.Errorf("new-file tag missing:\n%s", plain)
	}
	// Git metadata noise is hidden.
	for _, noise := range []string{"index 1..2", "--- a/alpha.go", "+++ b/alpha.go", "diff --git"} {
		if strings.Contains(plain, noise) {
			t.Errorf("metadata leaked into output: %q\n%s", noise, plain)
		}
	}
}

func TestDiffFilePathAndStatus(t *testing.T) {
	if p := diffFilePath("diff --git a/src/x.go b/src/x.go"); p != "src/x.go" {
		t.Fatalf("path = %q", p)
	}
	lines := []string{"diff --git a/f b/f", "deleted file mode 100644", "@@ -1 +0 @@"}
	if s := diffFileStatus(lines, 0); s != "deleted" {
		t.Fatalf("status = %q, want deleted", s)
	}
}
