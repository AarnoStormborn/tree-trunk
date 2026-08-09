package state

import (
	"testing"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

func mkRepo(id, name string) *model.Repo {
	return &model.Repo{ID: id, Name: name, Lifecycle: model.StateStale}
}

func TestUpsertAddsAndSorts(t *testing.T) {
	s := NewStore()
	s.Upsert(mkRepo("/b", "bravo"))
	s.Upsert(mkRepo("/a", "alpha"))
	s.Upsert(mkRepo("/c", "charlie"))

	got := s.List()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []string{"alpha", "bravo", "charlie"} {
		if got[i].Name != want {
			t.Fatalf("order[%d] = %s, want %s", i, got[i].Name, want)
		}
	}
}

func TestUpsertReplaceKeepsIdentityFields(t *testing.T) {
	s := NewStore()
	first := mkRepo("/r", "repo")
	first.Path = "/main/path"
	s.Upsert(first)

	// A refresh marks the repo fresh; a re-scan must not clobber that.
	s.SetLifecycle("/r", model.StateFresh, nil)

	second := mkRepo("/r", "repo")
	second.Path = "/linked/path" // linked-worktree resolution must not clobber
	s.Upsert(second)

	got := s.Get("/r")
	if got == nil {
		t.Fatal("repo missing")
	}
	if got.Path != "/main/path" {
		t.Fatalf("Path = %q, want first-seen /main/path", got.Path)
	}
	if got.Lifecycle != model.StateFresh {
		t.Fatalf("Lifecycle lost on replace: %v", got.Lifecycle)
	}
}

func TestRemove(t *testing.T) {
	s := NewStore()
	s.Upsert(mkRepo("/a", "alpha"))
	s.Upsert(mkRepo("/b", "beta"))
	s.Remove("/a")
	if s.Get("/a") != nil {
		t.Fatal("repo not removed")
	}
	if got := s.List(); len(got) != 1 || got[0].Name != "beta" {
		t.Fatalf("List = %v, want [beta]", got)
	}
}

func TestEvents(t *testing.T) {
	s := NewStore()
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)

	s.Upsert(mkRepo("/a", "alpha"))
	e := <-ch
	if e.Kind != EventRepoAdded || e.RepoID != "/a" {
		t.Fatalf("event = %+v, want RepoAdded /a", e)
	}

	s.Upsert(mkRepo("/a", "alpha"))
	e = <-ch
	if e.Kind != EventRepoUpdated {
		t.Fatalf("event = %+v, want RepoUpdated", e)
	}

	s.Remove("/a")
	e = <-ch
	if e.Kind != EventRepoRemoved {
		t.Fatalf("event = %+v, want RepoRemoved", e)
	}
}

func TestScanLifecycle(t *testing.T) {
	s := NewStore()
	if s.Scanning() {
		t.Fatal("should not be scanning initially")
	}
	s.SetScan(true)
	if !s.Scanning() {
		t.Fatal("should be scanning")
	}
	s.SetScan(false)
	if s.Scanning() {
		t.Fatal("scan should be complete")
	}
	// Nested scans (re-scan while scanning) need balanced completion.
	s.SetScan(true)
	s.SetScan(true)
	s.SetScan(false)
	if !s.Scanning() {
		t.Fatal("still one scan in flight")
	}
	s.SetScan(false)
	if s.Scanning() {
		t.Fatal("all scans complete")
	}
}
