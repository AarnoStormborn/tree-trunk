package model

import "testing"

func TestStatusSummary(t *testing.T) {
	cases := []struct {
		name string
		s    RepoStatus
		want string
	}{
		{"clean", RepoStatus{}, ""},
		{"unstaged", RepoStatus{Unstaged: 3}, " ~3"},
		{"untracked", RepoStatus{Untracked: 1}, " +1"},
		{"staged+unstaged", RepoStatus{Staged: 2, Unstaged: 3}, " *2 ~3"},
		{"conflicts first", RepoStatus{Conflicts: 1, Staged: 2, Unstaged: 3, Untracked: 4}, "!1 *2 ~3 +4"},
		{"ahead/behind not in summary", RepoStatus{Ahead: 5, Behind: 2}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.Summary(); got != tc.want {
				t.Fatalf("Summary() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusDirty(t *testing.T) {
	if (RepoStatus{}).Dirty() {
		t.Fatal("clean status should not be dirty")
	}
	if !(RepoStatus{Untracked: 1}).Dirty() {
		t.Fatal("untracked file should be dirty")
	}
}
