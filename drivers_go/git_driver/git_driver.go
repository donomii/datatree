package gitdriver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"datahumper/drivers_go/driverlib"
)

type MountState struct {
	Repository string `json:"repository"`
	View       string `json:"view,omitempty"`
	Revision   string `json:"revision,omitempty"`
}

type childEntry struct {
	state  []byte
	source fileSource
}

type fileSource struct {
	mode     string
	path     string
	revision string
}

const viewRepository = "repository"
const viewCommits = "commits"
const viewCommit = "commit"
const viewCommitted = "committed"
const viewUncommitted = "uncommitted"
const viewStaged = "staged"
const viewUnstaged = "unstaged"
const viewUntracked = "untracked"
const viewConflicted = "conflicted"

const sourceRevision = "revision"
const sourceIndex = "index"
const sourceWorktree = "worktree"

const maxGitMountStateBytes = 16 * 1024
const maxGitPathBytes = 4096

func Run() {
	defer driverlib.RecoverPanic("Git driver")

	enc := json.NewEncoder(os.Stdout)
	dec := json.NewDecoder(os.Stdin)
	stateBytes, fetchErr := driverlib.FetchUpstreamBytes(enc, dec)
	switch {
	case fetchErr != nil:
		logGitStartFailure(fetchErr)
		return
	case len(stateBytes) > maxGitMountStateBytes:
		driverlib.EmitError(enc, fmt.Sprintf("cannot parse the Git mount state: received %d bytes, but mount states are limited to %d bytes", len(stateBytes), maxGitMountStateBytes))
		return
	default:
	}
	state, stateErr := parseMountState(stateBytes)
	switch {
	case stateErr != nil:
		driverlib.EmitError(enc, stateErr.Error())
		return
	default:
	}
	config, configErr := resolveGitConfig(state.Repository)
	switch {
	case configErr != nil:
		driverlib.EmitError(enc, configErr.Error())
		return
	default:
	}
	state.Repository = config.repository
	resources, children, listErr := listLevel(config, state)
	switch {
	case listErr != nil:
		logGitReadFailure(state.Repository, state.View, listErr)
		driverlib.EmitError(enc, listErr.Error())
		return
	default:
	}
	for index := range resources {
		resource := &resources[index]
		switch {
		case resource.Payload != nil:
			key := fmt.Sprintf("\x00git:value:%d", index)
			children[key] = childEntry{state: resource.Payload}
			resource.Key, resource.Readable = key, true
		default:
			key := resource.Key
			switch {
			case key == "":
				key = resource.Name
			default:
			}
			resource.Readable = children[key].source.mode != ""
		}
	}
	switch err := enc.Encode(driverlib.ResourcesMsg{Resources: resources}); {
	case err != nil:
		logGitResourceEncodeFailure(state.Repository, state.View, len(resources), err)
		return
	default:
	}
	serveChildren(enc, dec, config, children)
}

func parseMountState(raw []byte) (MountState, error) {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "":
		return MountState{}, fmt.Errorf("cannot open a Git repository: expected an absolute repository path or a Git mount state, received no bytes")
	case strings.HasPrefix(trimmed, "{"):
		var state MountState
		switch err := json.Unmarshal([]byte(trimmed), &state); {
		case err != nil:
			return MountState{}, fmt.Errorf("cannot parse the Git mount state: expected repository, view and optional revision fields, got %q with error: %v", trimmed, err)
		default:
		}
		switch {
		case state.View == "":
			state.View = viewRepository
		default:
		}
		switch err := validateMountState(state); {
		case err != nil:
			return MountState{}, err
		default:
			return state, nil
		}
	default:
		state := MountState{Repository: trimmed, View: viewRepository}
		switch err := validateMountState(state); {
		case err != nil:
			return MountState{}, err
		default:
			return state, nil
		}
	}
}

