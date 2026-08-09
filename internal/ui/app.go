// Package ui is the bubbletea application layer (D2).
// M2: tabs (Status | Worktrees), worktree create/delete/lock/prune flows,
// confirm dialogs, repo-list worktree children.
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

	"github.com/AarnoStormborn/tree-trunk/internal/config"
	"github.com/AarnoStormborn/tree-trunk/internal/git"
	"github.com/AarnoStormborn/tree-trunk/internal/state"
)

// M2 keybinding registry subset (full registry: docs/design/04-tui-layout.md §3).
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
	Expand   key.Binding
	Collapse key.Binding
	TabNext  key.Binding
	TabPrev  key.Binding
	Tab1     key.Binding
	Tab2     key.Binding
	New      key.Binding
	Delete   key.Binding
	Open     key.Binding
	Lock     key.Binding
	Prune    key.Binding
	Confirm  key.Binding
	Cancel   key.Binding
	NextF    key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.MoveDown, k.MoveUp, k.Select, k.New, k.Delete, k.Filter, k.Refresh, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp(), {k.Suspend, k.Focus, k.TabNext, k.TabPrev}}
}

var m2Keys = keyMap{
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Refresh:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
	Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Select:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "focus")),
	Suspend:  key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("ctrl+z", "suspend")),
	MoveDown: key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	MoveUp:   key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Focus:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus pane")),
	Expand:   key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "expand")),
	Collapse: key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "collapse")),
	TabNext:  key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next tab")),
	TabPrev:  key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev tab")),
	Tab1:     key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "status")),
	Tab2:     key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "worktrees")),
	New:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new worktree")),
	Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete worktree")),
	Open:     key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open (copy path)")),
	Lock:     key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "lock/unlock")),
	Prune:    key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "prune")),
	Confirm:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	NextF:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
}

// tab IDs for the right pane.
const (
	tabStatus = iota
	tabWorktrees
)

// modal is an overlay dialog (create form or confirmation).
type modal interface {
	render(width int) string
}

type appModel struct {
	cfg       *config.Config
	store     *state.Store
	refresher *state.Refresher
	actions   *worktreeActions
	spinner   spinner.Model
	list      list.Model
	help      help.Model
	status    statusView
	wt        worktreesView
	events    chan state.Event

	tab        int
	modal      modal
	expanded   map[string]bool
	scanning   bool
	quit       bool
	statusText string
	width      int
	height     int

	listFocused bool
	selectedID  string
}

// New returns the root model.
func newAppModel(cfg *config.Config, store *state.Store, refresher *state.Refresher, actions *worktreeActions) appModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	l := list.New([]list.Item{}, newRepoItemDelegate(), 0, 0)
	l.Title = "repos"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{m2Keys.Quit, m2Keys.Refresh, m2Keys.Help}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{m2Keys.Suspend, m2Keys.Focus}
	}

	return appModel{
		cfg:         cfg,
		store:       store,
		refresher:   refresher,
		actions:     actions,
		spinner:     sp,
		list:        l,
		help:        help.New(),
		events:      store.Subscribe(),
		expanded:    map[string]bool{},
		statusText:  "scanning…",
		listFocused: true,
	}
}

func (m appModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, scanCmd(m.cfg, m.store, m.refresher), m.pollEvents()}
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
		// Modal takes priority.
		if m.modal != nil {
			return m.updateModal(msg)
		}
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		switch {
		case key.Matches(msg, m2Keys.Quit):
			m.quit = true
			return m, tea.Quit
		case key.Matches(msg, m2Keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		case key.Matches(msg, m2Keys.Suspend):
			return m, tea.Suspend
		case key.Matches(msg, m2Keys.Focus):
			m.listFocused = !m.listFocused
			return m, nil
		case key.Matches(msg, m2Keys.Refresh):
			m.statusText = "refreshing…"
			return m, refreshAllCmd(m.cfg, m.store, m.refresher, m.selectedID)
		case key.Matches(msg, m2Keys.TabNext):
			m.tab = (m.tab + 1) % 2
			return m, nil
		case key.Matches(msg, m2Keys.TabPrev):
			m.tab = (m.tab + 1) % 2
			return m, nil
		case key.Matches(msg, m2Keys.Tab1):
			m.tab = tabStatus
			return m, nil
		case key.Matches(msg, m2Keys.Tab2):
			m.tab = tabWorktrees
			return m, m.reloadWorktrees()
		}

		if !m.listFocused && m.tab == tabWorktrees {
			return m.updateWorktrees(msg)
		}
		return m.updateList(msg)

	case scanDoneMsg:
		m.scanning = false
		m.statusText = msg.status
		items := itemsFromStore(m.store, m.expanded)
		cmd := m.list.SetItems(items)
		// Auto-select the first repo after the initial scan (review of the
		// M1 live pass: without this the detail pane stays empty until an
		// explicit selection).
		if m.selectedID == "" && len(items) > 0 {
			if it, ok := items[0].(repoItem); ok {
				m.selectedID = it.repo.ID
				m.list.Select(0)
			}
		} else if m.selectedID != "" {
			m.list.Select(indexOfID(items, m.selectedID))
		}
		m.syncSelection()
		return m, tea.Batch(cmd, refreshAllCmd(m.cfg, m.store, m.refresher, m.selectedID))

	case refreshDoneMsg:
		m.statusText = msg.status
		return m, nil

	case storeEventMsg:
		return m, m.handleStoreEvent(msg.e)

	case worktreeActionMsg:
		mm, cmd := m.handleAction(msg)
		return mm, cmd

	case wtReloadMsg:
		m.statusText = "worktrees updated"
		m.syncSelection()
		return m, nil

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

	if !m.listFocused && m.tab == tabWorktrees {
		return m.updateWorktrees(msg)
	}
	return m.updateList(msg)
}

