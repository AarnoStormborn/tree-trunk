package ui

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// renderDiff turns a unified diff into a rich view (inspired by delta/riff,
// not a copy): line-number gutters, add/remove line backgrounds, and
// word-level intra-line highlighting on paired changed lines.
//
// It is deliberately conservative: file/hunk headers are styled, context
// lines get a dim numbered gutter, and word-diff is only computed for
// reasonably short paired lines to stay fast on large/minified files.
func renderDiff(raw string) string {
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	var out strings.Builder
	oldNum, newNum := 0, 0

	i := 0
	firstFile := true
	firstHunkInFile := true
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if !firstFile {
				out.WriteByte('\n')
			}
			out.WriteString(renderFileBar(diffFilePath(line), diffFileStatus(lines, i), 80))
			out.WriteByte('\n')
			firstFile = false
			firstHunkInFile = true
			i++
			for i < len(lines) && isDiffMetadata(lines[i]) {
				i++
			}

		case strings.HasPrefix(line, "@@"):
			oldNum, newNum = parseHunkHeader(line)
			if !firstHunkInFile {
				out.WriteString(gutterBlank() + g.lineNum.Render(strings.Repeat("─", 60)))
				out.WriteByte('\n')
			}
			firstHunkInFile = false
			out.WriteString(gutterBlank() + g.diffHunk.Render(line))
			out.WriteByte('\n')
			i++

		case isDiffMetadata(line):
			i++ // stray metadata: hide it

		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			// A change block: a run of '-' lines, then a run of '+' lines.
			del, add, next := collectChangeBlock(lines, i)
			renderChangeBlock(&out, del, add, &oldNum, &newNum)
			i = next

		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			// Pure addition (no preceding removals).
			newNum++
			out.WriteString(gutter(0, newNum) + g.diffAddLine.Render(line))
			out.WriteByte('\n')
			i++

		default:
			// Context line (starts with ' ' or is blank).
			oldNum++
			newNum++
			out.WriteString(gutter(oldNum, newNum) + g.lineNum.Render(line))
			out.WriteByte('\n')
			i++
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// parseHunkHeader returns the starting line numbers (minus one, so the first
// content line increments to the real start).
func parseHunkHeader(line string) (oldNum, newNum int) {
	m := hunkRe.FindStringSubmatch(line)
	if m == nil {
		return oldNum, newNum
	}
	o, _ := strconv.Atoi(m[1])
	n, _ := strconv.Atoi(m[2])
	return o - 1, n - 1
}

// collectChangeBlock gathers consecutive '-' lines then consecutive '+'
// lines starting at index i, returning their contents (without the marker)
// and the index just past the block.
func collectChangeBlock(lines []string, i int) (del, add []string, next int) {
	for i < len(lines) && strings.HasPrefix(lines[i], "-") && !strings.HasPrefix(lines[i], "---") {
		del = append(del, lines[i])
		i++
	}
	for i < len(lines) && strings.HasPrefix(lines[i], "+") && !strings.HasPrefix(lines[i], "+++") {
		add = append(add, lines[i])
		i++
	}
	return del, add, i
}

// renderChangeBlock renders removed lines then added lines. When a removed
// line pairs with an added line (same offset) and both are short enough,
// word-level highlighting marks the exact changed spans.
func renderChangeBlock(out *strings.Builder, del, add []string, oldNum, newNum *int) {
	for k, d := range del {
		*oldNum++
		var body string
		if k < len(add) && wordDiffable(d, add[k]) {
			body = highlightWords(d, add[k], true)
		} else {
			body = g.diffDelLine.Render(d)
		}
		out.WriteString(gutter(*oldNum, 0) + body)
		out.WriteByte('\n')
	}
	for k, a := range add {
		*newNum++
		var body string
		if k < len(del) && wordDiffable(del[k], a) {
			body = highlightWords(del[k], a, false)
		} else {
			body = g.diffAddLine.Render(a)
		}
		out.WriteString(gutter(0, *newNum) + body)
		out.WriteByte('\n')
	}
}

// wordDiffable reports whether a removed/added pair is worth (and safe) to
// word-diff: both present, not too long, and not wholly different.
func wordDiffable(del, add string) bool {
	if len(del) < 2 || len(add) < 2 {
		return false
	}
	if len(del) > 400 || len(add) > 400 {
		return false // minified/huge lines: skip for speed
	}
	return true
}

// highlightWords renders one side of a paired change with the differing
// segments highlighted brighter. del/add include the leading marker; delSide
// selects which side (and thus which edits) to render.
func highlightWords(del, add string, delSide bool) string {
	dOld := del[1:] // drop marker
	dNew := add[1:]

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(dOld, dNew, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	var b strings.Builder
	if delSide {
		b.WriteString(g.diffDelLine.Render("-"))
		for _, d := range diffs {
			switch d.Type {
			case diffmatchpatch.DiffEqual:
				b.WriteString(g.diffDelLine.Render(d.Text))
			case diffmatchpatch.DiffDelete:
				b.WriteString(g.diffDelWord.Render(d.Text))
			}
		}
	} else {
		b.WriteString(g.diffAddLine.Render("+"))
		for _, d := range diffs {
			switch d.Type {
			case diffmatchpatch.DiffEqual:
				b.WriteString(g.diffAddLine.Render(d.Text))
			case diffmatchpatch.DiffInsert:
				b.WriteString(g.diffAddWord.Render(d.Text))
			}
		}
	}
	return b.String()
}

// gutter renders the two-column line-number gutter. A zero means "blank"
// (e.g. old number on an added line).
func gutter(oldN, newN int) string {
	return g.lineNum.Render(pad(oldN) + " " + pad(newN) + " ")
}

func gutterBlank() string {
	return g.lineNum.Render(strings.Repeat(" ", 5) + "  ")
}

func pad(n int) string {
	if n == 0 {
		return "    "
	}
	s := strconv.Itoa(n)
	if len(s) >= 4 {
		return s
	}
	return strings.Repeat(" ", 4-len(s)) + s
}

// --- multi-file helpers (shared by unified + side-by-side) ---

// diffFilePath extracts the new-side path from a "diff --git a/OLD b/NEW"
// line (best-effort; handles the common unquoted case).
func diffFilePath(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	if i := strings.Index(rest, " b/"); i >= 0 {
		return rest[i+len(" b/"):]
	}
	return strings.TrimSpace(rest)
}

// diffFileStatus looks ahead from a "diff --git" line for a new/deleted/
// renamed marker and returns a short tag ("" for a plain modification).
func diffFileStatus(lines []string, start int) string {
	for j := start + 1; j < len(lines) && j < start+6; j++ {
		switch {
		case strings.HasPrefix(lines[j], "new file"):
			return "new file"
		case strings.HasPrefix(lines[j], "deleted file"):
			return "deleted"
		case strings.HasPrefix(lines[j], "rename "):
			return "renamed"
		case strings.HasPrefix(lines[j], "Binary files"):
			return "binary"
		case strings.HasPrefix(lines[j], "@@"), strings.HasPrefix(lines[j], "diff --git"):
			return ""
		}
	}
	return ""
}

// isDiffMetadata reports whether a line is git-diff metadata that we hide in
// favour of the clean file-header bar.
func isDiffMetadata(line string) bool {
	for _, p := range []string{"index ", "--- ", "+++ ", "old mode", "new mode", "new file", "deleted file", "similarity ", "rename ", "copy ", "Binary files", "GIT binary"} {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// renderFileBar renders a full-width file-header bar for the diff.
func renderFileBar(path, status string, width int) string {
	label := " " + path
	if status != "" {
		label += "  " + status
	}
	if width < 1 {
		width = 1
	}
	label = runewidth.Truncate(label, width, "…")
	return g.diffFileBar.Width(width).Render(label)
}
