// Package ui is the bubbletea application layer (D2).
// M0 scope: left-pane repo list rendering scanned repos, spinner during
// scan, q/?/R// keys. The full layout (right pane, tabs) lands in M1.
package ui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/harshsingh/tree-trunk/internal/config"
	"github.com/harshsingh/tree-trunk/internal/state"
)

// M0 keybinding registry subset (full registry: docs/design/04-tui-layout.md §3).
type keyMap struct {
	Quit     key.Binding
	Help     key.Binding
	Refresh  key.Binding
	Filter   key.Binding
	Select   key.Binding
	Suspend  key.Binding
	MoveDown key.Binding
	MoveUp   key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.MoveDown, k.MoveUp, k.Select, k.Filter, k.Refresh, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp(), {k.Suspend}}
}

var m0Keys = keyMap{
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Refresh:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "re-scan")),
	Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Select:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Suspend:  key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("ctrl+z", "suspend")),
	MoveDown: key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	MoveUp:   key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
}

type appModel struct {
	cfg      *config.Config
	store    *state.Store
	gitPath  string
	spinner  spinner.Model
	list     list.Model
	help     help.Model
	scanning bool
	quit     bool
	status   string
	width    int
	height   int
}

// New returns the root model with an initial repo list.
func newAppModel(cfg *config.Config, store *state.Store, gitPath string) appModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	l := list.New([]list.Item{}, newRepoItemDelegate(), 0, 0)
	l.Title = "repos"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{m0Keys.Quit, m0Keys.Refresh, m0Keys.Help}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{m0Keys.Suspend}
	}

	return appModel{
		cfg:     cfg,
		store:   store,
		gitPath: gitPath,
		spinner: sp,
		list:    l,
		help:    help.New(),
		status:  "scanning…",
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, scanCmd(m.cfg, m.store, m.gitPath))
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
		m.help.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		switch {
		case key.Matches(msg, m0Keys.Quit):
			m.quit = true
			return m, tea.Quit
		case key.Matches(msg, m0Keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case key.Matches(msg, m0Keys.Suspend):
			return m, tea.Suspend
		case key.Matches(msg, m0Keys.Refresh):
			m.status = "rescanning…"
			return m, scanCmd(m.cfg, m.store, m.gitPath)
		case key.Matches(msg, m0Keys.Select):
			if it, ok := m.list.SelectedItem().(repoItem); ok {
				m.status = "selected: " + it.repo.Name + " (detail views land in M1)"
			}
			return m, nil
		}

	case scanDoneMsg:
		m.scanning = false
		m.status = msg.status
		items := itemsFromStore(m.store)
		cur := ""
		if it, ok := m.list.SelectedItem().(repoItem); ok {
			cur = it.repo.ID
		}
		cmd := m.list.SetItems(items)
		if cur != "" {
			m.list.Select(indexOfID(items, cur))
		}
		return m, cmd

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.scanning {
			m.status = m.spinner.View() + " scanning for repos…"
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m appModel) View() string {
	if m.quit {
		return ""
	}
	var statusLine string
	if m.scanning {
		statusLine = dimStyle.Render(m.status)
	} else {
		statusLine = m.status
	}

	helpLine := m.help.View(m0Keys)
	body := lipgloss.NewStyle().Padding(0, 1).Render(m.list.View())
	status := lipgloss.NewStyle().Padding(0, 1).Render(statusLine)

	return lipgloss.JoinVertical(lipgloss.Left,
		body,
		helpLine,
		status,
	)
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

// Run starts the bubbletea program.
func Run(ctx context.Context, cfg *config.Config, store *state.Store, gitPath string) error {
	p := tea.NewProgram(newAppModel(cfg, store, gitPath), tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return ctx.Err()
}
