package ui

import (
	"strings"
	"testing"

	"github.com/AarnoStormborn/tree-trunk/internal/config"
	"github.com/AarnoStormborn/tree-trunk/internal/model"
	"github.com/AarnoStormborn/tree-trunk/internal/state"
)

// newTestModel builds an appModel with one selected repo for render tests.
func newTestModel(t *testing.T) appModel {
	t.Helper()
	cfg := config.Defaults()
	store := state.NewStore()
	store.Upsert(&model.Repo{
		ID: "/r", Name: "repo", Path: "/r", Branch: "main",
		Status: model.RepoStatus{Unstaged: 1, Files: []model.StatusFile{{X: ' ', Y: 'M', Path: "f.txt"}}},
		Worktrees: []model.Worktree{{Path: "/r", Branch: "main", IsMain: true},
			{Path: "/wt/feat", Branch: "feat", Dirty: true}},
	})
	store.SetLifecycle("/r", model.StateFresh, nil)

	actions := newWorktreeActions("git", store)
	m := newAppModel(&cfg, store, nil, actions)
	m.selectedID = "/r"
	m.listFocused = false
	m.tab = tabStatus
	m.width = 120
	m.height = 40
	m.layout()

	items := itemsFromStore(store, m.expanded)
	m.list.SetItems(items)
	m.list.SetSize(m.leftWidth(), m.height-9)
	m.status.repo = store.Get("/r")
	m.status.rebuildFiles()
	m.statusText = "refreshed 1 repos"
	return m
}

func TestViewHasFrameAndSplit(t *testing.T) {
	m := newTestModel(t)
	v := m.View()
	lines := strings.Split(v, "\n")

	if len(lines) == 0 || !strings.HasPrefix(lines[0], "╭") {
		t.Fatalf("frame top missing: %q", firstLine(v))
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") {
		t.Fatalf("frame bottom missing: %q", lines[len(lines)-1])
	}

	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"tree-trunk", // header title
		"repos (1)",  // sidebar header
		"repo",       // main repo header
		"On branch",  // status content
		"M f.txt",    // status file row
		"status",     // tab bar
		"refreshed",  // footer status
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("render missing %q\n%s", want, joined)
		}
	}
}

func TestViewFullscreenHidesSidebar(t *testing.T) {
	m := newTestModel(t)
	m.fullscreen = true
	m.layout()
	v := m.View()
	if strings.Contains(v, "repos (1)") {
		t.Fatalf("sidebar header visible in fullscreen:\n%s", v)
	}
	if !strings.Contains(v, "repo") {
		t.Fatal("main content missing in fullscreen")
	}
}

func TestViewWorktreesTab(t *testing.T) {
	m := newTestModel(t)
	m.tab = tabWorktrees
	m.wt.repo = m.store.Get("/r")
	m.wt.hasFocus = true
	v := m.View()
	joined := strings.Join(strings.Split(v, "\n"), "\n")
	for _, want := range []string{"feat", "main", "worktrees"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("worktrees tab missing %q", want)
		}
	}
}

func TestViewModalOverlays(t *testing.T) {
	m := newTestModel(t)
	m.modal = newCreateForm(m.store.Get("/r"), m.cfg.Worktrees.Directory)
	v := m.View()
	if !strings.Contains(v, "New worktree") {
		t.Fatalf("modal not rendered:\n%s", v)
	}
}

func firstLine(v string) string {
	if i := strings.Index(v, "\n"); i >= 0 {
		return v[:i]
	}
	return v
}
