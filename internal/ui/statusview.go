package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// statusView renders the selected repo's Status (docs/design/04-tui-layout.md
// §5.1). It is a pure projection of the model — no git calls here. File rows
// are selectable (j/k) so `enter` can open a scoped diff (review M9).
type statusView struct {
	repo     *model.Repo
	width    int
	height   int
	files    []model.StatusFile // flattened, section-ordered rows
	cursor   int
	hasFocus bool
}

// rebuildFiles flattens the file rows in section order (conflicts, staged,
// unstaged, untracked) for cursor navigation.
func (v *statusView) rebuildFiles() {
	if v.repo == nil {
		v.files = nil
		return
	}
	st := &v.repo.Status
	v.files = v.files[:0]
	appendFiles := func(filter fileFilter) {
		for _, f := range st.Files {
			if filter(f) {
				v.files = append(v.files, f)
			}
		}
	}
	appendFiles(conflictFilter)
	appendFiles(stagedFilter)
	appendFiles(unstagedFilter)
	appendFiles(untrackedFilter)
	if v.cursor >= len(v.files) {
		v.cursor = 0
	}
}

// render produces the pane body for the current repo ("" when none).
func (v statusView) render() string {
	if v.repo == nil {
		return g.dim.Render("no repo selected")
	}

	var b strings.Builder
	st := &v.repo.Status

	// Branch line.
	branch := st.Branch
	if branch == "" {
		branch = "HEAD"
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("On branch " + branch))
	b.WriteString("\n")

	// Ahead/behind line.
	switch {
	case st.Upstream == "":
		b.WriteString(g.dim.Render("(no upstream)"))
	case st.Ahead > 0 && st.Behind > 0:
		b.WriteString(g.dim.Render("Your branch is ahead of '" + st.Upstream + "' by " + itoa(st.Ahead) + " commits, and behind by " + itoa(st.Behind) + "."))
	case st.Ahead > 0:
		b.WriteString(g.dim.Render("Your branch is ahead of '" + st.Upstream + "' by " + itoa(st.Ahead) + " commits."))
	case st.Behind > 0:
		b.WriteString(g.dim.Render("Your branch is behind '" + st.Upstream + "' by " + itoa(st.Behind) + " commits."))
	default:
		b.WriteString(g.dim.Render("Your branch is up to date with '" + st.Upstream + "'."))
	}
	b.WriteString("\n\n")

	if st.Conflicts > 0 {
		writeSection(&b, "Unmerged paths (conflicts)", st, conflictFilter, g.conflict, v)
	}
	if st.Staged > 0 {
		writeSection(&b, "Changes to be committed", st, stagedFilter, g.staged, v)
	}
	if st.Unstaged > 0 {
		writeSection(&b, "Changes not staged for commit", st, unstagedFilter, g.unstaged, v)
	}
	if st.Untracked > 0 {
		writeSection(&b, "Untracked files", st, untrackedFilter, g.untracked, v)
	}

	if !st.Dirty() {
		b.WriteString(g.dim.Render("nothing to commit, working tree clean"))
	}

	// Truncate to the pane height.
	lines := strings.Split(b.String(), "\n")
	if v.height > 0 && len(lines) > v.height {
		lines = append(lines[:v.height-1], g.dim.Render("… truncated"))
	}
	return strings.Join(lines, "\n")
}

type fileFilter func(f model.StatusFile) bool

func conflictFilter(f model.StatusFile) bool  { return f.Conflict() }
func stagedFilter(f model.StatusFile) bool    { return f.Staged() && !f.Conflict() }
func unstagedFilter(f model.StatusFile) bool  { return f.Unstaged() && !f.Conflict() }
func untrackedFilter(f model.StatusFile) bool { return f.Untracked() }

func writeSection(b *strings.Builder, title string, st *model.RepoStatus, filter fileFilter, style lipgloss.Style, v statusView) {
	b.WriteString(style.Render(title))
	b.WriteString("\n")
	for _, f := range st.Files {
		if !filter(f) {
			continue
		}
		line := "  " + string([]byte{f.X, f.Y}) + " " + f.Path
		if f.OrigPath != "" {
			line += " (from " + f.OrigPath + ")"
		}
		// Highlight the selected row (cursor maps into the flattened list).
		if v.hasFocus {
			for i := range v.files {
				if i == v.cursor && v.files[i].Path == f.Path && v.files[i].X == f.X && v.files[i].Y == f.Y {
					line = ">" + line[1:]
					style = style.Background(g.selColor)
				}
			}
		}
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}
}

// selectedFile returns the file under the cursor (nil when clean).
func (v *statusView) selectedFile() *model.StatusFile {
	if v.repo == nil || len(v.files) == 0 {
		return nil
	}
	if v.cursor >= len(v.files) {
		v.cursor = len(v.files) - 1
	}
	return &v.files[v.cursor]
}
