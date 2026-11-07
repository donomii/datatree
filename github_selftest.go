package main

import (
	"fmt"
	"os"
	"strings"
)

const fakeGitHubPullRequestURL = "https://github.com/datatree/example/pull/42"
const fakeGitHubPullRequestName = "datatree/example #42 Datatree GitHub PR"

func fakeGitHubScript(logPath string) string {
	return "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> '" + logPath + "'\n" +
		"case \"$*\" in\n" +
		"'search prs --author=@me --state=open --limit=1000 --json=isDraft,number,repository,state,title,updatedAt,url') printf '%s' '[{\"isDraft\":false,\"number\":42,\"repository\":{\"nameWithOwner\":\"datatree/example\"},\"state\":\"open\",\"title\":\"Datatree GitHub PR\",\"updatedAt\":\"2026-08-09T00:00:00Z\",\"url\":\"" + fakeGitHubPullRequestURL + "\"}]' ;;\n" +
		"'search prs --review-requested=@me --state=open --limit=1000 --json=isDraft,number,repository,state,title,updatedAt,url') printf '[]' ;;\n" +
		"'pr view " + fakeGitHubPullRequestURL + " --json=additions,assignees,author,baseRefName,body,changedFiles,closedAt,comments,commits,createdAt,deletions,files,headRefName,isDraft,labels,latestReviews,mergeStateStatus,mergeable,mergedAt,mergedBy,milestone,number,reviewDecision,reviewRequests,state,statusCheckRollup,title,updatedAt,url') printf '%s' '{\"additions\":2,\"files\":[{\"additions\":2,\"deletions\":1,\"path\":\"main.go\"}],\"headRefName\":\"feature/readable\",\"isDraft\":false,\"number\":42,\"state\":\"OPEN\",\"title\":\"Datatree GitHub PR\",\"url\":\"" + fakeGitHubPullRequestURL + "\"}' ;;\n" +
		"'pr diff " + fakeGitHubPullRequestURL + " --patch --color=never') printf '%s' 'diff --git a/main.go b/main.go\nindex 1111111..2222222 100644\n--- a/main.go\n+++ b/main.go\n@@ -1,2 +1,3 @@\n package main\n-oldValue := 1\n+newValue := 2\n+enabled := true' ;;\n" +
		"'run list --repo datatree/example --branch feature/readable --limit=100 --json=attempt,conclusion,createdAt,databaseId,displayTitle,event,headBranch,headSha,name,number,startedAt,status,updatedAt,url,workflowDatabaseId,workflowName') printf '%s' '[{\"attempt\":1,\"conclusion\":\"failure\",\"databaseId\":9001,\"displayTitle\":\"Build PR\",\"name\":\"CI\",\"number\":12,\"status\":\"completed\",\"workflowName\":\"CI\"}]' ;;\n" +
		"'run view 9001 --repo datatree/example --json=attempt,conclusion,createdAt,databaseId,displayTitle,event,headBranch,headSha,jobs,name,number,startedAt,status,updatedAt,url,workflowDatabaseId,workflowName') printf '%s' '{\"attempt\":1,\"conclusion\":\"failure\",\"databaseId\":9001,\"displayTitle\":\"Build PR\",\"jobs\":[{\"conclusion\":\"failure\",\"databaseId\":9101,\"name\":\"test\",\"status\":\"completed\"}],\"name\":\"CI\",\"number\":12,\"status\":\"completed\",\"workflowName\":\"CI\"}' ;;\n" +
		"'pr comment " + fakeGitHubPullRequestURL + " --body Integration comment') printf '%s' 'comment created' ;;\n" +
		"'run rerun 9001 --repo datatree/example --failed') printf '%s' 'rerun requested' ;;\n" +
		"*) echo \"fake gh refusing unsupported invocation: $*\" >&2; exit 3 ;;\n" +
		"esac\n"
}

