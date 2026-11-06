package githubdriver

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type githubConfig struct {
	executable     string
	maxOutputBytes int64
}

type boundedBuffer struct {
	data  bytes.Buffer
	limit int64
	total int64
}

const defaultGitHubMaxOutputBytes int64 = 8 * 1024 * 1024

func resolveGitHubConfig() (githubConfig, error) {
	candidate := os.Getenv("DATATREE_GH")
	switch {
	case candidate == "":
		candidate = "gh"
	default:
	}
	executable, err := exec.LookPath(candidate)
	switch {
	case err != nil:
		return githubConfig{}, fmt.Errorf("cannot find a usable GitHub CLI executable: looked for %q (set DATATREE_GH to override), got: %v", candidate, err)
	default:
	}
	maxOutputBytes, settingErr := positiveInt64Setting("DATATREE_GITHUB_MAX_OUTPUT_BYTES", defaultGitHubMaxOutputBytes)
	switch {
	case settingErr != nil:
		return githubConfig{}, settingErr
	default:
		return githubConfig{executable: executable, maxOutputBytes: maxOutputBytes}, nil
	}
}

func authoredSearchArgs() []string {
	return []string{"search", "prs", "--author=@me", "--state=open", "--limit=" + strconv.Itoa(githubSearchLimit), "--json=" + searchJSONFields}
}

func reviewRequestedSearchArgs() []string {
	return []string{"search", "prs", "--review-requested=@me", "--state=open", "--limit=" + strconv.Itoa(githubSearchLimit), "--json=" + searchJSONFields}
}

func pullRequestViewArgs(pullRequestURL string) []string {
	return []string{"pr", "view", pullRequestURL, "--json=" + pullRequestJSONFields}
}

func pullRequestDiffArgs(pullRequestURL string) []string {
	return []string{"pr", "diff", pullRequestURL, "--patch", "--color=never"}
}

func workflowRunListArgs(repository string, branch string) []string {
	return []string{"run", "list", "--repo", repository, "--branch", branch, "--limit=" + strconv.Itoa(githubRunLimit), "--json=" + workflowRunJSONFields}
}

func workflowRunViewArgs(repository string, runID int64) []string {
	return []string{"run", "view", strconv.FormatInt(runID, 10), "--repo", repository, "--json=" + workflowRunViewJSONFields}
}

func runReadOnlyGitHub(config githubConfig, args []string) ([]byte, error) {
	assertReadOnlyGitHubInvocation(args)
	cmd := exec.Command(config.executable, args...)
	stdout := boundedBuffer{limit: config.maxOutputBytes}
	stderr := boundedBuffer{limit: config.maxOutputBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	switch {
	case stdout.exceeded():
		return nil, fmt.Errorf("gh %s produced %d stdout bytes while reading GitHub data, exceeding DATATREE_GITHUB_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stdout.total, config.maxOutputBytes)
	case stderr.exceeded():
		return nil, fmt.Errorf("gh %s produced %d stderr bytes while reading GitHub data, exceeding DATATREE_GITHUB_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stderr.total, config.maxOutputBytes)
	case runErr != nil:
		return nil, fmt.Errorf("gh %s failed while reading GitHub data: expected a zero exit, got %v with %d stdout bytes: %s", strings.Join(args, " "), runErr, stdout.total, strings.TrimSpace(stderr.data.String()))
	default:
		return stdout.data.Bytes(), nil
	}
}

func assertReadOnlyGitHubInvocation(args []string) {
	switch {
	case stringSlicesEqual(args, authoredSearchArgs()):
	case stringSlicesEqual(args, reviewRequestedSearchArgs()):
	case validPullRequestViewInvocation(args):
	case validPullRequestDiffInvocation(args):
	case validWorkflowRunListInvocation(args):
	case validWorkflowRunViewInvocation(args):
	default:
		panic(fmt.Sprintf("GitHub CLI invocation blocked: expected an exact read-only gh search prs, gh pr view, gh pr diff, gh run list or gh run view command, refusing to run %q", strings.Join(args, " ")))
	}
}

func validWorkflowRunListInvocation(args []string) bool {
	switch {
	case len(args) != 8:
		return false
	case args[0] != "run" || args[1] != "list" || args[2] != "--repo" || args[4] != "--branch" || args[6] != "--limit="+strconv.Itoa(githubRunLimit) || args[7] != "--json="+workflowRunJSONFields:
		return false
	case validateRepositoryName(args[3]) != nil || args[5] == "" || strings.ContainsAny(args[5], "\t\r\n"):
		return false
	default:
		return true
	}
}

func validWorkflowRunViewInvocation(args []string) bool {
	switch {
	case len(args) != 6:
		return false
	case args[0] != "run" || args[1] != "view" || args[3] != "--repo" || args[5] != "--json="+workflowRunViewJSONFields:
		return false
	case validateRepositoryName(args[4]) != nil:
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

func validPullRequestDiffInvocation(args []string) bool {
	switch {
	case len(args) != 5:
		return false
	case args[0] != "pr" || args[1] != "diff" || args[3] != "--patch" || args[4] != "--color=never":
		return false
	default:
		return validatePullRequestURL(args[2]) == nil
	}
}

func validPullRequestViewInvocation(args []string) bool {
	switch {
	case len(args) != 4:
		return false
	case args[0] != "pr" || args[1] != "view" || args[3] != "--json="+pullRequestJSONFields:
		return false
	default:
		return validatePullRequestURL(args[2]) == nil
	}
}

func stringSlicesEqual(left []string, right []string) bool {
	switch {
	case len(left) != len(right):
		return false
	default:
	}
	for index := range left {
		switch {
		case left[index] != right[index]:
			return false
		default:
		}
	}
	return true
}

func positiveInt64Setting(name string, defaultValue int64) (int64, error) {
	raw := os.Getenv(name)
	switch {
	case raw == "":
		return defaultValue, nil
	default:
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	switch {
	case err != nil || value <= 0:
		return 0, fmt.Errorf("cannot configure GitHub browsing: expected %s to be a positive byte count, got %q", name, raw)
	default:
		return value, nil
	}
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	written := len(data)
	remaining := buffer.limit - int64(buffer.data.Len())
	switch {
	case remaining <= 0:
	case int64(len(data)) <= remaining:
		_, _ = buffer.data.Write(data)
	default:
		_, _ = buffer.data.Write(data[:remaining])
	}
	buffer.total += int64(written)
	return written, nil
}

func (buffer *boundedBuffer) exceeded() bool {
	return buffer.total > buffer.limit
}
