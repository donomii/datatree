package githubdriver

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"datahumper/drivers_go/driverlib"
)

type mountState struct {
	View       string `json:"view,omitempty"`
	URL        string `json:"url,omitempty"`
	Repository string `json:"repository,omitempty"`
	Branch     string `json:"branch,omitempty"`
	State      string `json:"state,omitempty"`
	IsDraft    bool   `json:"isDraft,omitempty"`
	RunID      int64  `json:"runId,omitempty"`
}

type repositorySummary struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type pullRequestSummary struct {
	IsDraft    bool              `json:"isDraft"`
	Number     int64             `json:"number"`
	Repository repositorySummary `json:"repository"`
	State      string            `json:"state"`
	Title      string            `json:"title"`
	UpdatedAt  string            `json:"updatedAt"`
	URL        string            `json:"url"`
}

const viewRoot = "root"
const viewAuthored = "authored"
const viewReviewRequested = "review-requested"
const viewPullRequest = "pull-request"
const viewDiff = "diff"
const viewActions = "actions"
const viewRuns = "runs"
const viewRun = "run"
const viewRunActions = "run-actions"

const authoredEntryName = "[open pull requests I authored]"
const reviewRequestedEntryName = "[open pull requests requesting my review]"
const diffEntryName = "[diff]"
const actionsEntryName = "[actions]"
const runsEntryName = "[CI/CD runs]"
const githubSearchLimit = 1000
const maxGitHubMountStateBytes = 16 * 1024
const maxGitHubURLBytes = 4096

const searchJSONFields = "isDraft,number,repository,state,title,updatedAt,url"
const pullRequestJSONFields = "additions,assignees,author,baseRefName,body,changedFiles,closedAt,comments,commits,createdAt,deletions,files,headRefName,isDraft,labels,latestReviews,mergeStateStatus,mergeable,mergedAt,mergedBy,milestone,number,reviewDecision,reviewRequests,state,statusCheckRollup,title,updatedAt,url"

func Run() {
	driverlib.RunProvider("GitHub driver", buildGitHubView)
}

func buildGitHubView(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case len(raw) > maxGitHubMountStateBytes:
		return nil, nil, fmt.Errorf("cannot parse the GitHub mount state: received %d bytes, but mount states are limited to %d bytes", len(raw), maxGitHubMountStateBytes)
	default:
	}
	state, stateErr := parseMountState(raw)
	switch {
	case stateErr != nil:
		return nil, nil, stateErr
	default:
	}
	config, configErr := resolveGitHubConfig()
	switch {
	case configErr != nil:
		return nil, nil, configErr
	default:
	}
	switch state.View {
	case viewRoot:
		return listRoot()
	case viewAuthored, viewReviewRequested:
		return listPullRequests(config, state.View)
	case viewPullRequest:
		return listPullRequest(config, state.URL)
	case viewDiff:
		return listPullRequestDiff(config, state.URL)
	case viewActions:
		return listPullRequestActions(state)
	case viewRuns:
		return listWorkflowRuns(config, state)
	case viewRun:
		return listWorkflowRun(config, state)
	case viewRunActions:
		return listWorkflowRunActions(state)
	default:
		return nil, nil, fmt.Errorf("cannot browse GitHub: unsupported view %q", state.View)
	}
}

func parseMountState(raw []byte) (mountState, error) {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "":
		trimmed = "{}"
	default:
	}
	var state mountState
	switch err := json.Unmarshal([]byte(trimmed), &state); {
	case err != nil:
		return mountState{}, fmt.Errorf("cannot parse the GitHub mount state: expected a JSON object with view and optional url fields, got %q with error: %v", trimmed, err)
	default:
	}
	switch {
	case state.View == "":
		state.View = viewRoot
	default:
	}
	switch err := validateMountState(state); {
	case err != nil:
		return mountState{}, err
	default:
		return state, nil
	}
}