func validateMountState(state MountState) error {
	switch {
	case state.Repository == "":
		return fmt.Errorf("invalid Git mount state: expected a non-empty repository path")
	case !filepath.IsAbs(state.Repository):
		return fmt.Errorf("invalid Git repository path %q: expected an absolute path", state.Repository)
	case len(state.Repository) > maxGitPathBytes:
		return fmt.Errorf("invalid Git repository path: received %d bytes, but paths are limited to %d bytes", len(state.Repository), maxGitPathBytes)
	case strings.ContainsRune(state.Repository, '\x00'):
		return fmt.Errorf("invalid Git repository path %q: paths cannot contain a NUL byte", state.Repository)
	default:
	}
	switch state.View {
	case viewRepository, viewCommits, viewCommitted, viewUncommitted, viewStaged, viewUnstaged, viewUntracked, viewConflicted:
		switch {
		case state.Revision != "":
			return fmt.Errorf("invalid Git mount state for view %q: revision must be empty, got %q", state.View, state.Revision)
		default:
			return nil
		}
	case viewCommit:
		switch {
		case !validObjectID(state.Revision):
			return fmt.Errorf("invalid Git commit revision %q: expected a 40- or 64-character hexadecimal object id", state.Revision)
		default:
			return nil
		}
	default:
		return fmt.Errorf("invalid Git mount state view %q: expected repository, commits, commit, committed, uncommitted, staged, unstaged, untracked or conflicted", state.View)
	}
}

func listLevel(config gitConfig, state MountState) ([]driverlib.Resource, map[string]childEntry, error) {
	switch state.View {
	case viewRepository:
		return listRepository(config)
	case viewCommits:
		return listCommits(config)
	case viewCommit:
		return listCommit(config, state.Revision)
	case viewCommitted:
		return listCommittedFiles(config)
	case viewUncommitted, viewStaged, viewUnstaged, viewUntracked, viewConflicted:
		return listWorkingTreeState(config, state.View)
	default:
		return nil, nil, fmt.Errorf("cannot browse Git repository %s: unsupported view %q", config.repository, state.View)
	}
}

func listRepository(config gitConfig) ([]driverlib.Resource, map[string]childEntry, error) {
	branch, head, subject, headExists, headErr := readHead(config)
	switch {
	case headErr != nil:
		return nil, nil, headErr
	default:
	}
	resources := []driverlib.Resource{
		driverlib.KV("repository", config.repository),
		driverlib.KV("branch", branch),
	}
	switch {
	case headExists:
		resources = append(resources, driverlib.KV("HEAD", head+" "+subject))
	default:
		resources = append(resources, driverlib.KV("HEAD", "unborn; this repository has no commits"))
	}
	resources = append(resources, driverlib.KV("limits", fmt.Sprintf("at most %d rows and %d bytes from each Git command; change DATATREE_GIT_MAX_RESULTS or DATATREE_GIT_MAX_OUTPUT_BYTES to adjust them", config.maxResults, config.maxOutputBytes)))
	children := map[string]childEntry{}
	sections := []struct {
		name string
		view string
	}{
		{name: "[commits]", view: viewCommits},
		{name: "[committed files at HEAD]", view: viewCommitted},
		{name: "[all uncommitted changes]", view: viewUncommitted},
		{name: "[staged changes]", view: viewStaged},
		{name: "[unstaged changes]", view: viewUnstaged},
		{name: "[untracked files]", view: viewUntracked},
		{name: "[conflicted files]", view: viewConflicted},
	}
	for _, section := range sections {
		state, stateErr := encodeMountState(MountState{Repository: config.repository, View: section.view})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: section.name, Kind: "git", Size: 0})
		children[section.name] = childEntry{state: state}
	}
	return resources, children, nil
}

