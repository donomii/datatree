package githubdriver

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"datahumper/drivers_go/driverlib"
)

type workflowRunSummary struct {
	Attempt      int64  `json:"attempt"`
	Conclusion   string `json:"conclusion"`
	DatabaseID   int64  `json:"databaseId"`
	DisplayTitle string `json:"displayTitle"`
	Name         string `json:"name"`
	Number       int64  `json:"number"`
	Status       string `json:"status"`
	WorkflowName string `json:"workflowName"`
}

type actionTarget struct {
	URL        string `json:"url,omitempty"`
	Repository string `json:"repository,omitempty"`
	RunID      int64  `json:"runId,omitempty"`
}

const actionClose = "github-action-close"
const actionReopen = "github-action-reopen"
const actionMerge = "github-action-merge"
const actionComment = "github-action-comment"
const actionApprove = "github-action-approve"
const actionReviewComment = "github-action-review-comment"
const actionRequestChanges = "github-action-request-changes"
const actionRunRerun = "github-action-run-rerun"
const actionRunRerunFailed = "github-action-run-rerun-failed"
const actionRunCancel = "github-action-run-cancel"

const runActionsEntryName = "[actions]"
const githubRunLimit = 100
const workflowRunJSONFields = "attempt,conclusion,createdAt,databaseId,displayTitle,event,headBranch,headSha,name,number,startedAt,status,updatedAt,url,workflowDatabaseId,workflowName"
const workflowRunViewJSONFields = "attempt,conclusion,createdAt,databaseId,displayTitle,event,headBranch,headSha,jobs,name,number,startedAt,status,updatedAt,url,workflowDatabaseId,workflowName"

var repositoryNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func listPullRequestActions(state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	target, err := encodeActionTarget(actionTarget{URL: state.URL})
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var resources []driverlib.Resource
	children := map[string][]byte{}
	addActionResource := func(name string, kind string) {
		resources = append(resources, driverlib.Resource{Name: name, Kind: kind, Size: int64(len(target))})
		children[name] = target
	}
	addActionResource("Add comment", actionComment)
	switch state.State {
	case "OPEN":
		addActionResource("Close pull request", actionClose)
		switch {
		case state.IsDraft:
		default:
			addActionResource("Merge pull request", actionMerge)
		}
		addActionResource("Approve pull request", actionApprove)
		addActionResource("Submit review comment", actionReviewComment)
		addActionResource("Request changes", actionRequestChanges)
	case "CLOSED":
		addActionResource("Reopen pull request", actionReopen)
	case "MERGED":
	default:
		return nil, nil, fmt.Errorf("cannot list GitHub actions for pull request %q: expected state OPEN, CLOSED or MERGED, got %q", state.URL, state.State)
	}
	return resources, children, nil
}

func listWorkflowRuns(config githubConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyGitHub(config, workflowRunListArgs(state.Repository, state.Branch))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var runs []workflowRunSummary
	switch err := json.Unmarshal(out, &runs); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse GitHub workflow runs for repository %q branch %q: expected a JSON array, got %d bytes with error: %v", state.Repository, state.Branch, len(out), err)
	default:
	}
	sort.SliceStable(runs, func(left int, right int) bool {
		return runs[left].DatabaseID > runs[right].DatabaseID
	})
	resources := make([]driverlib.Resource, 0, len(runs))
	children := map[string][]byte{}
	for _, run := range runs {
		switch {
		case run.DatabaseID <= 0 || run.Number <= 0 || run.Status == "":
			return nil, nil, fmt.Errorf("cannot list GitHub workflow runs for repository %q branch %q: expected positive databaseId and number plus non-empty status, got %+v", state.Repository, state.Branch, run)
		default:
		}
		name := workflowRunName(run)
		runState, stateErr := encodeMountState(mountState{View: viewRun, Repository: state.Repository, RunID: run.DatabaseID})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: name, Kind: "github", Size: 0})
		children[name] = runState
	}
	return driverlib.RetainSource(resources, children, out)
}