func validateMountState(state mountState) error {
	switch state.View {
	case viewRoot, viewAuthored, viewReviewRequested:
		switch {
		case state.URL != "" || state.Repository != "" || state.Branch != "" || state.State != "" || state.IsDraft || state.RunID != 0:
			return fmt.Errorf("invalid GitHub mount state for view %q: expected no target fields, got url=%q repository=%q branch=%q state=%q isDraft=%t runId=%d", state.View, state.URL, state.Repository, state.Branch, state.State, state.IsDraft, state.RunID)
		default:
			return nil
		}
	case viewPullRequest, viewDiff:
		switch err := validatePullRequestURL(state.URL); {
		case err != nil:
			return err
		case state.Repository != "" || state.Branch != "" || state.State != "" || state.IsDraft || state.RunID != 0:
			return fmt.Errorf("invalid GitHub mount state for view %q: expected only url, got repository=%q branch=%q state=%q isDraft=%t runId=%d", state.View, state.Repository, state.Branch, state.State, state.IsDraft, state.RunID)
		default:
			return nil
		}
	case viewActions:
		switch err := validatePullRequestURL(state.URL); {
		case err != nil:
			return err
		case state.State != "OPEN" && state.State != "CLOSED" && state.State != "MERGED":
			return fmt.Errorf("invalid GitHub pull request action state %q: expected OPEN, CLOSED or MERGED for URL %q", state.State, state.URL)
		case state.Repository != "" || state.Branch != "" || state.RunID != 0:
			return fmt.Errorf("invalid GitHub mount state for actions: expected url, state and optional isDraft, got repository=%q branch=%q runId=%d", state.Repository, state.Branch, state.RunID)
		default:
			return nil
		}
	case viewRuns:
		switch err := validateRepositoryName(state.Repository); {
		case err != nil:
			return err
		case state.Branch == "" || len(state.Branch) > maxGitHubURLBytes || strings.ContainsAny(state.Branch, "\t\r\n"):
			return fmt.Errorf("invalid GitHub workflow branch %q for repository %q: expected a non-empty branch of at most %d bytes without control whitespace", state.Branch, state.Repository, maxGitHubURLBytes)
		case state.URL != "" || state.State != "" || state.IsDraft || state.RunID != 0:
			return fmt.Errorf("invalid GitHub mount state for runs: expected only repository and branch, got url=%q state=%q isDraft=%t runId=%d", state.URL, state.State, state.IsDraft, state.RunID)
		default:
			return nil
		}
	case viewRun, viewRunActions:
		switch err := validateRepositoryName(state.Repository); {
		case err != nil:
			return err
		case state.RunID <= 0:
			return fmt.Errorf("invalid GitHub workflow run id %d for repository %q: expected a positive integer", state.RunID, state.Repository)
		case state.View == viewRunActions && state.State != "queued" && state.State != "in_progress" && state.State != "completed":
			return fmt.Errorf("invalid GitHub workflow run action state %q for repository %q run %d: expected queued, in_progress or completed", state.State, state.Repository, state.RunID)
		case state.View == viewRun && state.State != "":
			return fmt.Errorf("invalid GitHub workflow run state field %q: the run detail view expects no state field", state.State)
		case state.URL != "" || state.Branch != "" || state.IsDraft:
			return fmt.Errorf("invalid GitHub mount state for view %q: got unexpected url=%q branch=%q isDraft=%t", state.View, state.URL, state.Branch, state.IsDraft)
		default:
			return nil
		}
	default:
		return fmt.Errorf("invalid GitHub mount state view %q: expected root, authored, review-requested, pull-request, diff, actions, runs, run or run-actions", state.View)
	}
}

func validatePullRequestURL(value string) error {
	parsed, err := url.Parse(value)
	switch {
	case value == "":
		return fmt.Errorf("invalid GitHub pull request URL: expected a non-empty HTTPS URL")
	case len(value) > maxGitHubURLBytes:
		return fmt.Errorf("invalid GitHub pull request URL: received %d bytes, but URLs are limited to %d bytes", len(value), maxGitHubURLBytes)
	case err != nil:
		return fmt.Errorf("invalid GitHub pull request URL %q: expected a well-formed HTTPS URL, got: %v", value, err)
	case parsed.Scheme != "https" || parsed.Host == "" || parsed.Path == "":
		return fmt.Errorf("invalid GitHub pull request URL %q: expected an HTTPS URL with a host and path", value)
	case strings.ContainsAny(value, "\t\r\n "):
		return fmt.Errorf("invalid GitHub pull request URL %q: URLs cannot contain whitespace", value)
	default:
		return nil
	}
}

