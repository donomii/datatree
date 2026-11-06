package githubdriver

import (
	"strings"
	"testing"
)

func invocationPanics(args []string) (panicked bool) {
	defer func() {
		switch {
		case recover() != nil:
			panicked = true
		default:
			panicked = false
		}
	}()
	assertReadOnlyGitHubInvocation(args)
	return false
}

func TestReadOnlyInvocationGate(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantPanic bool
	}{
		{name: "authored search", args: authoredSearchArgs(), wantPanic: false},
		{name: "review search", args: reviewRequestedSearchArgs(), wantPanic: false},
		{name: "pull request view", args: pullRequestViewArgs("https://github.com/example/repo/pull/7"), wantPanic: false},
		{name: "pull request diff", args: pullRequestDiffArgs("https://github.com/example/repo/pull/7"), wantPanic: false},
		{name: "workflow runs", args: workflowRunListArgs("example/repo", "feature/test"), wantPanic: false},
		{name: "workflow run", args: workflowRunViewArgs("example/repo", 99), wantPanic: false},
		{name: "merge blocked", args: []string{"pr", "merge", "7"}, wantPanic: true},
		{name: "web view blocked", args: []string{"pr", "view", "https://github.com/example/repo/pull/7", "--web"}, wantPanic: true},
		{name: "diff web view blocked", args: []string{"pr", "diff", "https://github.com/example/repo/pull/7", "--web"}, wantPanic: true},
		{name: "workflow cancel blocked from read gate", args: []string{"run", "cancel", "99", "--repo", "example/repo"}, wantPanic: true},
	}
	for _, testCase := range tests {
		got := invocationPanics(testCase.args)
		switch {
		case got != testCase.wantPanic:
			t.Errorf("%s panic mismatch: got %t expected %t for %#v", testCase.name, got, testCase.wantPanic, testCase.args)
		default:
		}
	}
}

func TestMountStateValidation(t *testing.T) {
	tests := []struct {
		name    string
		state   mountState
		wantErr bool
	}{
		{name: "root", state: mountState{View: viewRoot}, wantErr: false},
		{name: "pull request", state: mountState{View: viewPullRequest, URL: "https://github.com/example/repo/pull/7"}, wantErr: false},
		{name: "diff", state: mountState{View: viewDiff, URL: "https://github.com/example/repo/pull/7"}, wantErr: false},
		{name: "actions", state: mountState{View: viewActions, URL: "https://github.com/example/repo/pull/7", State: "OPEN"}, wantErr: false},
		{name: "runs", state: mountState{View: viewRuns, Repository: "example/repo", Branch: "feature/test"}, wantErr: false},
		{name: "run", state: mountState{View: viewRun, Repository: "example/repo", RunID: 99}, wantErr: false},
		{name: "run actions", state: mountState{View: viewRunActions, Repository: "example/repo", RunID: 99, State: "completed"}, wantErr: false},
		{name: "insecure URL", state: mountState{View: viewPullRequest, URL: "http://github.com/example/repo/pull/7"}, wantErr: true},
		{name: "URL on root", state: mountState{View: viewRoot, URL: "https://github.com/example/repo/pull/7"}, wantErr: true},
	}
	for _, testCase := range tests {
		err := validateMountState(testCase.state)
		switch {
		case (err != nil) != testCase.wantErr:
			t.Errorf("%s error mismatch: got %v expected error %t", testCase.name, err, testCase.wantErr)
		default:
		}
	}
}

