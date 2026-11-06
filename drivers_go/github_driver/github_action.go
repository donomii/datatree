package githubdriver

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const maxGitHubActionBodyBytes = 64 * 1024

func ExecuteAction(kind string, rawTarget []byte, body string, mergeMethod string) ([]byte, error) {
	var target actionTarget
	switch err := json.Unmarshal(rawTarget, &target); {
	case err != nil:
		return nil, fmt.Errorf("cannot execute GitHub action %q: expected a JSON action target, got %d bytes with error: %v", kind, len(rawTarget), err)
	default:
	}
	args, description, argsErr := githubActionArgs(kind, target, body, mergeMethod)
	switch {
	case argsErr != nil:
		return nil, argsErr
	default:
	}
	config, configErr := resolveGitHubConfig()
	switch {
	case configErr != nil:
		return nil, configErr
	default:
	}
	return runGitHubAction(config, args, description)
}

func githubActionArgs(kind string, target actionTarget, body string, mergeMethod string) ([]string, string, error) {
	switch kind {
	case actionClose, actionReopen, actionMerge, actionComment, actionApprove, actionReviewComment, actionRequestChanges:
		switch err := validatePullRequestURL(target.URL); {
		case err != nil:
			return nil, "", err
		case target.Repository != "" || target.RunID != 0:
			return nil, "", fmt.Errorf("cannot execute GitHub pull request action %q: expected only a pull request URL, got repository=%q runId=%d", kind, target.Repository, target.RunID)
		default:
		}
	case actionRunRerun, actionRunRerunFailed, actionRunCancel:
		switch err := validateRepositoryName(target.Repository); {
		case err != nil:
			return nil, "", err
		case target.RunID <= 0:
			return nil, "", fmt.Errorf("cannot execute GitHub workflow action %q for repository %q: expected a positive run id, got %d", kind, target.Repository, target.RunID)
		case target.URL != "":
			return nil, "", fmt.Errorf("cannot execute GitHub workflow action %q: expected repository and run id only, got URL %q", kind, target.URL)
		default:
		}
	default:
		return nil, "", fmt.Errorf("cannot execute GitHub action kind %q: expected a supported pull request or workflow action", kind)
	}
	trimmedBody := strings.TrimSpace(body)
	switch {
	case len(body) > maxGitHubActionBodyBytes:
		return nil, "", fmt.Errorf("cannot execute GitHub action %q: received a %d-byte body, but action bodies are limited to %d bytes", kind, len(body), maxGitHubActionBodyBytes)
	case kind != actionComment && kind != actionReviewComment && kind != actionRequestChanges && body != "":
		return nil, "", fmt.Errorf("cannot execute GitHub action %q: expected no text body, got %d bytes", kind, len(body))
	case kind != actionMerge && mergeMethod != "":
		return nil, "", fmt.Errorf("cannot execute GitHub action %q: merge_method applies only to merge, got %q", kind, mergeMethod)
	default:
	}
	switch kind {
	case actionClose:
		return []string{"pr", "close", target.URL}, "close pull request " + target.URL, nil
	case actionReopen:
		return []string{"pr", "reopen", target.URL}, "reopen pull request " + target.URL, nil
	case actionMerge:
		method := mergeMethod
		switch {
		case method == "":
			method = "squash"
		case method == "merge", method == "squash", method == "rebase":
		default:
			return nil, "", fmt.Errorf("cannot merge pull request %q: expected merge method merge, squash or rebase, got %q", target.URL, method)
		}
		return []string{"pr", "merge", target.URL, "--" + method}, method + " merge pull request " + target.URL, nil
	case actionComment:
		switch {
		case trimmedBody == "":
			return nil, "", fmt.Errorf("cannot comment on pull request %q: expected a non-empty comment body", target.URL)
		default:
			return []string{"pr", "comment", target.URL, "--body", body}, "comment on pull request " + target.URL, nil
		}
	case actionApprove:
		return []string{"pr", "review", target.URL, "--approve"}, "approve pull request " + target.URL, nil
	case actionReviewComment:
		switch {
		case trimmedBody == "":
			return nil, "", fmt.Errorf("cannot submit a review comment on pull request %q: expected a non-empty review body", target.URL)
		default:
			return []string{"pr", "review", target.URL, "--comment", "--body", body}, "submit review comment on pull request " + target.URL, nil
		}
	case actionRequestChanges:
		switch {
		case trimmedBody == "":
			return nil, "", fmt.Errorf("cannot request changes on pull request %q: expected a non-empty review body", target.URL)
		default:
			return []string{"pr", "review", target.URL, "--request-changes", "--body", body}, "request changes on pull request " + target.URL, nil
		}
	case actionRunRerun:
		return []string{"run", "rerun", strconv.FormatInt(target.RunID, 10), "--repo", target.Repository}, fmt.Sprintf("rerun all jobs in repository %s workflow run %d", target.Repository, target.RunID), nil
	case actionRunRerunFailed:
		return []string{"run", "rerun", strconv.FormatInt(target.RunID, 10), "--repo", target.Repository, "--failed"}, fmt.Sprintf("rerun failed jobs in repository %s workflow run %d", target.Repository, target.RunID), nil
	case actionRunCancel:
		return []string{"run", "cancel", strconv.FormatInt(target.RunID, 10), "--repo", target.Repository}, fmt.Sprintf("cancel repository %s workflow run %d", target.Repository, target.RunID), nil
	default:
		return nil, "", fmt.Errorf("cannot execute GitHub action kind %q: expected a supported action", kind)
	}
}