func listRoot() ([]driverlib.Resource, map[string][]byte, error) {
	authoredState, authoredErr := encodeMountState(mountState{View: viewAuthored})
	switch {
	case authoredErr != nil:
		return nil, nil, authoredErr
	default:
	}
	reviewState, reviewErr := encodeMountState(mountState{View: viewReviewRequested})
	switch {
	case reviewErr != nil:
		return nil, nil, reviewErr
	default:
	}
	resources := []driverlib.Resource{
		{Name: authoredEntryName, Kind: "github", Size: 0},
		{Name: reviewRequestedEntryName, Kind: "github", Size: 0},
	}
	children := map[string][]byte{
		authoredEntryName:        authoredState,
		reviewRequestedEntryName: reviewState,
	}
	return resources, children, nil
}

func listPullRequests(config githubConfig, view string) ([]driverlib.Resource, map[string][]byte, error) {
	args := authoredSearchArgs()
	switch view {
	case viewAuthored:
	case viewReviewRequested:
		args = reviewRequestedSearchArgs()
	default:
		return nil, nil, fmt.Errorf("cannot list GitHub pull requests: unsupported list view %q", view)
	}
	out, err := runReadOnlyGitHub(config, args)
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var summaries []pullRequestSummary
	switch err := json.Unmarshal(out, &summaries); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse GitHub pull request search output for view %q: expected a JSON array, got %d bytes with error: %v", view, len(out), err)
	default:
	}
	sort.Slice(summaries, func(left int, right int) bool {
		leftName := summaries[left].Repository.NameWithOwner + fmt.Sprintf("#%020d", summaries[left].Number)
		rightName := summaries[right].Repository.NameWithOwner + fmt.Sprintf("#%020d", summaries[right].Number)
		return leftName < rightName
	})
	var resources []driverlib.Resource
	children := map[string][]byte{}
	for _, summary := range summaries {
		switch err := validateSummary(summary); {
		case err != nil:
			return nil, nil, err
		default:
		}
		name := pullRequestName(summary)
		state, stateErr := encodeMountState(mountState{View: viewPullRequest, URL: summary.URL})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: name, Kind: "github", Size: 0})
		children[name] = state
	}
	switch {
	case len(summaries) >= githubSearchLimit:
		message := fmt.Sprintf("GitHub returned %d pull requests, its maximum search window; some results may be unavailable", githubSearchLimit)
		resources = append(resources, driverlib.Resource{Name: message, Kind: "error", Size: 0})
	default:
	}
	return driverlib.RetainSource(resources, children, out)
}

func validateSummary(summary pullRequestSummary) error {
	switch {
	case summary.Repository.NameWithOwner == "":
		return fmt.Errorf("cannot list GitHub pull requests: expected repository.nameWithOwner, got an empty value for pull request URL %q", summary.URL)
	case summary.Number <= 0:
		return fmt.Errorf("cannot list GitHub pull requests: expected a positive number for repository %q, got %d", summary.Repository.NameWithOwner, summary.Number)
	case summary.Title == "":
		return fmt.Errorf("cannot list GitHub pull requests: expected a non-empty title for %s#%d", summary.Repository.NameWithOwner, summary.Number)
	default:
		return validatePullRequestURL(summary.URL)
	}
}

func pullRequestName(summary pullRequestSummary) string {
	draft := ""
	switch {
	case summary.IsDraft:
		draft = " [draft]"
	default:
	}
	return summary.Repository.NameWithOwner + " #" + strconv.FormatInt(summary.Number, 10) + draft + " " + summary.Title
}

