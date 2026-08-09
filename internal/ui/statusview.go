package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/harshsingh/tree-trunk/internal/model"
)

// statusView renders the selected repo's Status (docs/design/04-tui-layout.md
// §5.1). It is a pure projection of the model — no git calls here.
type statusView struct {
	repo          *model.Repo
	width, height int
}

// render produces the pane body for the current repo ("" when none).
func (v statusView) render() string {
	if v.repo == nil {
		return dimStyle.Render("no repo selected")
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
		b.WriteString(dimStyle.Render("(no upstream)"))
	case st.Ahead > 0 && st.Behind > 0:
		b.WriteString(dimStyle.Render("Your branch is ahead of '" + st.Upstream + "' by " + itoa(st.Ahead) + " commits, and behind by " + itoa(st.Behind) + "."))
	case st.Ahead > 0:
		b.WriteString(dimStyle.Render("Your branch is ahead of '" + st.Upstream + "' by " + itoa(st.Ahead) + " commits."))
	case st.Behind > 0:
		b.WriteString(dimStyle.Render("Your branch is behind '" + st.Upstream + "' by " + itoa(st.Behind) + " commits."))
	default:
		b.WriteString(dimStyle.Render("Your branch is up to date with '" + st.Upstream + "'."))
	}
	b.WriteString("\n\n")

	if st.Conflicts > 0 {
		writeSection(&b, "Unmerged paths (conflicts)", st, conflictFilter, conflictStyle)
	}
	if st.Staged > 0 {
		writeSection(&b, "Changes to be committed", st, stagedFilter, stagedStyle)
	}
	if st.Unstaged > 0 {
		writeSection(&b, "Changes not staged for commit", st, unstagedFilter, unstagedStyle)
	}
	if st.Untracked > 0 {
		writeSection(&b, "Untracked files", st, untrackedFilter, untrackedStyle)
	}

	if !st.Dirty() {
		b.WriteString(dimStyle.Render("nothing to commit, working tree clean"))
	}

	// Truncate to the pane height.
	lines := strings.Split(b.String(), "\n")
	if v.height > 0 && len(lines) > v.height {
		lines = append(lines[:v.height-1], dimStyle.Render("… truncated"))
	}
	return strings.Join(lines, "\n")
}

type fileFilter func(f model.StatusFile) bool

func conflictFilter(f model.StatusFile) bool  { return f.Conflict() }
func stagedFilter(f model.StatusFile) bool    { return f.Staged() && !f.Conflict() }
func unstagedFilter(f model.StatusFile) bool  { return f.Unstaged() && !f.Conflict() }
func untrackedFilter(f model.StatusFile) bool { return f.Untracked() }

func writeSection(b *strings.Builder, title string, st *model.RepoStatus, filter fileFilter, style lipgloss.Style) {
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
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}
}

var (
	conflictStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	stagedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))  // green
	unstagedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // yellow
	untrackedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // dim
)