func runGitHubAction(config githubConfig, args []string, description string) ([]byte, error) {
	switch err := validateGitHubActionInvocation(args); {
	case err != nil:
		return nil, err
	default:
	}
	cmd := exec.Command(config.executable, args...)
	stdout := boundedBuffer{limit: config.maxOutputBytes}
	stderr := boundedBuffer{limit: config.maxOutputBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	switch {
	case stdout.exceeded():
		return nil, fmt.Errorf("GitHub action failed while attempting to %s: gh produced %d stdout bytes, exceeding DATATREE_GITHUB_MAX_OUTPUT_BYTES=%d", description, stdout.total, config.maxOutputBytes)
	case stderr.exceeded():
		return nil, fmt.Errorf("GitHub action failed while attempting to %s: gh produced %d stderr bytes, exceeding DATATREE_GITHUB_MAX_OUTPUT_BYTES=%d", description, stderr.total, config.maxOutputBytes)
	case runErr != nil:
		return nil, fmt.Errorf("GitHub action failed while attempting to %s: expected a zero exit, got %v with stderr: %s", description, runErr, strings.TrimSpace(stderr.data.String()))
	case len(strings.TrimSpace(stdout.data.String())) == 0:
		return []byte("GitHub action completed: " + description), nil
	default:
		return []byte(strings.TrimSpace(stdout.data.String())), nil
	}
}

func validateGitHubActionInvocation(args []string) error {
	switch {
	case validSimplePullRequestAction(args, "close"):
	case validSimplePullRequestAction(args, "reopen"):
	case validMergeAction(args):
	case validBodyPullRequestAction(args, "comment", "--body"):
	case validReviewAction(args):
	case validWorkflowAction(args):
	default:
		return fmt.Errorf("GitHub action invocation blocked: expected an exact supported gh pr or gh run action, refusing argument shape with %d elements", len(args))
	}
	return nil
}

func validSimplePullRequestAction(args []string, operation string) bool {
	switch {
	case len(args) != 3:
		return false
	case args[0] != "pr" || args[1] != operation:
		return false
	default:
		return validatePullRequestURL(args[2]) == nil
	}
}

func validMergeAction(args []string) bool {
	switch {
	case len(args) != 4 || args[0] != "pr" || args[1] != "merge":
		return false
	case args[3] != "--merge" && args[3] != "--squash" && args[3] != "--rebase":
		return false
	default:
		return validatePullRequestURL(args[2]) == nil
	}
}

func validBodyPullRequestAction(args []string, operation string, flag string) bool {
	switch {
	case len(args) != 5 || args[0] != "pr" || args[1] != operation || args[3] != flag:
		return false
	case validatePullRequestURL(args[2]) != nil || strings.TrimSpace(args[4]) == "" || len(args[4]) > maxGitHubActionBodyBytes:
		return false
	default:
		return true
	}
}

func validReviewAction(args []string) bool {
	switch {
	case len(args) == 4 && args[0] == "pr" && args[1] == "review" && args[3] == "--approve":
		return validatePullRequestURL(args[2]) == nil
	case len(args) == 6 && args[0] == "pr" && args[1] == "review" && (args[3] == "--comment" || args[3] == "--request-changes") && args[4] == "--body":
		return validatePullRequestURL(args[2]) == nil && strings.TrimSpace(args[5]) != "" && len(args[5]) <= maxGitHubActionBodyBytes
	default:
		return false
	}
}

func validWorkflowAction(args []string) bool {
	switch {
	case len(args) != 5 && len(args) != 6:
		return false
	case args[0] != "run" || (args[1] != "rerun" && args[1] != "cancel") || args[3] != "--repo":
		return false
	case validateRepositoryName(args[4]) != nil:
		return false
	case len(args) == 6 && (args[1] != "rerun" || args[5] != "--failed"):
		return false
	default:
	}
	runID, err := strconv.ParseInt(args[2], 10, 64)
	switch {
	case err != nil || runID <= 0:
		return false
	default:
		return true
	}
}