// --- list pane ---

func (m *appModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	prev := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	// Expand/collapse repo rows (l/→, h/←).
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, m2Keys.Expand):
			if it, ok := m.list.SelectedItem().(repoItem); ok {
				m.expanded[it.repo.ID] = true
				items := itemsFromStore(m.store, m.expanded)
				m.list.SetItems(items)
				return m, cmd
			}
		case key.Matches(k, m2Keys.Collapse):
			if it, ok := m.list.SelectedItem().(repoItem); ok {
				delete(m.expanded, it.repo.ID)
				items := itemsFromStore(m.store, m.expanded)
				m.list.SetItems(items)
				return m, cmd
			}
		case key.Matches(k, m2Keys.Open):
			if it, ok := m.list.SelectedItem().(worktreeItem); ok {
				return m, openWorktree(it.wt.Path)
			}
			if it, ok := m.list.SelectedItem().(repoItem); ok {
				return m, openWorktree(it.repo.Path)
			}
		case key.Matches(k, m2Keys.Lock):
			if it, ok := m.list.SelectedItem().(worktreeItem); ok {
				return m, m.actions.lockCmd(it.repo, it.wt)
			}
		case key.Matches(k, m2Keys.Delete):
			if it, ok := m.list.SelectedItem().(worktreeItem); ok {
				m.modal = newDeleteConfirm(m.actions, it.repo, it.wt)
				return m, nil
			}
		case key.Matches(k, m2Keys.New):
			if it, ok := m.list.SelectedItem().(repoItem); ok {
				m.modal = newCreateForm(it.repo, m.cfg.Worktrees.Directory)
				return m, nil
			}
		case key.Matches(k, m2Keys.Select):
			if it, ok := m.list.SelectedItem().(worktreeItem); ok {
				m.tab = tabWorktrees
				m.wt.cursor = worktreeIndex(it.repo.Worktrees, it.wt.Path)
				m.wt.hasFocus = true
				m.listFocused = false
			} else if it, ok := m.list.SelectedItem().(repoItem); ok {
				m.selectedID = it.repo.ID
				m.listFocused = false
				m.syncSelection()
				m.refreshFocused()
			}
			return m, nil
		}
	}

	if m.list.Index() != prev {
		m.syncSelection()
	}
	return m, cmd
}

// --- worktrees pane ---

func (m *appModel) updateWorktrees(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, m2Keys.MoveDown):
			if m.wt.cursor < len(m.wt.repo.Worktrees)-1 {
				m.wt.cursor++
			}
			return m, nil
		case key.Matches(k, m2Keys.MoveUp):
			if m.wt.cursor > 0 {
				m.wt.cursor--
			}
			return m, nil
		case key.Matches(k, m2Keys.New):
			if m.wt.repo != nil {
				m.modal = newCreateForm(m.wt.repo, m.cfg.Worktrees.Directory)
			}
			return m, nil
		case key.Matches(k, m2Keys.Delete):
			if m.wt.repo != nil && m.wt.cursor < len(m.wt.repo.Worktrees) {
				wt := m.wt.repo.Worktrees[m.wt.cursor]
				if !wt.IsMain { // git refuses main removal; don't offer
					m.modal = newDeleteConfirm(m.actions, m.wt.repo, wt)
				}
			}
			return m, nil
		case key.Matches(k, m2Keys.Open):
			if m.wt.repo != nil && m.wt.cursor < len(m.wt.repo.Worktrees) {
				return m, openWorktree(m.wt.repo.Worktrees[m.wt.cursor].Path)
			}
		case key.Matches(k, m2Keys.Lock):
			if m.wt.repo != nil && m.wt.cursor < len(m.wt.repo.Worktrees) {
				wt := m.wt.repo.Worktrees[m.wt.cursor]
				if !wt.IsMain {
					return m, m.actions.lockCmd(m.wt.repo, wt)
				}
			}
		case key.Matches(k, m2Keys.Prune):
			if m.wt.repo != nil {
				m.modal = newPruneConfirm(m.actions, m.wt.repo)
			}
			return m, nil
		}
	}
	return m, nil
}