func listCommits(config gitConfig) ([]driverlib.Resource, map[string]childEntry, error) {
	out, err := runGit(config, []string{"log", "-z", "--max-count=" + strconv.Itoa(config.maxResults+1), "--date=iso-strict", "--format=%H%x1f%h%x1f%aI%x1f%an%x1f%s"})
	switch {
	case isMissingHeadError(err):
		return nil, map[string]childEntry{}, nil
	case err != nil:
		return nil, nil, err
	default:
	}
	records := splitNUL(out)
	totalShown := len(records)
	truncated := totalShown > config.maxResults
	switch {
	case truncated:
		records = records[:config.maxResults]
	default:
	}
	resources := make([]driverlib.Resource, 0, len(records)+1)
	children := map[string]childEntry{}
	for _, record := range records {
		fields := strings.SplitN(record, "\x1f", 5)
		switch {
		case len(fields) != 5 || !validObjectID(fields[0]):
			return nil, nil, fmt.Errorf("cannot parse Git history for repository %s: expected five fields with a full object id, got %q", config.repository, record)
		default:
		}
		name := fmt.Sprintf("%s %s %s — %s", fields[1], fields[2], fields[3], fields[4])
		state, stateErr := encodeMountState(MountState{Repository: config.repository, View: viewCommit, Revision: fields[0]})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: name, Kind: "git", Size: 0})
		children[name] = childEntry{state: state}
	}
	switch {
	case truncated:
		resources = append(resources, driverlib.Resource{Name: fmt.Sprintf("showing the first %d commits; increase DATATREE_GIT_MAX_RESULTS to see more", config.maxResults), Kind: "error", Size: 0})
	default:
	}
	return resources, children, nil
}

func listCommit(config gitConfig, revision string) ([]driverlib.Resource, map[string]childEntry, error) {
	out, err := runGit(config, []string{"show", "-s", "--date=iso-strict", "--format=%H%x1f%P%x1f%aI%x1f%an%x1f%ae%x1f%s", revision})
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	fields := strings.SplitN(strings.TrimSuffix(string(out), "\n"), "\x1f", 6)
	switch {
	case len(fields) != 6 || fields[0] != revision:
		return nil, nil, fmt.Errorf("cannot parse Git commit %s in repository %s: expected six metadata fields, got %q", revision, config.repository, string(out))
	default:
	}
	parents := fields[1]
	switch {
	case parents == "":
		parents = "none; root commit"
	default:
	}
	resources := []driverlib.Resource{
		driverlib.KV("commit", fields[0]),
		driverlib.KV("parents", parents),
		driverlib.KV("author", fields[3]+" <"+fields[4]+">"),
		driverlib.KV("date", fields[2]),
		driverlib.KV("subject", fields[5]),
	}
	changes, changeErr := runGit(config, []string{"diff-tree", "--root", "--no-commit-id", "--name-status", "-r", "-z", "--no-renames", revision})
	switch {
	case changeErr != nil:
		return nil, nil, changeErr
	default:
	}
	changedResources, children, parseErr := resourcesForNameStatus(config, changes, sourceRevision, revision)
	switch {
	case parseErr != nil:
		return nil, nil, parseErr
	default:
	}
	resources = append(resources, changedResources...)
	return resources, children, nil
}

