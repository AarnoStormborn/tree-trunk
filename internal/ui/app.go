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
	Quit       key.Binding
	Help       key.Binding
	Refresh    key.Binding
	Filter     key.Binding
	Select     key.Binding
	Suspend    key.Binding
	MoveDown   key.Binding
	MoveUp     key.Binding
	Focus      key.Binding
	Expand     key.Binding
	Collapse   key.Binding
	TabNext    key.Binding
	TabPrev    key.Binding
	Tab1       key.Binding
	Tab2       key.Binding
	Tab3       key.Binding
	Tab4       key.Binding
	Mode       key.Binding
	Stat       key.Binding
	Copy       key.Binding
	PageDown   key.Binding
	PageUp     key.Binding
	Fullscreen key.Binding
	Recent     key.Binding
	New        key.Binding
	Delete     key.Binding
	Open       key.Binding
	Lock       key.Binding
	Prune      key.Binding
	Confirm    key.Binding
	Cancel     key.Binding
	NextF      key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.MoveDown, k.MoveUp, k.Select, k.New, k.Delete, k.Filter, k.Refresh, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp(), {k.Suspend, k.Focus, k.TabNext, k.TabPrev}}
}

var m2Keys = keyMap{
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Refresh:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
	Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Select:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "focus")),
	Suspend:    key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("ctrl+z", "suspend")),
	MoveDown:   key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	MoveUp:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Focus:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus pane")),
	Expand:     key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "expand")),
	Collapse:   key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "collapse")),
	TabNext:    key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next tab")),
	TabPrev:    key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev tab")),
	Tab1:       key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "status")),
	Tab2:       key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "worktrees")),
	Tab3:       key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "log")),
	Tab4:       key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "diff")),
	Mode:       key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "diff mode")),
	Stat:       key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "stat/raw")),
	Copy:       key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy")),
	PageDown:   key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
	PageUp:     key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
	Fullscreen: key.NewBinding(key.WithKeys("+", "_"), key.WithHelp("+/_", "fullscreen")),
	Recent:     key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "recent repos")),
	New:        key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new worktree")),
	Delete:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete worktree")),
	Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open (copy path)")),
	Lock:       key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "lock/unlock")),
	Prune:      key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "prune")),
	Confirm:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	Cancel:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	NextF:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
}

// tab IDs for the right pane.
const (
	tabStatus = iota
	tabWorktrees
	tabLog
	tabDiff
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
	log        logView
	diff       diffView
	diffCommit string // selected commit for DiffCommit mode
	scanning   bool
	quit       bool
	statusText string
	width      int
	height     int

	listFocused bool
	selectedID  string

	fullscreen bool
	toast      string
	toastUntil time.Time
	appState   stateFile
	bodyH      int
}