// --- modals ---

func (m *appModel) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch modal := m.modal.(type) {
	case *createForm:
		upd := modal.handleKey(msg, m.cfg.Worktrees.Directory)
		if upd.cancel {
			m.modal = nil
			return m, nil
		}
		if upd.done {
			m.modal = nil
			m.statusText = "creating worktree…"
			return m, m.actions.addCmd(modal.repo, modal.branch.Value(), modal.base.Value(), modal.path.Value())
		}
		m.modal = modal
		return m, nil
	case *confirmModal:
		switch {
		case key.Matches(msg, m2Keys.Confirm):
			cmd := modal.onConfirm()
			m.modal = nil
			m.statusText = "working…"
			return m, cmd
		case key.Matches(msg, m2Keys.Cancel):
			m.modal = nil
			return m, nil
		}
	}
	return m, nil
}

// --- actions ---

func (m *appModel) handleAction(msg worktreeActionMsg) (tea.Model, tea.Cmd) {
	m.syncSelection()
	switch {
	case msg.err != nil:
		switch e := msg.err.(type) {
		case *git.WorktreeDirtyError:
			// Two-step: safe remove was refused — offer explicit force.
			repo := m.store.Get(msg.repo)
			if repo != nil {
				if wt := findWorktree(repo, e.Path); wt != nil {
					m.modal = newForceDeleteConfirm(m.actions, repo, *wt)
				}
			}
			return m, nil
		default:
			m.statusText = "✗ " + wtErrorText(msg.err)
			return m, nil
		}
	default:
		switch msg.op {
		case "add":
			m.statusText = "✓ worktree created"
		case "remove":
			m.statusText = "✓ worktree removed"
		case "lock":
			m.statusText = "✓ lock toggled"
		case "prune":
			m.statusText = "✓ pruned"
		case "open":
			m.statusText = "✓ path copied to clipboard"
		}
	}
	return m, m.reloadWorktrees()
}

func (m *appModel) reloadWorktrees() tea.Cmd {
	if m.selectedID == "" {
		return nil
	}
	return wtReloadCmd(m.refresher, m.selectedID)
}

// --- shared ---

func (m *appModel) syncSelection() {
	if m.selectedID == "" {
		m.status.repo = nil
		m.wt.repo = nil
		return
	}
	r := m.store.Get(m.selectedID)
	m.status.repo = r
	m.wt.repo = r
	m.wt.hasFocus = !m.listFocused && m.tab == tabWorktrees
}

func (m *appModel) refreshFocused() {
	if m.selectedID == "" {
		return
	}
	ctx := context.Background()
	go m.refresher.RefreshOne(ctx, m.selectedID)
	m.statusText = "refreshing…"
}

func (m *appModel) handleStoreEvent(e state.Event) tea.Cmd {
	var cmds []tea.Cmd
	switch e.Kind {
	case state.EventRepoUpdated, state.EventRefreshFinished:
		m.syncSelection()
		cur := m.selectedID
		items := itemsFromStore(m.store, m.expanded)
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
	right := m.renderRight()

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(m.leftWidth()).Padding(0, 1).Render(left),
		lipgloss.NewStyle().Width(m.rightWidth()).Padding(0, 1).BorderLeft(true).Render(right),
	)

	// Modal overlay on top of the split.
	if m.modal != nil {
		overlay := m.modal.render(m.width)
		body = overlay
	}

	var statusLine string
	if m.scanning {
		statusLine = dimStyle.Render(m.statusText)
	} else {
		statusLine = m.statusText
	}

	helpLine := m.help.View(m2Keys)
	status := lipgloss.NewStyle().Padding(0, 1).Render(statusLine)

	return lipgloss.JoinVertical(lipgloss.Left,
		body,
		helpLine,
		status,
	)
}

func (m appModel) renderRight() string {
	// Tab bar.
	tabs := ""
	for i, name := range []string{"1 status", "2 worktrees"} {
		style := lipgloss.NewStyle().Padding(0, 1)
		if i == m.tab {
			style = style.Bold(true).Underline(true)
		}
		tabs += style.Render(name)
	}
	content := ""
	switch m.tab {
	case tabStatus:
		content = m.status.render()
	case tabWorktrees:
		content = m.wt.render()
	}
	return tabs + "\n" + content
}

func (m *appModel) layout() {
	listW := m.leftWidth()
	statusW := m.rightWidth()
	m.list.SetSize(listW, m.height-4)
	m.status.width = statusW
	m.status.height = m.height - 4
	m.wt.width = statusW
	m.wt.height = m.height - 4
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
	w := m.width - m.leftWidth() - 1
	if w < 10 {
		w = 10
	}
	return w
}

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
	gitPath, err := git.LookPath()
	if err != nil {
		return err
	}
	actions := newWorktreeActions(gitPath, store)
	p := tea.NewProgram(newAppModel(cfg, store, refresher, actions), tea.WithAltScreen())
	_, err = p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return ctx.Err()
}