func listCommittedFiles(config gitConfig) ([]driverlib.Resource, map[string]childEntry, error) {
	out, err := runGit(config, []string{"ls-tree", "-r", "-z", "-l", "HEAD"})
	switch {
	case isMissingHeadError(err):
		return nil, map[string]childEntry{}, nil
	case err != nil:
		return nil, nil, err
	default:
	}
	records := splitNUL(out)
	total := len(records)
	switch {
	case len(records) > config.maxResults:
		records = records[:config.maxResults]
	default:
	}
	resources := make([]driverlib.Resource, 0, len(records)+1)
	children := map[string]childEntry{}
	for _, record := range records {
		meta, path, found := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		switch {
		case !found || len(fields) != 4:
			return nil, nil, fmt.Errorf("cannot parse committed file entry in repository %s: expected mode, type, object id, size and path, got %q", config.repository, record)
		case fields[1] != "blob":
			resources = append(resources, driverlib.Resource{Name: "submodule: " + path, Kind: "kv", Size: 0})
			continue
		default:
		}
		size, sizeErr := strconv.ParseInt(fields[3], 10, 64)
		switch {
		case sizeErr != nil || size < 0:
			return nil, nil, fmt.Errorf("cannot parse committed file size in repository %s: expected a non-negative integer, got %q for %q", config.repository, fields[3], path)
		default:
		}
		resources = append(resources, driverlib.Resource{Name: path, Kind: driverlib.KindForName(path), Size: size})
		children[path] = childEntry{source: fileSource{mode: sourceRevision, path: path, revision: "HEAD"}}
	}
	return appendResultLimitNotice(resources, total, config.maxResults, "committed files at HEAD"), children, nil
}