// New returns the root model.
func newAppModel(cfg *config.Config, store *state.Store, refresher *state.Refresher, actions *worktreeActions) appModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	l := list.New([]list.Item{}, newRepoItemDelegate(), 0, 0)
	l.Title = ""
	l.SetShowStatusBar(true)
	l.SetShowHelp(false) // one legend only — the app footer (currentHelp)
	l.SetFilteringEnabled(true)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{m2Keys.Quit, m2Keys.Refresh, m2Keys.Help}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{m2Keys.Suspend, m2Keys.Focus}
	}

	return appModel{
		log:         newLogView(),
		diff:        diffView{mode: git.DiffWorking},
		cfg:         cfg,
		store:       store,
		refresher:   refresher,
		actions:     actions,
		spinner:     sp,
		list:        l,
		help:        help.New(),
		appState:    loadState(cfg.Home),
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
		m.help.Width = m.contentWidth()
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
			return m, tea.Batch(saveStateCmd(m.cfg.Home, m.appState), tea.Quit)
		case key.Matches(msg, m2Keys.Help):
			m.modal = newHelpModal(helpBindings())
			return m, nil
		case key.Matches(msg, m2Keys.Fullscreen):
			m.fullscreen = !m.fullscreen
			m.layout()
			return m, nil
		case key.Matches(msg, m2Keys.Recent):
			m.modal = m.recentModal()
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
			m.tab = (m.tab + 1) % 4
			return m, m.onTabChanged()
		case key.Matches(msg, m2Keys.TabPrev):
			m.tab = (m.tab + 3) % 4
			return m, m.onTabChanged()
		case key.Matches(msg, m2Keys.Tab1):
			m.tab = tabStatus
			return m, nil
		case key.Matches(msg, m2Keys.Tab2):
			m.tab = tabWorktrees
			return m, m.reloadWorktrees()
		case key.Matches(msg, m2Keys.Tab3):
			m.tab = tabLog
			return m, m.loadLog(false)
		case key.Matches(msg, m2Keys.Tab4):
			m.tab = tabDiff
			return m, m.loadDiff()
		}

		if !m.listFocused {
			switch m.tab {
			case tabWorktrees:
				return m.updateWorktrees(msg)
			case tabLog:
				return m.updateLog(msg)
			case tabDiff:
				return m.updateDiff(msg)
			case tabStatus:
				return m.updateStatus(msg)
			}
		}
		return m.updateList(msg)

	case logPageMsg:
		if msg.repo != m.selectedID {
			return m, nil // stale page for a repo we navigated away from
		}
		m.log.loading = false
		if msg.err != nil {
			return m, setToast(&m, "✗ log: "+wtErrorText(msg.err))
		}
		m.log.hasMore = len(msg.commits) == m.logPageSize()
		m.log.setCommits(msg.commits, true)
		return m, nil

	case diffLoadedMsg:
		if msg.repo != m.selectedID {
			return m, nil // stale diff (single-flight, cancel-previous)
		}
		m.diff.loading = false
		switch e := msg.err.(type) {
		case git.ErrUntrackedFile:
			m.diff.content = ""
			m.diff.err = "untracked file — no diff to show"
		case nil:
			m.diff.content = msg.content
			m.diff.err = ""
		default:
			m.diff.content = ""
			m.diff.err = wtErrorText(e)
		}
		return m, nil

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
				m.appState.touchRecent(it.repo.ID)
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

	case toastExpireMsg:
		m.toast = ""
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.scanning {
			m.statusText = m.spinner.View() + " scanning for repos…"
		}
		return m, cmd
	}

	if !m.listFocused {
		switch m.tab {
		case tabWorktrees:
			return m.updateWorktrees(msg)
		case tabLog:
			return m.updateLog(msg)
		case tabDiff:
			return m.updateDiff(msg)
		case tabStatus:
			return m.updateStatus(msg)
		}
	}
	return m.updateList(msg)
}

// --- right-pane key handlers (status / log / diff) ---

func (m *appModel) updateStatus(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(k, m2Keys.MoveDown):
		if m.status.cursor < len(m.status.files)-1 {
			m.status.cursor++
		}
	case key.Matches(k, m2Keys.MoveUp):
		if m.status.cursor > 0 {
			m.status.cursor--
		}
	case key.Matches(k, m2Keys.Select):
		if f := m.status.selectedFile(); f != nil {
			m.diff.path = f.Path
			m.diff.mode = git.DiffFileWorking
			if f.Staged() && !f.Unstaged() {
				m.diff.mode = git.DiffFileStaged
			}
			m.diff.stat = false
			m.tab = tabDiff
			return m, m.loadDiff()
		}
	}
	return m, nil
}

