package ui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// recentModal is the ctrl+r recent-repos menu: jump to a previously opened
// repo (lazygit-style recent repos, 04 §3.1).
type recentModal struct {
	app *appModel
	l   list.Model
}

func newRecentModal(app *appModel, items []list.Item) *recentModal {
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "recent repos"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	return &recentModal{app: app, l: l}
}

func (r *recentModal) render(width int) string {
	if width > 60 {
		width = 60
	}
	return lipglossBox(r.l.View(), width)
}

func (r *recentModal) handleKey(msg tea.KeyMsg) (close bool, cmd tea.Cmd) {
	if r.l.FilterState() == list.Filtering {
		var c tea.Cmd
		r.l, c = r.l.Update(msg)
		return false, c
	}
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "?"))):
		return true, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if it, ok := r.l.SelectedItem().(recentItem); ok {
			r.app.selectedID = it.id
			r.app.appState.touchRecent(it.id)
			r.app.listFocused = true
			r.app.syncSelection()
			r.app.refreshFocused()
			return true, nil
		}
		return false, nil
	default:
		var c tea.Cmd
		r.l, c = r.l.Update(msg)
		return false, c
	}
}
