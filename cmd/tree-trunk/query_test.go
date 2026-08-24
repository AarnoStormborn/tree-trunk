package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

func testRepo(id, name string, dirty bool) *model.Repo {
	st := model.RepoStatus{Branch: "main"}
	if dirty {
		st.Untracked = 1
	}
	return &model.Repo{ID: id, Name: name, Path: id, Branch: "main", Status: st}
}

func TestFilterRepos(t *testing.T) {
	repos := []*model.Repo{
		testRepo("/a", "alpha", true),
		testRepo("/b", "beta", false),
	}
	// dirty=true: alpha has untracked, so dirty.
	got := filterRepos(repos, []string{"dirty=true"})
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Fatalf("dirty filter: got %d", len(got))
	}
	got = filterRepos(repos, []string{"name=beta"})
	if len(got) != 1 || got[0].Name != "beta" {
		t.Fatalf("name filter: got %+v", got)
	}
	// Multiple filters AND together.
	got = filterRepos(repos, []string{"dirty=true", "name=alpha"})
	if len(got) != 1 {
		t.Fatalf("AND filter: got %d", len(got))
	}
	got = filterRepos(repos, []string{"dirty=true", "name=beta"})
	if len(got) != 0 {
		t.Fatalf("AND filter mismatch: got %d", len(got))
	}
}

func TestTrimRepoFields(t *testing.T) {
	r := testRepo("/a", "alpha", true)
	trimmed := trimRepo(*r, map[string]bool{"id": true, "branch": true})
	if trimmed.ID != "/a" || trimmed.Branch != "main" {
		t.Fatalf("expected id+branch kept, got %+v", trimmed)
	}
	if trimmed.Name != "" {
		t.Fatalf("name should be dropped, got %q", trimmed.Name)
	}
}

func TestStatusFileMarshalJSON(t *testing.T) {
	f := model.StatusFile{X: '?', Y: '?', Path: "new.md"}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"code":"??"`) || !strings.Contains(s, `"untracked":true`) {
		t.Fatalf("bad status json: %s", s)
	}
}
