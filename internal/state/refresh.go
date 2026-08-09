// Package state: refresh machinery (docs/design/01-architecture.md §3–§4).
package state

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/harshsingh/tree-trunk/internal/git"
	"github.com/harshsingh/tree-trunk/internal/model"
)

// Refresher runs per-repo git refreshes on a bounded worker pool with
// fingerprint dedup and sequenced loads (architecture §3.1–§3.3).
//
// Dedup rules:
//   - `git status` re-reads on EVERY refresh (staging/editing changes no
//     ref, so fingerprint gating would hide dirtiness — review M2).
//   - ref-dependent reads are gated by the fingerprint (stored on the repo
//     as RefState; deeper consumers like the log cache use it in M3).
//   - results with a stale sequence number are dropped, so an older
//     in-flight refresh can't clobber a newer one.
type Refresher struct {
	runner  git.Runner
	store   *Store
	workers int

	seq     atomic.Uint64
	mu      sync.Mutex
	applied map[string]uint64 // per-repo applied sequence
}

// NewRefresher builds a refresher. workers is clamped to >= 1.
func NewRefresher(runner git.Runner, store *Store, workers int) *Refresher {
	if workers < 1 {
		workers = 1
	}
	return &Refresher{
		runner:  runner,
		store:   store,
		workers: workers,
		applied: map[string]uint64{},
	}
}

// RefreshAll refreshes every non-bare repo, bounded by the worker pool.
func (rf *Refresher) RefreshAll(ctx context.Context) {
	repos := rf.store.List()
	sem := make(chan struct{}, rf.workers)
	var wg sync.WaitGroup
	for _, r := range repos {
		if r.Bare || r.Path == "" {
			continue // bare repos have no worktree status (v1)
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			rf.refreshOne(ctx, id)
		}(r.ID)
	}
	wg.Wait()
}

// RefreshOne refreshes a single repo (focused-repo priority, §3.4).
func (rf *Refresher) RefreshOne(ctx context.Context, id string) {
	rf.refreshOne(ctx, id)
}

func (rf *Refresher) refreshOne(ctx context.Context, id string) {
	repo := rf.store.Get(id)
	if repo == nil || repo.Path == "" {
		return
	}
	rf.store.SetLifecycle(id, model.StateRefreshing, nil)

	seq := rf.seq.Add(1)

	// Status: always re-read (cheap; must not be skipped).
	st, err := git.Status(ctx, rf.runner, repo.Path)
	if err != nil {
		rf.store.SetLifecycle(id, model.StateError, err)
		return
	}

	// Ref fingerprint: gates ref-dependent reads (stored for later use).
	fp, ferr := git.Fingerprint(ctx, rf.runner, repo.Path)

	// Sequencing: drop stale results.
	rf.mu.Lock()
	if prev, ok := rf.applied[id]; ok && seq < prev {
		rf.mu.Unlock()
		return
	}
	rf.applied[id] = seq
	rf.mu.Unlock()

	repo = rf.store.Get(id)
	if repo == nil {
		return
	}
	repo.Status = *st
	repo.Branch = st.Branch
	if ferr != nil {
		repo.RefState = ""
	} else {
		repo.RefState = fp
	}
	rf.store.Upsert(repo) // preserves lifecycle; see below
	rf.store.SetLifecycle(id, model.StateFresh, nil)
}

// RefreshingCount returns how many repos are currently mid-refresh (for the
// status bar busy display).
func (rf *Refresher) RefreshingCount() int {
	n := 0
	for _, r := range rf.store.List() {
		if r.Lifecycle == model.StateRefreshing {
			n++
		}
	}
	return n
}