func TestGitHubActionArguments(t *testing.T) {
	pullTarget := actionTarget{URL: "https://github.com/example/repo/pull/7"}
	runTarget := actionTarget{Repository: "example/repo", RunID: 99}
	tests := []struct {
		name        string
		kind        string
		target      actionTarget
		body        string
		mergeMethod string
		wantArgs    []string
		wantErr     bool
	}{
		{name: "close", kind: actionClose, target: pullTarget, wantArgs: []string{"pr", "close", pullTarget.URL}},
		{name: "reopen", kind: actionReopen, target: pullTarget, wantArgs: []string{"pr", "reopen", pullTarget.URL}},
		{name: "default squash merge", kind: actionMerge, target: pullTarget, wantArgs: []string{"pr", "merge", pullTarget.URL, "--squash"}},
		{name: "rebase merge", kind: actionMerge, target: pullTarget, mergeMethod: "rebase", wantArgs: []string{"pr", "merge", pullTarget.URL, "--rebase"}},
		{name: "comment", kind: actionComment, target: pullTarget, body: "Looks good", wantArgs: []string{"pr", "comment", pullTarget.URL, "--body", "Looks good"}},
		{name: "approve", kind: actionApprove, target: pullTarget, wantArgs: []string{"pr", "review", pullTarget.URL, "--approve"}},
		{name: "review comment", kind: actionReviewComment, target: pullTarget, body: "Question", wantArgs: []string{"pr", "review", pullTarget.URL, "--comment", "--body", "Question"}},
		{name: "request changes", kind: actionRequestChanges, target: pullTarget, body: "Please fix", wantArgs: []string{"pr", "review", pullTarget.URL, "--request-changes", "--body", "Please fix"}},
		{name: "rerun", kind: actionRunRerun, target: runTarget, wantArgs: []string{"run", "rerun", "99", "--repo", "example/repo"}},
		{name: "rerun failed", kind: actionRunRerunFailed, target: runTarget, wantArgs: []string{"run", "rerun", "99", "--repo", "example/repo", "--failed"}},
		{name: "cancel", kind: actionRunCancel, target: runTarget, wantArgs: []string{"run", "cancel", "99", "--repo", "example/repo"}},
		{name: "empty comment", kind: actionComment, target: pullTarget, body: " ", wantErr: true},
		{name: "admin merge blocked", kind: actionMerge, target: pullTarget, mergeMethod: "admin", wantErr: true},
		{name: "wrong target", kind: actionClose, target: runTarget, wantErr: true},
	}
	for _, testCase := range tests {
		args, _, err := githubActionArgs(testCase.kind, testCase.target, testCase.body, testCase.mergeMethod)
		switch {
		case (err != nil) != testCase.wantErr:
			t.Errorf("%s error mismatch: got %v expected error %t", testCase.name, err, testCase.wantErr)
		case err == nil && !stringSlicesEqual(args, testCase.wantArgs):
			t.Errorf("%s arguments mismatch: got %#v expected %#v", testCase.name, args, testCase.wantArgs)
		case err == nil && validateGitHubActionInvocation(args) != nil:
			t.Errorf("%s generated arguments failed the action gate: %#v", testCase.name, args)
		default:
		}
	}
}

func TestGitHubActionGateBlocksUnsafeFlags(t *testing.T) {
	unsafe := [][]string{
		{"pr", "merge", "https://github.com/example/repo/pull/7", "--admin"},
		{"pr", "close", "https://github.com/example/repo/pull/7", "--delete-branch"},
		{"run", "cancel", "99", "--repo", "example/repo", "--force"},
		{"pr", "comment", "https://github.com/example/repo/pull/7", "--web", "ignored"},
	}
	for _, args := range unsafe {
		switch err := validateGitHubActionInvocation(args); {
		case err == nil:
			t.Errorf("unsafe GitHub action arguments were not blocked: %#v", args)
		default:
		}
	}
}

func TestSplitUnifiedDiff(t *testing.T) {
	raw := []byte("diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\ndiff --git a/old.txt b/new.txt\nsimilarity index 100%\nrename from old.txt\nrename to new.txt\n")
	files := splitUnifiedDiff(raw)
	switch {
	case len(files) != 2:
		t.Fatalf("split diff count mismatch: got %d expected 2", len(files))
	case unifiedDiffFileName(files[0], 0) != "main.go":
		t.Fatalf("first diff name mismatch: got %q expected main.go", unifiedDiffFileName(files[0], 0))
	case unifiedDiffFileName(files[1], 1) != "new.txt":
		t.Fatalf("renamed diff name mismatch: got %q expected new.txt", unifiedDiffFileName(files[1], 1))
	default:
	}
}

func TestMergeUnifiedDiffFilesCombinesRepeatedCommitPatches(t *testing.T) {
	raw := []byte("diff --git a/display-map.json b/display-map.json\n--- a/display-map.json\n+++ b/display-map.json\n@@ -1 +1 @@\n-old-one\n+new-one\ndiff --git a/other.go b/other.go\n--- a/other.go\n+++ b/other.go\n@@ -1 +1 @@\n-old\n+new\ndiff --git a/display-map.json b/display-map.json\n--- a/display-map.json\n+++ b/display-map.json\n@@ -2 +2 @@\n-old-two\n+new-two")
	resources, children := mergeUnifiedDiffFiles(splitUnifiedDiff(raw))
	switch {
	case len(resources) != 2:
		t.Fatalf("merged diff resource count mismatch: got %d expected 2: %+v", len(resources), resources)
	case resources[0].Name != "display-map.json" || resources[1].Name != "other.go":
		t.Fatalf("merged diff order mismatch: got %+v", resources)
	case !strings.Contains(string(children["display-map.json"]), "+new-one") || !strings.Contains(string(children["display-map.json"]), "+new-two"):
		t.Fatalf("repeated file patches were not combined in order: %q", children["display-map.json"])
	case resources[0].Size != int64(len(children["display-map.json"])):
		t.Fatalf("merged diff size mismatch: got %d expected %d", resources[0].Size, len(children["display-map.json"]))
	default:
	}
}
