// Package ui is the bubbletea application layer (D2).
// M1: repo list (left) + status detail (right); per-repo refresh with
// dedup driven by selection; event bridge from the state store.
package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/harshsingh/tree-trunk/internal/config"
	"github.com/harshsingh/tree-trunk/internal/state"
)

// M1 keybinding registry subset (full registry: docs/design/04-tui-layout.md §3).
type keyMap struct {
	Quit     key.Binding
	Help     key.Binding
	Refresh  key.Binding
	Filter   key.Binding
	Select   key.Binding
	Suspend  key.Binding
	MoveDown key.Binding
	MoveUp   key.Binding
	Focus    key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.MoveDown, k.MoveUp, k.Select, k.Filter, k.Refresh, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp(), {k.Suspend, k.Focus}}
}

var m1Keys = keyMap{
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Refresh:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
	Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Select:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "focus status")),
	Suspend:  key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("ctrl+z", "suspend")),
	MoveDown: key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	MoveUp:   key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Focus:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus pane")),
}

type appModel struct {
	cfg       *config.Config
	store     *state.Store
	refresher *state.Refresher
	spinner   spinner.Model
	list      list.Model
	help      help.Model
	status    statusView
	events    chan state.Event

	scanning   bool
	quit       bool
	statusText string
	width      int
	height     int

	listFocused bool
	selectedID  string
}

// New returns the root model with an initial repo list.
func newAppModel(cfg *config.Config, store *state.Store, refresher *state.Refresher) appModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	l := list.New([]list.Item{}, newRepoItemDelegate(), 0, 0)
	l.Title = "repos"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{m1Keys.Quit, m1Keys.Refresh, m1Keys.Help}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{m1Keys.Suspend, m1Keys.Focus}
	}

	return appModel{
		cfg:         cfg,
		store:       store,
		refresher:   refresher,
		spinner:     sp,
		list:        l,
		help:        help.New(),
		events:      store.Subscribe(),
		statusText:  "scanning…",
		listFocused: true,
	}
}

func (m appModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, scanCmd(m.cfg, m.store, m.refresher), m.pollEvents()}
	// Optional periodic poll (09-config.md §1: refresh.poll_interval_ms).
	if m.cfg.Refresh.PollIntervalMS > 0 {
		cmds = append(cmds, pollTickCmd(m.cfg.Refresh.PollIntervalMS))
	}
	return tea.Batch(cmds...)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		m.help.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		switch {
		case key.Matches(msg, m1Keys.Quit):
			m.quit = true
			return m, tea.Quit
		case key.Matches(msg, m1Keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case key.Matches(msg, m1Keys.Suspend):
			return m, tea.Suspend
		case key.Matches(msg, m1Keys.Focus):
			m.listFocused = !m.listFocused
			return m, nil
		case key.Matches(msg, m1Keys.Refresh):
			m.statusText = "refreshing…"
			return m, refreshAllCmd(m.cfg, m.store, m.refresher, m.selectedID)
		case key.Matches(msg, m1Keys.Select):
			if it, ok := m.list.SelectedItem().(repoItem); ok {
				m.selectedID = it.repo.ID
				m.refreshFocused()
			}
			return m, nil
		}

	case scanDoneMsg:
		m.scanning = false
		m.statusText = msg.status
		items := itemsFromStore(m.store)
		cur := m.selectedID
		cmd := m.list.SetItems(items)
		if cur != "" {
			m.list.Select(indexOfID(items, cur))
		}
		m.refreshFocused()
		return m, tea.Batch(cmd, refreshAllCmd(m.cfg, m.store, m.refresher, m.selectedID))

	case refreshDoneMsg:
		m.statusText = msg.status
		return m, nil

	case storeEventMsg:
		return m, m.handleStoreEvent(msg.e)

	case pollTickMsg:
		return m, pollTickCmd(m.cfg.Refresh.PollIntervalMS)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.scanning {
			m.statusText = m.spinner.View() + " scanning for repos…"
		}
		return m, cmd
	}

	// List is focused by default; otherwise the status pane swallows keys.
	if !m.listFocused {
		return m, nil
	}

	prev := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	// Selection change → refresh the newly focused repo (architecture §3.4).
	if m.list.Index() != prev {
		if it, ok := m.list.SelectedItem().(repoItem); ok {
			m.selectedID = it.repo.ID
			m.refreshFocused()
		}
	}
	return m, cmd
}