func (m *appModel) updateLog(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(k, m2Keys.MoveDown):
		m.log.list.CursorDown()
		if m.log.hasMore && m.log.list.Index() >= len(m.log.commits)-3 && !m.log.loading {
			return m, m.loadLog(true)
		}
	case key.Matches(k, m2Keys.MoveUp):
		m.log.list.CursorUp()
	case key.Matches(k, m2Keys.Select):
		if it, ok := m.log.list.SelectedItem().(commitItem); ok {
			m.diffCommit = it.c.Hash
			m.diff.mode = git.DiffCommit
			m.diff.path = ""
			m.diff.stat = false
			m.tab = tabDiff
			return m, m.loadDiff()
		}
	case key.Matches(k, m2Keys.New):
		// w → create worktree from the selected commit (04 §3.3).
		if it, ok := m.log.list.SelectedItem().(commitItem); ok {
			f := newCreateForm(m.log.repo, m.cfg.Worktrees.Directory)
			f.base.SetValue(it.c.Hash)
			m.modal = f
		}
	case key.Matches(k, m2Keys.Copy):
		if it, ok := m.log.list.SelectedItem().(commitItem); ok {
			return m, copyText(it.c.Hash)
		}
	case key.Matches(k, m2Keys.PageDown):
		m.log.list.CursorDown()
		for i := 0; i < 10 && m.log.list.Index() < len(m.log.commits)-1; i++ {
			m.log.list.CursorDown()
		}
	case key.Matches(k, m2Keys.PageUp):
		for i := 0; i < 10 && m.log.list.Index() > 0; i++ {
			m.log.list.CursorUp()
		}
	}
	return m, nil
}

func (m *appModel) updateDiff(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(k, m2Keys.Mode):
		m.diff.mode = (m.diff.mode + 1) % 3 // working → staged → vs main
		m.diff.path = ""
		m.diff.stat = false
		return m, m.loadDiff()
	case key.Matches(k, m2Keys.Stat):
		m.diff.stat = !m.diff.stat
		return m, m.loadDiff()
	case key.Matches(k, m2Keys.Copy):
		return m, copyText(m.diff.path)
	}
	return m, nil
}

// onTabChanged loads content when entering log/diff tabs.
func (m *appModel) onTabChanged() tea.Cmd {
	switch m.tab {
	case tabLog:
		return m.loadLog(false)
	case tabDiff:
		return m.loadDiff()
	}
	return nil
}

// loadLog fetches the first page (or the next page when append_).
func (m *appModel) loadLog(append_ bool) tea.Cmd {
	if m.selectedID == "" {
		return nil
	}
	repo := m.store.Get(m.selectedID)
	if repo == nil {
		return nil
	}
	m.log.repo = repo
	m.log.loading = true
	skip := 0
	if append_ {
		skip = len(m.log.commits)
	}
	return loadMoreCmd(m.actions.runner, repo, skip, m.logPageSize())
}

func (m *appModel) logPageSize() int { return 200 }

// loadDiff fetches the diff for the current diff-view state.
func (m *appModel) loadDiff() tea.Cmd {
	if m.selectedID == "" {
		return nil
	}
	repo := m.store.Get(m.selectedID)
	if repo == nil {
		return nil
	}
	m.diff.repo = repo
	m.diff.loading = true
	commit := m.diffCommit
	if m.diff.mode != git.DiffCommit {
		commit = ""
	}
	return diffCmd(m.actions.runner, repo, m.diff.mode, m.diff.stat, m.diff.path, commit)
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
		if it, ok := m.list.SelectedItem().(repoItem); ok {
			m.appState.touchRecent(it.repo.ID)
		}
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
	case *helpModal:
		if modal.handleKey(msg) {
			m.modal = nil
		}
		return m, nil
	case *recentModal:
		close_, cmd := modal.handleKey(msg)
		if close_ {
			m.modal = nil
		}
		if cmd != nil {
			return m, cmd
		}
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
			return m, setToast(m, "✗ "+wtErrorText(msg.err))
		}
	default:
		var text string
		switch msg.op {
		case "add":
			text = "✓ worktree created"
		case "remove":
			text = "✓ worktree removed"
		case "lock":
			text = "✓ lock toggled"
		case "prune":
			text = "✓ pruned"
		case "open":
			text = "✓ path copied to clipboard"
		}
		return m, tea.Batch(setToast(m, text), m.reloadWorktrees())
	}
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
		m.log.repo = nil
		m.diff.repo = nil
		return
	}
	r := m.store.Get(m.selectedID)
	m.status.repo = r
	m.status.rebuildFiles()
	m.status.hasFocus = !m.listFocused && m.tab == tabStatus
	m.wt.repo = r
	m.wt.hasFocus = !m.listFocused && m.tab == tabWorktrees
	m.log.repo = r
	m.diff.repo = r
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