func listPullRequest(config githubConfig, pullRequestURL string) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyGitHub(config, pullRequestViewArgs(pullRequestURL))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var detail struct {
		HeadRefName string `json:"headRefName"`
		IsDraft     bool   `json:"isDraft"`
		State       string `json:"state"`
	}
	switch err := json.Unmarshal(out, &detail); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse GitHub pull request detail for %q: expected a JSON object with state and headRefName, got %d bytes with error: %v", pullRequestURL, len(out), err)
	default:
	}
	repository, repositoryErr := repositoryFromPullRequestURL(pullRequestURL)
	switch {
	case repositoryErr != nil:
		return nil, nil, repositoryErr
	default:
	}
	resources, children, listErr := driverlib.ListGenericJSON(out)
	switch {
	case listErr != nil:
		return nil, nil, listErr
	default:
	}
	actionsState, actionsErr := encodeMountState(mountState{View: viewActions, URL: pullRequestURL, State: detail.State, IsDraft: detail.IsDraft})
	switch {
	case actionsErr != nil:
		return nil, nil, actionsErr
	default:
	}
	runsState, runsErr := encodeMountState(mountState{View: viewRuns, Repository: repository, Branch: detail.HeadRefName})
	switch {
	case runsErr != nil:
		return nil, nil, runsErr
	default:
	}
	diffState, stateErr := encodeMountState(mountState{View: viewDiff, URL: pullRequestURL})
	switch {
	case stateErr != nil:
		return nil, nil, stateErr
	default:
	}
	resources = append([]driverlib.Resource{{Name: actionsEntryName, Kind: "github", Size: 0}, {Name: runsEntryName, Kind: "github", Size: 0}, {Name: diffEntryName, Kind: "github", Size: 0}}, resources...)
	children[actionsEntryName] = actionsState
	children[runsEntryName] = runsState
	children[diffEntryName] = diffState
	return driverlib.RetainSource(resources, children, out)
}

func listPullRequestDiff(config githubConfig, pullRequestURL string) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyGitHub(config, pullRequestDiffArgs(pullRequestURL))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	files := splitUnifiedDiff(out)
	resources, children := mergeUnifiedDiffFiles(files)
	return resources, children, nil
}

func mergeUnifiedDiffFiles(files [][]byte) ([]driverlib.Resource, map[string][]byte) {
	resources := make([]driverlib.Resource, 0, len(files))
	children := map[string][]byte{}
	resourceIndexes := map[string]int{}
	for index, file := range files {
		name := unifiedDiffFileName(file, index)
		switch resourceIndex, duplicate := resourceIndexes[name]; {
		case duplicate:
			combined := make([]byte, 0, len(children[name])+1+len(file))
			combined = append(combined, children[name]...)
			combined = append(combined, '\n')
			combined = append(combined, file...)
			children[name] = combined
			resources[resourceIndex].Size = int64(len(combined))
		default:
			resourceIndexes[name] = len(resources)
			resources = append(resources, driverlib.Resource{Name: name, Kind: "diff", Size: int64(len(file))})
			children[name] = file
		}
	}
	return resources, children
}

func splitUnifiedDiff(raw []byte) [][]byte {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	var files [][]byte
	var current []string
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			switch {
			case len(current) > 0:
				files = append(files, []byte(strings.Join(current, "\n")))
			default:
			}
			current = []string{line}
		case current != nil:
			current = append(current, line)
		default:
		}
	}
	switch {
	case len(current) > 0:
		files = append(files, []byte(strings.TrimSuffix(strings.Join(current, "\n"), "\n")))
	default:
	}
	return files
}

func unifiedDiffFileName(file []byte, index int) string {
	lines := strings.Split(string(file), "\n")
	for _, prefix := range []string{"+++ ", "rename to ", "--- ", "rename from "} {
		for _, line := range lines {
			switch {
			case strings.HasPrefix(line, prefix):
				name := cleanUnifiedDiffPath(strings.TrimPrefix(line, prefix))
				switch {
				case name != "" && name != "/dev/null":
					return name
				default:
				}
			default:
			}
		}
	}
	return fmt.Sprintf("[changed file %d]", index+1)
}

func cleanUnifiedDiffPath(raw string) string {
	name := strings.TrimSpace(strings.SplitN(raw, "\t", 2)[0])
	switch {
	case strings.HasPrefix(name, "\""):
		decoded, err := strconv.Unquote(name)
		switch {
		case err == nil:
			name = decoded
		default:
		}
	default:
	}
	switch {
	case strings.HasPrefix(name, "a/") || strings.HasPrefix(name, "b/"):
		return name[2:]
	default:
		return name
	}
}

func encodeMountState(state mountState) ([]byte, error) {
	encoded, err := json.Marshal(state)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot encode GitHub mount state for view %q URL %q: %v", state.View, state.URL, err)
	default:
		return encoded, nil
	}
}
