package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// worktreesView renders the Worktrees tab (04-tui-layout.md §5.4): one row
// per worktree with branch, head, dirty, locked, prunable, current.
type worktreesView struct {
	repo     *model.Repo
	cursor   int
	width    int
	height   int
	hasFocus bool
}

func (v worktreesView) render() string {
	if v.repo == nil {
		return dimStyle.Render("no repo selected")
	}
	wts := v.repo.Worktrees
	if len(wts) == 0 {
		return dimStyle.Render("no worktrees — press n to create one")
	}
	if v.cursor >= len(wts) {
		v.cursor = len(wts) - 1
	}

	var b strings.Builder
	for i, wt := range wts {
		sel := i == v.cursor && v.hasFocus
		line := renderWorktreeRow(wt, sel)
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Footer with the two-step delete hint.
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("n new · d delete (two-step) · L lock/unlock · P prune · o open"))

	return b.String()
}

func renderWorktreeRow(wt model.Worktree, selected bool) string {
	var s string

	prefix := " "
	if selected {
		prefix = "▶"
	}
	s += prefix + " "

	// Dirty / current / locked / prunable markers.
	markers := ""
	if wt.Dirty {
		markers += "~"
	}
	if wt.IsCurrent {
		markers += "*"
	}
	if wt.Locked {
		markers += "🔒"
	}
	if wt.Prunable {
		markers += "⚠"
	}
	if markers != "" {
		s += markers + " "
	}

	branch := wt.Branch
	if branch == "" {
		branch = "HEAD (detached)"
	}
	if wt.IsMain {
		s += lipgloss.NewStyle().Bold(true).Render(branch)
	} else {
		s += branch
	}

	// Head (short) + path.
	if len(wt.Head) >= 7 {
		s += " " + dimStyle.Render(wt.Head[:7])
	}
	s += "  " + dimStyle.Render(shortPath(wt.Path))

	if wt.LockReason != "" {
		s += dimStyle.Render(" (" + wt.LockReason + ")")
	}
	if wt.IsPathMissing {
		s += dimStyle.Render(" [missing]")
	}

	style := lipgloss.NewStyle()
	if selected {
		style = style.Background(lipgloss.Color("237"))
	}
	if wt.Dirty {
		style = style.Foreground(lipgloss.Color("220"))
	}
	return style.Render(s)
}

func shortPath(p string) string {
	home, err := homeDir()
	if err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// createForm is the worktree-creation modal (04-tui-layout.md §4).
type createForm struct {
	repo    *model.Repo
	branch  textinput.Model
	base    textinput.Model
	path    textinput.Model
	focused int // 0..2
	pathDir string
}

func newCreateForm(repo *model.Repo, worktreesDir string) *createForm {
	suggestion := repo.Branch
	if suggestion == "" || suggestion == "HEAD" {
		suggestion = "main"
	}

	mk := func(placeholder, value string) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.SetValue(value)
		return t
	}

	f := &createForm{
		repo:    repo,
		branch:  mk("feature/my-thing", ""),
		base:    mk("base branch", suggestion),
		pathDir: worktreesDir,
		focused: 0,
	}
	f.branch.Focus()
	f.syncPath()
	return f
}

func (f *createForm) syncPath() {
	branch := f.branch.Value()
	if branch == "" {
		branch = "feature/worktree"
	}
	p := suggestWorktreePath(f.pathDir, f.repo.Name, branch)
	f.path.SetValue(uniquePath(p))
}

// handleKey processes form keystrokes: typing edits the focused field, tab
// moves focus, enter submits, esc cancels. Returns the outcome.
func (f *createForm) handleKey(msg tea.KeyMsg, _ string) createFormUpdate {
	fields := []*textinput.Model{&f.branch, &f.base, &f.path}
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		return createFormUpdate{form: f, cancel: true}
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if f.path.Value() == "" || f.branch.Value() == "" {
			return createFormUpdate{form: f}
		}
		return createFormUpdate{form: f, done: true}
	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		fields[f.focused].Blur()
		f.focused = (f.focused + 1) % 3
		fields[f.focused].Focus()
		return createFormUpdate{form: f}
	default:
		before := f.branch.Value()
		var cmd tea.Cmd
		*fields[f.focused], cmd = fields[f.focused].Update(msg)
		if f.branch.Value() != before {
			f.syncPath()
		}
		_ = cmd
		return createFormUpdate{form: f}
	}
}