// View renders the full app frame: header / sidebar+main body / footer
// (help + status). Visual boundaries: outer rounded frame, horizontal rules
// between sections, a vertical border between the sidebar and the main
// section, and a content box inside main.
func (m appModel) View() string {
	if m.quit {
		return ""
	}

	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()

	frame := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	if m.width > 0 {
		frame = frame.Width(m.width - 2) // account for the border columns
	}
	return frame.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
}

// contentWidth is the usable width inside the app frame (border + padding).
func (m appModel) contentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return m.width - 4
}

// renderHeader is the app title bar: "tree-trunk" + version on the right.
func (m appModel) renderHeader() string {
	titleStr := "tree-trunk"
	title := g.title.Render(titleStr)
	info := g.dim.Render("v" + appVersion)
	head := lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		lipgloss.NewStyle().Width(m.contentWidth()-len(titleStr)-1).Align(lipgloss.Right).Render(info),
	)
	return head + "\n" + hrule(m.contentWidth())
}

// renderBody splits the screen into the sidebar (repo list) and the main
// section (tabs + content). Both boxes share the body height so the sidebar
// boundary spans the full column; the main section is padded away from the
// boundary.
func (m appModel) renderBody() string {
	var body string
	if m.fullscreen {
		body = lipgloss.NewStyle().Width(m.rightWidth()).Height(m.bodyH).Padding(0, 2).Render(m.renderMain())
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(m.leftWidth()).Height(m.bodyH).
				BorderRight(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(g.borderColor).
				Padding(0, 1).
				Render(m.renderSidebar()),
			lipgloss.NewStyle().Width(m.rightWidth()).Height(m.bodyH).Padding(0, 2).Render(m.renderMain()),
		)
	}

	// Modal overlay on top of the split.
	if m.modal != nil {
		body = m.modal.render(m.width)
	}
	return body
}

// renderSidebar is the repo list pane with its own header.
func (m appModel) renderSidebar() string {
	n := len(m.store.List())
	head := g.title.Render("repos") + " " + g.dim.Render("("+itoa(n)+")")
	listView := m.list.View()
	return head + "\n" + listView
}

// renderMain is the tabbed content section with a repo header and an
// unpadded content area (no boxed boundary).
func (m appModel) renderMain() string {
	repoName := "—"
	repoBranch := ""
	if r := m.store.Get(m.selectedID); r != nil {
		repoName = r.Name
		repoBranch = r.Branch
		if repoBranch == "" {
			repoBranch = "HEAD"
		}
	}
	head := g.title.Render(repoName) + " " + g.dim.Render(repoBranch)

	// Tab bar: consistent tabs, active one highlighted with accent bg.
	tabs := ""
	for i, name := range []string{"status", "worktrees", "log", "diff"} {
		style := lipgloss.NewStyle().Padding(0, 1).MarginRight(1)
		if i == m.tab {
			style = style.Bold(true).Foreground(g.tabActiveFg).Background(g.accentColor)
		} else {
			style = style.Foreground(g.dimColor)
		}
		tabs += style.Render(name)
	}

	content := ""
	switch m.tab {
	case tabStatus:
		content = m.status.render()
	case tabWorktrees:
		content = m.wt.render()
	case tabLog:
		content = m.log.render()
	case tabDiff:
		content = m.diff.render()
	}

	return head + "\n" + tabs + "\n" + content
}

// renderFooter is the help + status area with a horizontal rule between.
// The help reflects the CURRENT context (list vs the active tab) — one
// legend, not two.
func (m appModel) renderFooter() string {
	helpLine := m.help.View(m.currentHelp())

	var statusLine string
	if m.toast != "" && time.Now().Before(m.toastUntil) {
		statusLine = m.toast
	} else if m.scanning {
		statusLine = g.dim.Render(m.statusText)
	} else {
		statusLine = m.statusText
	}
	status := lipgloss.NewStyle().Padding(0, 1).Render(statusLine)

	return helpLine + "\n" + hrule(m.contentWidth()) + "\n" + status
}