func (m *appModel) refreshFocused() {
	if m.selectedID == "" {
		m.status.repo = nil
		return
	}
	m.status.repo = m.store.Get(m.selectedID)
	ctx := context.Background()
	go m.refresher.RefreshOne(ctx, m.selectedID)
	m.statusText = "refreshing…"
}

func (m *appModel) handleStoreEvent(e state.Event) tea.Cmd {
	var cmds []tea.Cmd
	switch e.Kind {
	case state.EventRepoUpdated, state.EventRefreshFinished:
		// Refresh rows + the status pane from the store.
		if m.selectedID != "" {
			m.status.repo = m.store.Get(m.selectedID)
		}
		cur := m.selectedID
		items := itemsFromStore(m.store)
		cmd := m.list.SetItems(items)
		cmds = append(cmds, cmd)
		if cur != "" {
			m.list.Select(indexOfID(items, cur))
		}
		if n := m.refresher.RefreshingCount(); n > 0 {
			m.statusText = fmt.Sprintf("refreshing %d repos…", n)
		} else if m.statusText == "refreshing…" || m.statusText == "refreshing" {
			m.statusText = "up to date"
		}
	}
	cmds = append(cmds, m.pollEvents())
	return tea.Batch(cmds...)
}

// pollEvents bridges the state store event channel into the tea loop.
func (m appModel) pollEvents() tea.Cmd {
	return func() tea.Msg {
		e := <-m.events
		return storeEventMsg{e: e}
	}
}

func pollTickCmd(intervalMS int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(time.Duration(intervalMS) * time.Millisecond)
		return pollTickMsg{}
	}
}

func (m appModel) View() string {
	if m.quit {
		return ""
	}
	left := m.list.View()
	right := m.status.render()

	// Split layout: 40% list / 60% status (04-tui-layout.md §1).
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(m.leftWidth()).Padding(0, 1).Render(left),
		lipgloss.NewStyle().Width(m.rightWidth()).Padding(0, 1).BorderLeft(true).Render(right),
	)

	var statusLine string
	if m.scanning {
		statusLine = dimStyle.Render(m.statusText)
	} else {
		statusLine = m.statusText
	}

	helpLine := m.help.View(m1Keys)
	status := lipgloss.NewStyle().Padding(0, 1).Render(statusLine)

	return lipgloss.JoinVertical(lipgloss.Left,
		body,
		helpLine,
		status,
	)
}

func (m *appModel) layout() {
	listW := m.leftWidth()
	statusW := m.rightWidth()
	m.list.SetSize(listW, m.height-4)
	m.status.width = statusW
	m.status.height = m.height - 4
}

func (m *appModel) leftWidth() int {
	if m.width <= 0 {
		return 0
	}
	w := m.width * 40 / 100
	if w < 20 {
		w = 20
	}
	return w
}

func (m *appModel) rightWidth() int {
	if m.width <= 0 {
		return 0
	}
	w := m.width - m.leftWidth() - 1 // 1 for the border
	if w < 10 {
		w = 10
	}
	return w
}

// refreshAllCmd runs a full refresh on a worker so the UI stays responsive.
func refreshAllCmd(cfg *config.Config, store *state.Store, refresher *state.Refresher, focusID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if focusID != "" {
			refresher.RefreshOne(ctx, focusID)
		}
		refresher.RefreshAll(ctx)
		n := len(store.List())
		return refreshDoneMsg{status: fmt.Sprintf("refreshed %d repos", n)}
	}
}

// Run starts the bubbletea program.
func Run(ctx context.Context, cfg *config.Config, store *state.Store, refresher *state.Refresher) error {
	p := tea.NewProgram(newAppModel(cfg, store, refresher), tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return ctx.Err()
}
