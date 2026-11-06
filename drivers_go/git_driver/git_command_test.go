package gitdriver

import "testing"

func TestConflictStatusLabels(t *testing.T) {
	cases := []struct {
		status string
		label  string
	}{
		{status: "DD", label: "both deleted"},
		{status: "AU", label: "added by us"},
		{status: "UD", label: "deleted by them"},
		{status: "UA", label: "added by them"},
		{status: "DU", label: "deleted by us"},
		{status: "AA", label: "both added"},
		{status: "UU", label: "both modified"},
	}
	for _, testCase := range cases {
		switch {
		case !statusMatchesView(testCase.status, viewConflicted):
			t.Errorf("status %s was not included in the conflicted view", testCase.status)
		case statusLabel(testCase.status, viewConflicted) != testCase.label:
			t.Errorf("status %s label mismatch: got %q expected %q", testCase.status, statusLabel(testCase.status, viewConflicted), testCase.label)
		case statusLabel(testCase.status, viewUncommitted) != "conflicted, "+testCase.label:
			t.Errorf("status %s all-uncommitted label mismatch: got %q expected %q", testCase.status, statusLabel(testCase.status, viewUncommitted), "conflicted, "+testCase.label)
		default:
		}
	}
}

func TestWorkingStateMembership(t *testing.T) {
	cases := []struct {
		status      string
		uncommitted bool
		staged      bool
		unstaged    bool
		untracked   bool
	}{
		{status: "A ", uncommitted: true, staged: true},
		{status: " M", uncommitted: true, unstaged: true},
		{status: "AM", uncommitted: true, staged: true, unstaged: true},
		{status: "??", uncommitted: true, untracked: true},
		{status: "UU", uncommitted: true},
	}
	for _, testCase := range cases {
		actual := []bool{
			statusMatchesView(testCase.status, viewUncommitted),
			statusMatchesView(testCase.status, viewStaged),
			statusMatchesView(testCase.status, viewUnstaged),
			statusMatchesView(testCase.status, viewUntracked),
		}
		expected := []bool{testCase.uncommitted, testCase.staged, testCase.unstaged, testCase.untracked}
		for index := range expected {
			switch {
			case actual[index] != expected[index]:
				t.Errorf("status %s membership mismatch at position %d: got %t expected %t", testCase.status, index, actual[index], expected[index])
			default:
			}
		}
	}
}
