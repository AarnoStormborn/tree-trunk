package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// stateFile is the persisted app state (recent repos).
// XDG-ish location: ~/.local/state/tree-trunk/state.json.
type stateFile struct {
	RecentRepos []string `json:"recent_repos"` // canonical repo IDs, MRU first
}

func statePath(home string) string {
	return filepath.Join(home, ".local", "state", "tree-trunk", "state.json")
}

// loadState reads the persisted state; a missing file yields empty state.
func loadState(home string) stateFile {
	var st stateFile
	data, err := os.ReadFile(statePath(home))
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

// saveState writes the persisted state (best-effort; failures ignored).
func saveState(home string, st stateFile) {
	path := statePath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// touchRecent moves id to the front of the MRU list (cap 20).
func (st *stateFile) touchRecent(id string) {
	out := make([]string, 0, 20)
	out = append(out, id)
	for _, r := range st.RecentRepos {
		if r == id {
			continue
		}
		out = append(out, r)
		if len(out) >= 20 {
			break
		}
	}
	st.RecentRepos = out
}

// recentItem is one row in the ctrl+r menu.
type recentItem struct {
	id string
}

func (i recentItem) Title() string       { return filepath.Base(i.id) }
func (i recentItem) Description() string { return i.id }
func (i recentItem) FilterValue() string { return i.id }

var _ = model.StateStale
