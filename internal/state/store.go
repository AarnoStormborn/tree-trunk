// Package state holds the authoritative app model and event bus (D9).
// See docs/design/01-architecture.md §4.
package state

import (
	"sort"
	"sync"

	"github.com/harshsingh/tree-trunk/internal/model"
)

// EventKind enumerates store mutations.
type EventKind int

const (
	EventScanStarted EventKind = iota
	EventScanComplete
	EventRepoAdded
	EventRepoUpdated
	EventRepoRemoved
	EventRefreshStarted
	EventRefreshFinished
	EventError
)

// Event is one state mutation (herdr-style snapshot + event deltas).
type Event struct {
	Kind   EventKind
	RepoID string
	Err    error
}

// Store is the single authoritative model. Workers mutate it and emit
// events; the UI subscribes and re-renders on change.
type Store struct {
	mu        sync.RWMutex
	repos     map[string]*model.Repo
	order     []string // stable display order (by Name, then ID)
	subs      map[chan Event]struct{}
	scanCount int
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		repos: map[string]*model.Repo{},
		subs:  map[chan Event]struct{}{},
	}
}

// Upsert adds or replaces a repo, preserving lifecycle state on replace.
func (s *Store) Upsert(repo *model.Repo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, existed := s.repos[repo.ID]
	if repo.Lifecycle == "" {
		repo.Lifecycle = model.StateStale
	}
	if prev, ok := s.repos[repo.ID]; ok {
		repo.Lifecycle = prev.Lifecycle
		repo.LastError = prev.LastError
		// M0 heuristic: keep the first-seen Path as the repo path (the
		// linked-worktree resolution is identical by ID but points at the
		// worktree dir). M1 replaces this with proper `git worktree list`
		// loading that marks IsMain.
		if repo.Path == "" || (prev.Path != "" && prev.Path != repo.Path) {
			repo.Path = prev.Path
		}
	}
	s.repos[repo.ID] = repo
	if !existed {
		s.order = append(s.order, repo.ID)
		sortOrder(s.order, s.repos)
		s.emit(Event{Kind: EventRepoAdded, RepoID: repo.ID})
	} else {
		s.emit(Event{Kind: EventRepoUpdated, RepoID: repo.ID})
	}
}

// Remove deletes a repo (e.g. it vanished on re-scan).
func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.repos[id]; !ok {
		return
	}
	delete(s.repos, id)
	for i, r := range s.order {
		if r == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	s.emit(Event{Kind: EventRepoRemoved, RepoID: id})
}

// Get returns a repo by ID (nil when absent).
func (s *Store) Get(id string) *model.Repo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.repos[id]
}

// List returns repos in stable display order.
func (s *Store) List() []*model.Repo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Repo, 0, len(s.order))
	for _, id := range s.order {
		if r := s.repos[id]; r != nil {
			out = append(out, r)
		}
	}
	return out
}

// SetLifecycle updates a repo's refresh state and emits the matching event.
func (s *Store) SetLifecycle(id string, ls model.RefreshState, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.repos[id]
	if r == nil {
		return
	}
	r.Lifecycle = ls
	r.LastError = err
	kind := EventRefreshFinished
	if ls == model.StateRefreshing {
		kind = EventRefreshStarted
	}
	s.emit(Event{Kind: kind, RepoID: id, Err: err})
}

// SetScan marks the scan phase.
func (s *Store) SetScan(active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if active {
		s.scanCount++
		s.emit(Event{Kind: EventScanStarted})
		return
	}
	s.scanCount--
	if s.scanCount < 0 {
		s.scanCount = 0
	}
	if s.scanCount == 0 {
		s.emit(Event{Kind: EventScanComplete})
	}
}

// Scanning reports whether a scan is in flight.
func (s *Store) Scanning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scanCount > 0
}

// Subscribe returns a buffered event channel; the caller must drain it.
// Unsubscribe by closing the channel.
func (s *Store) Subscribe() chan Event {
	ch := make(chan Event, 128)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Store) Unsubscribe(ch chan Event) {
	s.mu.Lock()
	delete(s.subs, ch)
	s.mu.Unlock()
}

func (s *Store) emit(e Event) {
	for ch := range s.subs {
		select {
		case ch <- e:
		default: // slow subscriber: drop rather than block workers
		}
	}
}

func sortOrder(order []string, repos map[string]*model.Repo) {
	sort.SliceStable(order, func(i, j int) bool {
		a, b := repos[order[i]], repos[order[j]]
		if a == nil || b == nil {
			return a != nil
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
}
