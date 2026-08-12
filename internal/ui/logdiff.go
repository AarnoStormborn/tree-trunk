package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AarnoStormborn/tree-trunk/internal/git"
	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// logView renders the Log tab (04-tui-layout.md §5.2): paged commit list
// with "load more" at scroll end.
type logView struct {
	repo    *model.Repo
	list    list.Model
	commits []model.Commit
	skip    int
	hasMore bool
	loading bool
	width   int
	height  int
}

func newLogView() logView {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.Title = ""
	return logView{list: l, hasMore: true}
}

type commitItem struct{ c model.Commit }

func (i commitItem) Title() string {
	return i.c.Hash + "  " + i.c.Subject
}
func (i commitItem) Description() string {
	return i.c.Author + "  " + i.c.AuthorDate.Format("2006-01-02 15:04")
}
func (i commitItem) FilterValue() string { return i.c.Subject + " " + i.c.Hash }

func (v logView) render() string {
	if v.repo == nil {
		return g.dim.Render("no repo selected")
	}
	if len(v.commits) == 0 {
		if v.loading {
			return g.dim.Render("loading…")
		}
		return g.dim.Render("no commits")
	}
	return v.list.View()
}

func (v *logView) setCommits(commits []model.Commit, append_ bool) {
	items := make([]list.Item, 0, len(commits))
	for _, c := range commits {
		items = append(items, commitItem{c: c})
	}
	if append_ {
		for _, it := range items {
			v.list.InsertItem(len(v.commits), it)
		}
		v.commits = append(v.commits, commits...)
	} else {
		v.list.SetItems(items)
		v.commits = commits
		if len(commits) > 0 {
			v.list.Select(0) // auto-select the newest commit
		}
	}
}

// loadMoreCmd fetches the next page and appends it.
func loadMoreCmd(runner *git.ExecRunner, repo *model.Repo, skip, limit int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		commits, err := git.Log(ctx, runner, repo.Path, git.LogOptions{Skip: skip, Limit: limit})
		return logPageMsg{repo: repo.ID, commits: commits, err: err}
	}
}

type logPageMsg struct {
	repo    string
	commits []model.Commit
	err     error
}

// diffView renders the Diff tab (04 §5.3): a scrollable viewport with
// mode/stat toggles (held to stat/raw only per M3 scope).
type diffView struct {
	repo     *model.Repo
	viewport viewport.Model
	content  string
	mode     git.DiffMode
	stat     bool
	path     string // scoped file ("" = repo-level)
	loading  bool
	err      string
	width    int
	height   int
}

func (v *diffView) render() string {
	if v.repo == nil {
		return g.dim.Render("no repo selected")
	}
	var head strings.Builder
	head.WriteString(lipgloss.NewStyle().Bold(true).Render("diff: " + v.mode.String()))
	if v.stat {
		head.WriteString(g.dim.Render("  [stat]"))
	}
	if v.path != "" {
		head.WriteString(g.dim.Render("  " + v.path))
	}
	head.WriteString(g.dim.Render("   m cycle · p stat/raw"))
	head.WriteString("\n")

	var body string
	switch {
	case v.loading:
		body = g.dim.Render("loading…")
	case v.err != "":
		body = g.conflict.Render(v.err)
	case v.content == "":
		body = g.dim.Render("(no diff)")
	case v.stat:
		// --stat is a summary, not a unified diff; keep the lighter coloring.
		body = colorizeDiff(v.content)
	default:
		body = renderDiff(v.content)
	}
	return head.String() + "\n" + body
}

// colorizeDiff applies syntax highlighting to unified-diff output: green for
// additions, red for deletions, cyan for hunk headers, bold-dim for file
// headers (04-tui-layout §5.3).
func colorizeDiff(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "@@"):
			b.WriteString(g.diffHunk.Render(line))
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"),
			strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "new file"), strings.HasPrefix(line, "deleted file"),
			strings.HasPrefix(line, "rename "), strings.HasPrefix(line, "similarity "):
			b.WriteString(g.diffHeader.Render(line))
		case strings.HasPrefix(line, "+"):
			b.WriteString(g.diffAdd.Render(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(g.diffDel.Render(line))
		default:
			b.WriteString(line)
		}
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// diffCmd fetches the diff for the current selection (single-flight:
// starting a new one cancels the previous — 03 §7).
func diffCmd(runner *git.ExecRunner, repo *model.Repo, mode git.DiffMode, stat bool, path, commit string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		out, err := git.Diff(ctx, runner, repo.Path, git.DiffOptions{
			Mode: mode, Stat: stat, Path: path, Commit: commit,
		})
		return diffLoadedMsg{repo: repo.ID, content: out, err: err}
	}
}

type diffLoadedMsg struct {
	repo    string
	content string
	err     error
}

var _ = context.Background