func listWorkingTreeState(config gitConfig, view string) ([]driverlib.Resource, map[string]childEntry, error) {
	out, err := runGit(config, []string{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames"})
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	records := splitNUL(out)
	resources := []driverlib.Resource{}
	children := map[string]childEntry{}
	total := 0
	for _, record := range records {
		status, path, parseErr := parseStatusRecord(config.repository, record)
		switch {
		case parseErr != nil:
			return nil, nil, parseErr
		case !statusMatchesView(status, view):
			continue
		default:
		}
		total++
		switch {
		case len(resources) >= config.maxResults:
			continue
		default:
		}
		label := statusLabel(status, view)
		name := label + ": " + path
		source := sourceForView(config.repository, view, status, path)
		switch {
		case source.mode == "":
			resources = append(resources, driverlib.Resource{Name: name, Kind: "kv", Size: 0})
		default:
			size := workingFileSize(config.repository, source)
			resources = append(resources, driverlib.Resource{Name: name, Kind: driverlib.KindForName(path), Size: size})
			children[name] = childEntry{source: source}
		}
	}
	return appendResultLimitNotice(resources, total, config.maxResults, view+" files"), children, nil
}

func resourcesForNameStatus(config gitConfig, raw []byte, sourceMode string, revision string) ([]driverlib.Resource, map[string]childEntry, error) {
	fields := splitNUL(raw)
	resources := []driverlib.Resource{}
	children := map[string]childEntry{}
	total := len(fields) / 2
	switch {
	case len(fields)%2 != 0:
		return nil, nil, fmt.Errorf("cannot parse changed files in repository %s: expected status/path pairs, got %d NUL-separated fields", config.repository, len(fields))
	default:
	}
	for index := 0; index < len(fields) && len(resources) < config.maxResults; index += 2 {
		status := fields[index]
		path := fields[index+1]
		label := singleStatusLabel(status)
		name := label + ": " + path
		switch {
		case status == "D":
			resources = append(resources, driverlib.Resource{Name: name, Kind: "kv", Size: 0})
		default:
			resources = append(resources, driverlib.Resource{Name: name, Kind: driverlib.KindForName(path), Size: -1})
			children[name] = childEntry{source: fileSource{mode: sourceMode, path: path, revision: revision}}
		}
	}
	switch {
	case total > config.maxResults:
		resources = resources[:config.maxResults]
	default:
	}
	return appendResultLimitNotice(resources, total, config.maxResults, "files changed by this commit"), children, nil
}

func appendResultLimitNotice(resources []driverlib.Resource, total int, limit int, description string) []driverlib.Resource {
	switch {
	case total > limit:
		return append(resources, driverlib.Resource{Name: fmt.Sprintf("showing the first %d of %d %s; increase DATATREE_GIT_MAX_RESULTS to see more", limit, total, description), Kind: "error", Size: 0})
	default:
		return resources
	}
}

func encodeMountState(state MountState) ([]byte, error) {
	encoded, err := json.Marshal(state)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot encode Git mount state for repository %q view %q revision %q: %v", state.Repository, state.View, state.Revision, err)
	default:
		return encoded, nil
	}
}

func serveChildren(enc *json.Encoder, dec *json.Decoder, config gitConfig, children map[string]childEntry) {
	for {
		var req driverlib.RangeReq
		switch err := dec.Decode(&req); {
		case err != nil:
			return
		default:
		}
		switch {
		case req.Cmd != "range":
			continue
		default:
		}
		child, known := children[req.Child]
		switch {
		case req.Child == "" || !known:
			driverlib.SendRange(enc, nil)
		case len(child.state) > 0:
			driverlib.SendRange(enc, driverlib.SliceRange(child.state, req.Offset, req.Length))
		default:
			data, readErr := readSource(config, child.source)
			switch {
			case readErr != nil:
				logGitFileReadFailure(config.repository, child.source.mode, child.source.path, readErr)
				driverlib.SendRangeError(enc, readErr.Error())
			default:
				driverlib.SendRange(enc, driverlib.SliceRange(data, req.Offset, req.Length))
			}
		}
	}
}

func readSource(config gitConfig, source fileSource) ([]byte, error) {
	switch source.mode {
	case sourceRevision:
		return runGitFile(config, []string{"show", "--no-textconv", source.revision + ":" + source.path})
	case sourceIndex:
		return runGitFile(config, []string{"show", "--no-textconv", ":" + source.path})
	case sourceWorktree:
		return readWorkingFile(config, source.path)
	default:
		return nil, fmt.Errorf("cannot read Git file %q in repository %s: unsupported source mode %q", source.path, config.repository, source.mode)
	}
}

func readWorkingFile(config gitConfig, path string) ([]byte, error) {
	fullPath, pathErr := safeWorkingPath(config.repository, path)
	switch {
	case pathErr != nil:
		return nil, pathErr
	default:
	}
	info, err := os.Lstat(fullPath)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot read working-tree file %q from repository %s: expected the path to exist, got: %v", path, config.repository, err)
	case info.Mode()&os.ModeSymlink != 0:
		target, targetErr := os.Readlink(fullPath)
		switch {
		case targetErr != nil:
			return nil, fmt.Errorf("cannot read working-tree symbolic link %q from repository %s: %v", path, config.repository, targetErr)
		default:
			return []byte(target), nil
		}
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("cannot read working-tree path %q from repository %s: expected a regular file or symbolic link, got mode %s", path, config.repository, info.Mode())
	default:
	}
	data, readErr := os.ReadFile(fullPath)
	switch {
	case readErr != nil:
		return nil, fmt.Errorf("cannot read working-tree file %q from repository %s: %v", path, config.repository, readErr)
	default:
		return data, nil
	}
}

func safeWorkingPath(repository string, path string) (string, error) {
	fullPath := filepath.Join(repository, filepath.FromSlash(path))
	switch {
	case !driverlib.InsideBase(repository, fullPath):
		return "", fmt.Errorf("cannot read working-tree file %q from repository %s: the lexical path %s is outside the repository", path, repository, fullPath)
	default:
	}
	resolvedRepository, repositoryErr := filepath.EvalSymlinks(repository)
	switch {
	case repositoryErr != nil:
		return "", fmt.Errorf("cannot read working-tree file %q from repository %s: cannot resolve the repository path: %v", path, repository, repositoryErr)
	default:
	}
	resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(fullPath))
	switch {
	case parentErr != nil:
		return "", fmt.Errorf("cannot read working-tree file %q from repository %s: cannot resolve parent directory %s: %v", path, repository, filepath.Dir(fullPath), parentErr)
	case !driverlib.InsideBase(resolvedRepository, resolvedParent):
		return "", fmt.Errorf("cannot read working-tree file %q from repository %s: parent directory resolves to %s outside repository %s", path, repository, resolvedParent, resolvedRepository)
	default:
		return fullPath, nil
	}
}