// currentHelp returns a COMPACT legend for the focused context: the repo
// list when the sidebar is focused, or the active tab's keys otherwise.
// One legend only (the sidebar has no legend of its own).
func (m appModel) currentHelp() contextHelp {
	glob := []key.Binding{
		kb("R", "refresh"), kb("?", "help"), kb("q", "quit"),
	}
	tabs := kb("1-4/[]", "tabs")
	if m.listFocused {
		return contextHelp{keys: append([]key.Binding{
			kb("j/k", "move"), kb("enter", "focus"), kb("n", "new wt"),
			kb("d", "delete wt"), kb("L", "lock"), kb("l/→", "expand"),
			kb("/", "filter"),
		}, glob...)}
	}
	var ctx []key.Binding
	switch m.tab {
	case tabStatus:
		ctx = []key.Binding{kb("j/k", "move"), kb("enter", "file diff"), tabs}
	case tabWorktrees:
		ctx = []key.Binding{kb("j/k", "move"), kb("n", "new"), kb("d", "delete"),
			kb("L", "lock"), kb("P", "prune"), kb("o", "open"), tabs}
	case tabLog:
		ctx = []key.Binding{kb("j/k", "move"), kb("enter", "commit diff"),
			kb("w", "wt from commit"), kb("c", "copy hash"), tabs}
	case tabDiff:
		ctx = []key.Binding{kb("j/k", "scroll"), kb("m", "mode"),
			kb("p", "stat/raw"), kb("c", "copy path"), tabs}
	}
	return contextHelp{keys: append(ctx, glob...)}
}

// kb builds a display-only binding for the legend.
func kb(keys, help string) key.Binding {
	return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, help))
}

func (m *appModel) layout() {
	// Frame budget: rounded border (2) + header (2) + footer (3: help,
	// rule, status).
	bodyH := m.height - 7
	if bodyH < 5 {
		bodyH = 5
	}
	m.bodyH = bodyH
	// Sidebar border (1) + sidebar padding (2) + main padding (4).
	m.list.SetSize(m.leftWidth()-2, bodyH-2)
	m.status.width = m.rightWidth() - 4
	m.status.height = bodyH - 2 // main header + tab bar
	m.wt.width = m.rightWidth() - 4
	m.wt.height = bodyH - 2
	m.log.list.SetSize(m.rightWidth()-4, bodyH-3)
	m.log.width = m.rightWidth() - 4
	m.log.height = bodyH - 2
	m.diff.width = m.rightWidth() - 4
	m.diff.height = bodyH - 3
}

func (m *appModel) leftWidth() int {
	if m.fullscreen || m.width <= 0 {
		return 0
	}
	w := m.contentWidth() * 33 / 100
	if w < 24 {
		w = 24
	}
	return w
}

func (m *appModel) rightWidth() int {
	if m.width <= 0 {
		return 0
	}
	if m.fullscreen {
		return m.contentWidth()
	}
	// sidebar border (1) + sidebar padding (2) + main padding (4)
	w := m.contentWidth() - m.leftWidth() - 7
	if w < 20 {
		w = 20
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

// appVersion is the build version, injected from main at startup.
var appVersion = "dev"

// Run starts the bubbletea program. version is the build version ("" = dev).
func Run(ctx context.Context, cfg *config.Config, store *state.Store, refresher *state.Refresher, version string) error {
	if version != "" {
		appVersion = version
	}
	gitPath, err := git.LookPath()
	if err != nil {
		return err
	}
	actions := newWorktreeActions(gitPath, store)
	t := DefaultTheme()
	if cfg.Theme.Variant == "light" {
		t = LightTheme()
	}
	initStyles(t, cfg.Theme.Overrides)
	p := tea.NewProgram(newAppModel(cfg, store, refresher, actions), tea.WithAltScreen())
	_, err = p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return ctx.Err()
}

// contextHelp adapts a flat binding list to bubbles/help's interface.
type contextHelp struct{ keys []key.Binding }

func (c contextHelp) ShortHelp() []key.Binding  { return c.keys }
func (c contextHelp) FullHelp() [][]key.Binding { return [][]key.Binding{c.keys} }
