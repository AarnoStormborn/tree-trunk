package state

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AarnoStormborn/tree-trunk/internal/git"
	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// fakeStatusRunner implements git.Runner, returning serialized porcelain for
// the status command and a canned fingerprint for ref commands.
type fakeStatusRunner struct {
	mu          sync.Mutex
	porcelain   []byte
	statusErr   error
	fp          string
	statusCalls atomic.Int32
	// firstStatusBlock, when non-nil, blocks the FIRST status call until
	// released (for sequencing tests).
	firstStatusBlock chan struct{}
	released         chan struct{}
}

func (r *fakeStatusRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	return nil, errors.New("n/a")
}
func (r *fakeStatusRunner) RunIn(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "status" {
		n := r.statusCalls.Add(1)
		if r.firstStatusBlock != nil && n == 1 {
			<-r.released // block the FIRST refresh mid-flight, before locking
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.statusErr != nil {
			return nil, r.statusErr
		}
		return r.porcelain, nil
	}
	// Fingerprint runs for-each-ref + rev-parse; keep them distinct so the
	// joined fingerprint is deterministic ("refs\x00head").
	for _, a := range args {
		if a == "for-each-ref" {
			return []byte("refs"), nil
		}
	}
	return []byte("head"), nil
}
func (r *fakeStatusRunner) RunStream(ctx context.Context, w io.Writer, args ...string) error {
	return errors.New("n/a")
}
func (r *fakeStatusRunner) RunPaged(ctx context.Context, args []string, onLines func(line []byte) error) error {
	return errors.New("n/a")
}

var _ git.Runner = (*fakeStatusRunner)(nil)

func TestRefreshOnePopulatesStatus(t *testing.T) {
	store := NewStore()
	store.Upsert(&model.Repo{ID: "/r", Name: "r", Path: "/r"})

	runner := &fakeStatusRunner{
		porcelain: []byte("## main...origin/main\x00 M f.txt\x00?? u.txt\x00"),
		fp:        "fp1",
	}
	rf := NewRefresher(runner, store, 2)

	rf.RefreshOne(context.Background(), "/r")

	repo := store.Get("/r")
	if repo.Lifecycle != model.StateFresh {
		t.Fatalf("lifecycle = %v, want fresh", repo.Lifecycle)
	}
	if repo.Branch != "main" || repo.Status.Unstaged != 1 || repo.Status.Untracked != 1 {
		t.Fatalf("branch=%q status=%+v", repo.Branch, repo.Status)
	}
	if repo.RefState != "refs\x00head" {
		t.Fatalf("RefState = %q", repo.RefState)
	}
}

func TestRefreshErrorSetsErrorState(t *testing.T) {
	store := NewStore()
	store.Upsert(&model.Repo{ID: "/r", Name: "r", Path: "/r"})

	runner := &fakeStatusRunner{statusErr: errors.New("git exploded")}
	rf := NewRefresher(runner, store, 2)

	rf.RefreshOne(context.Background(), "/r")

	repo := store.Get("/r")
	if repo.Lifecycle != model.StateError {
		t.Fatalf("lifecycle = %v, want error", repo.Lifecycle)
	}
	if repo.LastError == nil {
		t.Fatal("LastError not set")
	}
}

// TestRefreshStaleDropped verifies sequenced loads: refresh #1 starts first
// (seq 1) but completes after refresh #2 (seq 2); its result must be
// dropped so the store reflects #2.
func TestRefreshStaleDropped(t *testing.T) {
	store := NewStore()
	store.Upsert(&model.Repo{ID: "/r", Name: "r", Path: "/r"})

	runner := &fakeStatusRunner{
		porcelain:        []byte("## main\x00"),
		fp:               "fp",
		firstStatusBlock: make(chan struct{}),
		released:         make(chan struct{}),
	}
	rf := NewRefresher(runner, store, 2)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rf.refreshOne(context.Background(), "/r") // seq 1 — blocks in status
	}()
	waitFor(t, func() bool { return runner.statusCalls.Load() == 1 })

	rf.refreshOne(context.Background(), "/r") // seq 2 — completes immediately
	close(runner.released)                    // release #1; it must be dropped
	wg.Wait()

	rf.mu.Lock()
	applied := rf.applied["/r"]
	rf.mu.Unlock()
	if applied != 2 {
		t.Fatalf("applied seq = %d, want 2 (older result must be dropped)", applied)
	}
	if repo := store.Get("/r"); repo.Lifecycle != model.StateFresh {
		t.Fatalf("lifecycle = %v", repo.Lifecycle)
	}
}

func TestRefreshAllSkipsBare(t *testing.T) {
	store := NewStore()
	store.Upsert(&model.Repo{ID: "/bare", Name: "bare", Path: "", Bare: true})
	store.Upsert(&model.Repo{ID: "/r", Name: "r", Path: "/r"})

	runner := &fakeStatusRunner{porcelain: []byte("## main\x00"), fp: "x"}
	rf := NewRefresher(runner, store, 2)
	rf.RefreshAll(context.Background())

	if store.Get("/bare").Lifecycle != model.StateStale {
		t.Fatal("bare repo should stay stale (no worktree status)")
	}
	if store.Get("/r").Lifecycle != model.StateFresh {
		t.Fatal("normal repo should be fresh")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 5s")
		}
		time.Sleep(time.Millisecond)
	}
}