func auditGitHubCallLog(logPath string) error {
	logBytes, err := os.ReadFile(logPath)
	switch {
	case err != nil:
		return fmt.Errorf("cannot read the fake GitHub CLI call log %s: %v", logPath, err)
	default:
	}
	calls := 0
	for _, line := range strings.Split(string(logBytes), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		default:
		}
		calls++
		switch {
		case strings.HasPrefix(trimmed, "search prs "):
		case strings.HasPrefix(trimmed, "pr view "):
		case strings.HasPrefix(trimmed, "pr diff "):
		case strings.HasPrefix(trimmed, "pr comment "):
		case strings.HasPrefix(trimmed, "run list "):
		case strings.HasPrefix(trimmed, "run view "):
		case strings.HasPrefix(trimmed, "run rerun "):
		default:
			return fmt.Errorf("GitHub invocation violation: the GitHub workflow ran unsupported gh command %q", trimmed)
		}
	}
	switch {
	case calls == 0:
		return fmt.Errorf("read-only audit found no GitHub CLI calls in %s: expected the GitHub walk to invoke the fake gh", logPath)
	default:
		return nil
	}
}

func checkGitHubBrowse(store *Store, rootResources []Resource) error {
	githubHandle, err := findResourceHandle(store, rootResources, "github", "github")
	switch {
	case err != nil:
		return err
	default:
	}
	githubResources, err := store.loadDriverForHandle("github_driver", githubHandle)
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed for the github root: %v", err)
	default:
	}
	authoredHandle, err := findResourceHandle(store, githubResources, "[open pull requests I authored]", "github")
	switch {
	case err != nil:
		return err
	default:
	}
	authoredResources, err := store.loadDriverForHandle("github_driver", authoredHandle)
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed for authored pull requests: %v", err)
	default:
	}
	pullRequestHandle, err := findResourceHandle(store, authoredResources, fakeGitHubPullRequestName, "github")
	switch {
	case err != nil:
		return err
	default:
	}
	pullRequestResources, err := store.loadDriverForHandle("github_driver", pullRequestHandle)
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed for pull request %s: %v", fakeGitHubPullRequestURL, err)
	default:
	}
	switch _, err := findResource(pullRequestResources, "title: Datatree GitHub PR", "kv"); {
	case err != nil:
		return err
	default:
	}
	actionsHandle, err := findResourceHandle(store, pullRequestResources, "[actions]", "github")
	switch {
	case err != nil:
		return err
	default:
	}
	actionResources, err := store.loadDriverForHandle("github_driver", actionsHandle)
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed for pull request actions: %v", err)
	default:
	}
	commentHandle, err := findResourceHandle(store, actionResources, "Add comment", "github-action-comment")
	switch {
	case err != nil:
		return err
	default:
	}
	commentMessage, err := executeGitHubAction(store, GitHubActionRequest{Handle: commentHandle.Hid, Body: "Integration comment"})
	switch {
	case err != nil:
		return fmt.Errorf("fake GitHub pull request comment action failed: %v", err)
	case commentMessage != "comment created":
		return fmt.Errorf("fake GitHub pull request comment response mismatch: got %q expected %q", commentMessage, "comment created")
	default:
	}
	runsHandle, err := findResourceHandle(store, pullRequestResources, "[CI/CD runs]", "github")
	switch {
	case err != nil:
		return err
	default:
	}
	runResources, err := store.loadDriverForHandle("github_driver", runsHandle)
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed for pull request workflow runs: %v", err)
	default:
	}
	runHandle, err := findResourceHandle(store, runResources, "CI #12 completed failure Build PR", "github")
	switch {
	case err != nil:
		return err
	default:
	}
	runDetailResources, err := store.loadDriverForHandle("github_driver", runHandle)
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed for workflow run 9001: %v", err)
	default:
	}
	runActionsHandle, err := findResourceHandle(store, runDetailResources, "[actions]", "github")
	switch {
	case err != nil:
		return err
	default:
	}
	runActionResources, err := store.loadDriverForHandle("github_driver", runActionsHandle)
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed for workflow run actions: %v", err)
	default:
	}
	rerunFailedHandle, err := findResourceHandle(store, runActionResources, "Rerun failed jobs", "github-action-run-rerun-failed")
	switch {
	case err != nil:
		return err
	default:
	}
	rerunMessage, err := executeGitHubAction(store, GitHubActionRequest{Handle: rerunFailedHandle.Hid})
	switch {
	case err != nil:
		return fmt.Errorf("fake GitHub workflow rerun action failed: %v", err)
	case rerunMessage != "rerun requested":
		return fmt.Errorf("fake GitHub workflow rerun response mismatch: got %q expected %q", rerunMessage, "rerun requested")
	default:
	}
	diffHandle, err := findResourceHandle(store, pullRequestResources, "[diff]", "github")
	switch {
	case err != nil:
		return err
	default:
	}
	diffResources, err := store.loadDriverForHandle("github_driver", diffHandle)
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed for pull request diff %s: %v", fakeGitHubPullRequestURL, err)
	default:
	}
	mainDiffHandle, err := findResourceHandle(store, diffResources, "main.go", "diff")
	switch {
	case err != nil:
		return err
	default:
	}
	diffLineResources, err := store.loadDriverForHandle("diff_driver", mainDiffHandle)
	switch {
	case err != nil:
		return fmt.Errorf("diff driver load failed for GitHub pull request file main.go: %v", err)
	default:
	}
	switch _, err := findResource(diffLineResources, "+             3 | enabled := true", "diff-add"); {
	case err != nil:
		return err
	default:
	}
	filesHandle, err := findResourceHandle(store, pullRequestResources, "files", "json")
	switch {
	case err != nil:
		return err
	default:
	}
	filesResources, err := store.loadDriverForHandle("json_driver", filesHandle)
	switch {
	case err != nil:
		return fmt.Errorf("JSON driver load failed for GitHub pull request files: %v", err)
	default:
	}
	fileHandle, err := findResourceHandle(store, filesResources, "[0]", "json")
	switch {
	case err != nil:
		return err
	default:
	}
	fileResources, err := store.loadDriverForHandle("json_driver", fileHandle)
	switch {
	case err != nil:
		return fmt.Errorf("JSON driver load failed for a GitHub pull request file: %v", err)
	default:
	}
	switch _, err := findResource(fileResources, "path: main.go", "kv"); {
	case err != nil:
		return err
	default:
	}
	reviewHandle, err := findResourceHandle(store, githubResources, "[open pull requests requesting my review]", "github")
	switch {
	case err != nil:
		return err
	default:
	}
	reviewResources, err := store.loadDriverForHandle("github_driver", reviewHandle)
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed for review-requested pull requests: %v", err)
	case len(reviewResources) != 1 || reviewResources[0].Kind != "raw":
		return fmt.Errorf("GitHub review-requested list mismatch: expected no pull requests, got %+v", reviewResources)
	default:
	}
	switch err := os.Setenv("DATATREE_GITHUB_MAX_OUTPUT_BYTES", "10"); {
	case err != nil:
		return fmt.Errorf("cannot set bounded GitHub output size: %v", err)
	default:
	}
	boundedResources, err := store.loadDriverForHandle("github_driver", authoredHandle)
	os.Unsetenv("DATATREE_GITHUB_MAX_OUTPUT_BYTES")
	switch {
	case err != nil:
		return fmt.Errorf("GitHub driver load failed with a ten-byte output limit: %v", err)
	case len(boundedResources) != 1 || boundedResources[0].Kind != "error" || !strings.Contains(boundedResources[0].Name, "DATATREE_GITHUB_MAX_OUTPUT_BYTES=10"):
		return fmt.Errorf("GitHub output limit did not produce an accurate error: got %+v", boundedResources)
	default:
		return nil
	}
}