func (f *createForm) update(msg interface{}) (createFormUpdate, bool) {
	type updateMsg interface{ String() string }
	switch m := msg.(type) {
	case updateMsg:
		_ = m
	}
	// handled by app layer key dispatch; fields updated there
	return createFormUpdate{form: f}, false
}

// createFormUpdate carries the result of a form keystroke.
type createFormUpdate struct {
	form   *createForm
	done   bool
	cancel bool
}

// formRender renders the modal.
func (f *createForm) render(width int) string {
	title := lipgloss.NewStyle().Bold(true).Render("New worktree — " + f.repo.Name)
	rows := []string{
		"branch  " + f.fieldRow(f.focused == 0, f.branch),
		"base    " + f.fieldRow(f.focused == 1, f.base),
		"path    " + f.fieldRow(f.focused == 2, f.path),
	}
	body := strings.Join(rows, "\n")
	hint := dimStyle.Render("tab next · enter create · esc cancel")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 2).
		Width(60)
	return box.Render(title + "\n\n" + body + "\n\n" + hint)
}

func (f *createForm) fieldRow(active bool, t textinput.Model) string {
	if active {
		return t.View()
	}
	return dimStyle.Render(t.Value())
}

func homeDir() (string, error) {
	return os.UserHomeDir()
}

// confirmModal is a two-choice dialog; onConfirm runs when the user
// confirms (04-tui-layout.md §7).
type confirmModal struct {
	title, msg, confirmLabel string
	danger                   bool
	onConfirm                func() tea.Cmd
}

func (c *confirmModal) render(width int) string {
	title := lipgloss.NewStyle().Bold(true).Render(c.title)
	if c.danger {
		title = conflictStyle.Render(c.title)
	}
	confirm := c.confirmLabel
	if c.danger {
		confirm = conflictStyle.Render(c.confirmLabel)
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 2).
		Width(64)
	return box.Render(title + "\n\n" + c.msg + "\n\n" +
		confirm + "  (enter)   " + dimStyle.Render("cancel (esc)"))
}

// newDeleteConfirm is the first step of the two-step delete (safe remove).
func newDeleteConfirm(actions *worktreeActions, repo *model.Repo, wt model.Worktree) *confirmModal {
	return &confirmModal{
		title:        "Delete worktree",
		msg:          "Delete the worktree at\n\n  " + wt.Path + "\n\nThe branch is NOT deleted.",
		confirmLabel: "Delete",
		danger:       true,
		onConfirm: func() tea.Cmd {
			return actions.removeCmd(repo, wt, false)
		},
	}
}

// newForceDeleteConfirm is the second step: safe remove was refused because
// the worktree is dirty — explicit --force confirmation (03 §3.2).
func newForceDeleteConfirm(actions *worktreeActions, repo *model.Repo, wt model.Worktree) *confirmModal {
	return &confirmModal{
		title:        "Force remove worktree",
		msg:          "The worktree has modified or untracked files.\n\nRemoving with --force DISCARDS those changes:\n\n  " + wt.Path,
		confirmLabel: "Force remove",
		danger:       true,
		onConfirm: func() tea.Cmd {
			return actions.removeCmd(repo, wt, true)
		},
	}
}

func newPruneConfirm(actions *worktreeActions, repo *model.Repo) *confirmModal {
	return &confirmModal{
		title:        "Prune worktrees",
		msg:          "Run `git worktree prune` for this repo?\n\nRemoves administrative metadata for deleted worktrees.",
		confirmLabel: "Prune",
		onConfirm: func() tea.Cmd {
			return actions.pruneCmd(repo, false)
		},
	}
}
