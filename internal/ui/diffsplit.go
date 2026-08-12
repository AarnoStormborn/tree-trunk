package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// renderDiffSideBySide renders a unified diff as two aligned columns (old on
// the left, new on the right) with a vertical divider, line-number gutters,
// add/remove backgrounds, and a rule separating each hunk. Inspired by
// delta --side-by-side / git-split-diffs (not a copy).
func renderDiffSideBySide(raw string, width int) string {
	if raw == "" {
		return ""
	}
	const gutterW = 5     // "1234 "
	const divider = " │ " // column separator (the section boundary)
	colW := (width - len(divider)) / 2
	if colW < gutterW+8 {
		// Too narrow for a useful split — fall back to unified.
		return renderDiff(raw)
	}
	contentW := colW - gutterW

	var out strings.Builder
	writeRow := func(left, right string) {
		out.WriteString(left)
		out.WriteString(g.lineNum.Render(divider))
		out.WriteString(right)
		out.WriteByte('\n')
	}
	// cell builds one side: dim line-number gutter + background-filled content.
	cell := func(num int, content string, bg lipgloss.Style, blank bool) string {
		gut := g.lineNum.Render(padNum(num) + " ")
		if blank {
			return gut + bg.Width(contentW).Render("")
		}
		text := runewidth.Truncate(content, contentW, "…")
		return gut + bg.Width(contentW).Render(text)
	}

	lines := strings.Split(raw, "\n")
	oldNum, newNum := 0, 0
	firstFile := true
	firstHunkInFile := true

	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if !firstFile {
				out.WriteByte('\n')
			}
			out.WriteString(renderFileBar(diffFilePath(line), diffFileStatus(lines, i), width))
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
				out.WriteString(g.lineNum.Render(strings.Repeat("─", width)))
				out.WriteByte('\n')
			}
			firstHunkInFile = false
			out.WriteString(g.diffHunk.Render(runewidth.Truncate(line, width, "")))
			out.WriteByte('\n')
			i++

		case isDiffMetadata(line):
			i++ // stray metadata: hide it

		case (strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---")),
			(strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++")):
			del, add, next := collectChangeBlock(lines, i)
			n := len(del)
			if len(add) > n {
				n = len(add)
			}
			for k := 0; k < n; k++ {
				var left, right string
				if k < len(del) {
					oldNum++
					left = cell(oldNum, del[k][1:], g.diffDelLine, false)
				} else {
					left = cell(0, "", plainCell, true)
				}
				if k < len(add) {
					newNum++
					right = cell(newNum, add[k][1:], g.diffAddLine, false)
				} else {
					right = cell(0, "", plainCell, true)
				}
				writeRow(left, right)
			}
			i = next

		default:
			// Context line: same on both sides.
			oldNum++
			newNum++
			content := line
			if len(content) > 0 {
				content = content[1:] // drop leading space
			}
			writeRow(
				cell(oldNum, content, plainCell, false),
				cell(newNum, content, plainCell, false),
			)
			i++
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

// plainCell is an unstyled full-width cell (context / blank lines).
var plainCell = lipgloss.NewStyle()

func padNum(n int) string {
	if n == 0 {
		return "    "
	}
	s := runewidth.Truncate(itoa(n), 4, "")
	return strings.Repeat(" ", 4-len(s)) + s
}
