package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/harshsingh/tree-trunk/internal/config"
	"github.com/harshsingh/tree-trunk/internal/discover"
	"github.com/harshsingh/tree-trunk/internal/git"
	"github.com/harshsingh/tree-trunk/internal/model"
	"github.com/harshsingh/tree-trunk/internal/state"
)

// scanDoneMsg reports a completed scan.
type scanDoneMsg struct {
	status string
}

// refreshDoneMsg reports a completed full refresh.
type refreshDoneMsg struct {
	status string
}

// storeEventMsg bridges one state-store event into the tea loop.
type storeEventMsg struct {
	e state.Event
}

// pollTickMsg fires on the configured refresh.poll_interval_ms cadence.
type pollTickMsg struct{}

// scanCmd runs the discovery scan on a worker and upserts repos into the
// store (docs/design/01-architecture.md §5).
func scanCmd(cfg *config.Config, store *state.Store, refresher *state.Refresher) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		gitPath, err := git.LookPath()
		if err != nil {
			return scanDoneMsg{status: "scan error: " + err.Error()}
		}
		roots := discover.Roots(cfg.Home, cfg.ScanRoots, cfg.Discover.ScanRoots, cfg.Discover.ScanHome && !cfg.NoScan)
		if cfg.NoScan {
			roots = nil
		}

		store.SetScan(true)
		defer store.SetScan(false)

		runner := git.NewExecRunner(gitPath)
		ids := map[string]struct{}{} // unique repo count (linked worktrees fold in)

		onHit := func(hit discover.Hit) error {
			repo, err := git.Resolve(ctx, runner, hit.Path)
			if err != nil {
				return nil // not a repo anymore / unreadable: skip
			}
			store.Upsert(repo)
			ids[repo.ID] = struct{}{}
			return nil
		}

		// Explicit --repo flags are always scanned (09-config.md §2.2.1).
		for _, p := range cfg.Repos {
			repo, err := git.Resolve(ctx, runner, p)
			if err != nil {
				continue
			}
			store.Upsert(repo)
			ids[repo.ID] = struct{}{}
		}

		opts := discover.Options{
			Roots:           roots,
			MaxDepth:        cfg.Discover.MaxDepth,
			Ignore:          cfg.Discover.Ignore,
			IncludeBare:     cfg.Discover.IncludeBare,
			FollowSymlinks:  cfg.Discover.FollowSymlinks,
			Hidden:          cfg.Discover.HiddenDirs,
			HiddenPeekDepth: cfg.Discover.HiddenPeekDepth,
		}
		if err := discover.Scanner(ctx, opts, onHit); err != nil {
			return scanDoneMsg{status: "scan error: " + err.Error()}
		}

		count := len(ids)
		plural := "repos"
		if count == 1 {
			plural = "repo"
		}
		return scanDoneMsg{status: statusForScan(count, plural)}
	}
}

func statusForScan(count int, plural string) string {
	if count == 0 {
		return "no repos found — press R to re-scan, or pass --repo PATH"
	}
	return "found " + itoa(count) + " " + plural
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// repoItem is one repo row in the list. Title renders the token row
// (docs/design/02-data-model.md §3).
type repoItem struct {
	repo *model.Repo
}

func (i repoItem) Title() string       { return renderRepoRow(i.repo) }
func (i repoItem) Description() string { return i.repo.GitDir }
func (i repoItem) FilterValue() string { return i.repo.Name + " " + i.repo.GitDir }

// renderRepoRow builds: [icon] name  branch  ↑2↓1  ~3 +1  (N wt)
func renderRepoRow(r *model.Repo) string {
	var s string

	icon := "·"
	switch r.Lifecycle {
	case model.StateError:
		icon = "e"
	case model.StateRefreshing:
		icon = "↻"
	case model.StateFresh, model.StateStale:
		if r.Status.Conflicts > 0 {
			icon = "!"
		} else if r.Status.Dirty() {
			icon = "~"
		} else {
			icon = "✓"
		}
	}

	s += icon + " " + r.Name

	branch := r.Branch
	if branch == "" {
		branch = "HEAD"
	}
	s += " " + dimStyle.Render(branch)

	if r.Status.Ahead > 0 || r.Status.Behind > 0 {
		ab := ""
		if r.Status.Ahead > 0 {
			ab += "↑" + itoa(r.Status.Ahead)
		}
		if r.Status.Behind > 0 {
			ab += "↓" + itoa(r.Status.Behind)
		}
		s += " " + dimStyle.Render(ab)
	}

	if sum := r.Status.Summary(); sum != "" {
		s += " " + sum
	}

	if n := len(r.Worktrees); n > 0 {
		s += " " + dimStyle.Render("("+itoa(n)+" wt)")
	}
	return s
}

// itemsFromStore renders the store's repos as list items.
func itemsFromStore(s *state.Store) []list.Item {
	repos := s.List()
	items := make([]list.Item, 0, len(repos))
	for _, r := range repos {
		items = append(items, repoItem{repo: r})
	}
	return items
}

func indexOfID(items []list.Item, id string) int {
	for i, it := range items {
		if r, ok := it.(repoItem); ok && r.repo.ID == id {
			return i
		}
	}
	return 0
}

func newRepoItemDelegate() list.ItemDelegate {
	return list.NewDefaultDelegate()
}

var (
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)