func workflowRunName(run workflowRunSummary) string {
	workflow := run.WorkflowName
	switch {
	case workflow == "":
		workflow = run.Name
	default:
	}
	conclusion := ""
	switch {
	case run.Conclusion != "":
		conclusion = " " + run.Conclusion
	default:
	}
	attempt := ""
	switch {
	case run.Attempt > 1:
		attempt = " attempt " + strconv.FormatInt(run.Attempt, 10)
	default:
	}
	return workflow + " #" + strconv.FormatInt(run.Number, 10) + attempt + " " + run.Status + conclusion + " " + run.DisplayTitle
}

func listWorkflowRun(config githubConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyGitHub(config, workflowRunViewArgs(state.Repository, state.RunID))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var detail workflowRunSummary
	switch err := json.Unmarshal(out, &detail); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse GitHub workflow run %d for repository %q: expected a JSON object, got %d bytes with error: %v", state.RunID, state.Repository, len(out), err)
	case detail.Status != "queued" && detail.Status != "in_progress" && detail.Status != "completed":
		return nil, nil, fmt.Errorf("cannot browse GitHub workflow run %d for repository %q: expected status queued, in_progress or completed, got %q", state.RunID, state.Repository, detail.Status)
	default:
	}
	resources, children, listErr := driverlib.ListGenericJSON(out)
	switch {
	case listErr != nil:
		return nil, nil, listErr
	default:
	}
	actionsState, stateErr := encodeMountState(mountState{View: viewRunActions, Repository: state.Repository, RunID: state.RunID, State: detail.Status})
	switch {
	case stateErr != nil:
		return nil, nil, stateErr
	default:
	}
	resources = append([]driverlib.Resource{{Name: runActionsEntryName, Kind: "github", Size: 0}}, resources...)
	children[runActionsEntryName] = actionsState
	return driverlib.RetainSource(resources, children, out)
}

func listWorkflowRunActions(state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	target, err := encodeActionTarget(actionTarget{Repository: state.Repository, RunID: state.RunID})
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var resources []driverlib.Resource
	children := map[string][]byte{}
	addActionResource := func(name string, kind string) {
		resources = append(resources, driverlib.Resource{Name: name, Kind: kind, Size: int64(len(target))})
		children[name] = target
	}
	switch state.State {
	case "queued", "in_progress":
		addActionResource("Cancel workflow run", actionRunCancel)
	case "completed":
		addActionResource("Rerun all jobs", actionRunRerun)
		addActionResource("Rerun failed jobs", actionRunRerunFailed)
	default:
		return nil, nil, fmt.Errorf("cannot list GitHub workflow actions for repository %q run %d: expected queued, in_progress or completed, got %q", state.Repository, state.RunID, state.State)
	}
	return resources, children, nil
}

func repositoryFromPullRequestURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot derive GitHub repository from pull request URL %q: expected a valid URL, got: %v", value, err)
	default:
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	switch {
	case len(parts) < 4 || parts[2] != "pull":
		return "", fmt.Errorf("cannot derive GitHub repository from pull request URL %q: expected path OWNER/REPOSITORY/pull/NUMBER", value)
	default:
	}
	repository := parts[0] + "/" + parts[1]
	switch err := validateRepositoryName(repository); {
	case err != nil:
		return "", err
	default:
		return repository, nil
	}
}

func validateRepositoryName(repository string) error {
	switch {
	case !repositoryNamePattern.MatchString(repository):
		return fmt.Errorf("invalid GitHub repository %q: expected OWNER/REPOSITORY using letters, numbers, dot, underscore or hyphen", repository)
	default:
		return nil
	}
}

func encodeActionTarget(target actionTarget) ([]byte, error) {
	encoded, err := json.Marshal(target)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot encode GitHub action target url=%q repository=%q runId=%d: %v", target.URL, target.Repository, target.RunID, err)
	default:
		return encoded, nil
	}
}
