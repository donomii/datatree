package gitdriver

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type gitConfig struct {
	repository     string
	executable     string
	maxResults     int
	maxOutputBytes int64
}

type boundedBuffer struct {
	data  bytes.Buffer
	limit int64
	total int64
}

const defaultGitMaxResults = 500
const defaultGitMaxOutputBytes int64 = 8 * 1024 * 1024

var readOnlyGitCommands = map[string]bool{
	"diff-tree":    true,
	"log":          true,
	"ls-tree":      true,
	"rev-parse":    true,
	"show":         true,
	"status":       true,
	"symbolic-ref": true,
}

func resolveGitConfig(repository string) (gitConfig, error) {
	executable, executableErr := resolveGit()
	switch {
	case executableErr != nil:
		return gitConfig{}, executableErr
	default:
	}
	maxResults, resultsErr := positiveIntSetting("DATATREE_GIT_MAX_RESULTS", defaultGitMaxResults)
	switch {
	case resultsErr != nil:
		return gitConfig{}, resultsErr
	default:
	}
	maxOutputBytes, outputErr := positiveInt64Setting("DATATREE_GIT_MAX_OUTPUT_BYTES", defaultGitMaxOutputBytes)
	switch {
	case outputErr != nil:
		return gitConfig{}, outputErr
	default:
	}
	config := gitConfig{repository: filepath.Clean(repository), executable: executable, maxResults: maxResults, maxOutputBytes: maxOutputBytes}
	topLevel, topLevelErr := runGit(config, []string{"rev-parse", "--show-toplevel"})
	switch {
	case topLevelErr != nil:
		return gitConfig{}, fmt.Errorf("cannot open Git repository %q: expected a non-bare working tree, got: %v", repository, topLevelErr)
	default:
	}
	resolvedTopLevel := filepath.Clean(strings.TrimSpace(string(topLevel)))
	switch {
	case !filepath.IsAbs(resolvedTopLevel):
		return gitConfig{}, fmt.Errorf("cannot open Git repository %q: git rev-parse returned non-absolute top-level path %q", repository, resolvedTopLevel)
	default:
		config.repository = resolvedTopLevel
		return config, nil
	}
}

func resolveGit() (string, error) {
	candidate := os.Getenv("DATATREE_GIT")
	switch {
	case candidate == "":
		candidate = "git"
	default:
	}
	path, err := exec.LookPath(candidate)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot find a usable Git executable: looked for %q (set DATATREE_GIT to override), got: %v", candidate, err)
	default:
		return path, nil
	}
}

func runGit(config gitConfig, args []string) ([]byte, error) {
	assertReadOnlyGitInvocation(args)
	commandArgs := []string{"--no-pager", "--no-optional-locks", "-c", "core.fsmonitor=false", "-C", config.repository}
	commandArgs = append(commandArgs, args...)
	cmd := exec.Command(config.executable, commandArgs...)
	cmd.Env = gitEnvironment()
	stdout := boundedBuffer{limit: config.maxOutputBytes}
	stderr := boundedBuffer{limit: config.maxOutputBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	switch {
	case stdout.exceeded():
		return nil, fmt.Errorf("Git %s produced %d stdout bytes while reading repository %s, exceeding DATATREE_GIT_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stdout.total, config.repository, config.maxOutputBytes)
	case stderr.exceeded():
		return nil, fmt.Errorf("Git %s produced %d stderr bytes while reading repository %s, exceeding DATATREE_GIT_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stderr.total, config.repository, config.maxOutputBytes)
	case runErr != nil:
		return nil, fmt.Errorf("Git %s failed while reading repository %s: expected a zero exit, got %w with %d stdout bytes and stderr %q", strings.Join(args, " "), config.repository, runErr, stdout.total, strings.TrimSpace(stderr.data.String()))
	default:
		return stdout.data.Bytes(), nil
	}
}

func assertReadOnlyGitInvocation(args []string) {
	switch {
	case len(args) == 0:
		panic("Git invocation blocked: expected a read-only subcommand, got no arguments")
	case !readOnlyGitCommands[args[0]]:
		panic(fmt.Sprintf("Git invocation blocked: subcommand %q is not in the read-only allowlist; refusing to run %q", args[0], strings.Join(args, " ")))
	default:
	}
}

func gitEnvironment() []string {
	environment := os.Environ()
	environment = replaceEnvironment(environment, "GIT_OPTIONAL_LOCKS", "0")
	environment = replaceEnvironment(environment, "GIT_PAGER", "cat")
	return replaceEnvironment(environment, "LC_ALL", "C")
}

func replaceEnvironment(environment []string, name string, value string) []string {
	prefix := name + "="
	replaced := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		switch {
		case strings.HasPrefix(entry, prefix):
			continue
		default:
			replaced = append(replaced, entry)
		}
	}
	return append(replaced, prefix+value)
}

func positiveIntSetting(name string, defaultValue int) (int, error) {
	raw := os.Getenv(name)
	switch {
	case raw == "":
		return defaultValue, nil
	default:
	}
	value, err := strconv.Atoi(raw)
	switch {
	case err != nil || value <= 0:
		return 0, fmt.Errorf("cannot configure Git browsing: expected %s to be a positive integer, got %q", name, raw)
	default:
		return value, nil
	}
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
		return 0, fmt.Errorf("cannot configure Git browsing: expected %s to be a positive byte count, got %q", name, raw)
	default:
		return value, nil
	}
}

func readHead(config gitConfig) (string, string, string, bool, error) {
	branchOut, branchErr := runGit(config, []string{"symbolic-ref", "--quiet", "--short", "HEAD"})
	headOut, headErr := runGit(config, []string{"rev-parse", "--verify", "--short=12", "HEAD"})
	switch {
	case branchErr == nil:
		branchOut = bytes.TrimSpace(branchOut)
	case headErr == nil:
		branchOut = []byte("detached")
	default:
		return "", "", "", false, fmt.Errorf("cannot read Git branch for repository %s: symbolic-ref failed with %v and HEAD lookup failed with %v", config.repository, branchErr, headErr)
	}
	switch {
	case headErr != nil && branchErr == nil:
		return string(branchOut), "", "", false, nil
	case headErr != nil:
		return "", "", "", false, headErr
	default:
	}
	subjectOut, subjectErr := runGit(config, []string{"show", "-s", "--format=%s", "HEAD"})
	switch {
	case subjectErr != nil:
		return "", "", "", false, subjectErr
	default:
		return string(branchOut), strings.TrimSpace(string(headOut)), strings.TrimSuffix(string(subjectOut), "\n"), true, nil
	}
}

func parseStatusRecord(repository string, record string) (string, string, error) {
	switch {
	case len(record) < 4 || record[2] != ' ':
		return "", "", fmt.Errorf("cannot parse Git working-tree status for repository %s: expected an XY status, space and path, got %q", repository, record)
	default:
		return record[:2], record[3:], nil
	}
}

func statusMatchesView(status string, view string) bool {
	conflicted := isConflictStatus(status)
	switch view {
	case viewUncommitted:
		return status != "!!"
	case viewStaged:
		return !conflicted && status[0] != ' ' && status != "??"
	case viewUnstaged:
		return !conflicted && status[1] != ' ' && status != "??"
	case viewUntracked:
		return status == "??"
	case viewConflicted:
		return conflicted
	default:
		return false
	}
}

func isConflictStatus(status string) bool {
	switch status {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	default:
		return false
	}
}

func statusLabel(status string, view string) string {
	switch view {
	case viewUncommitted:
		switch {
		case isConflictStatus(status):
			return "conflicted, " + statusLabel(status, viewConflicted)
		case status == "??":
			return "untracked"
		default:
		}
		labels := []string{}
		switch {
		case status[0] != ' ':
			labels = append(labels, "staged "+singleStatusLabel(status[:1]))
		default:
		}
		switch {
		case status[1] != ' ':
			labels = append(labels, "unstaged "+singleStatusLabel(status[1:]))
		default:
		}
		return strings.Join(labels, ", ")
	case viewStaged:
		return singleStatusLabel(status[:1])
	case viewUnstaged:
		return singleStatusLabel(status[1:])
	case viewUntracked:
		return "untracked"
	case viewConflicted:
		switch status {
		case "DD":
			return "both deleted"
		case "AU":
			return "added by us"
		case "UD":
			return "deleted by them"
		case "UA":
			return "added by them"
		case "DU":
			return "deleted by us"
		case "AA":
			return "both added"
		case "UU":
			return "both modified"
		default:
			return "conflicted"
		}
	default:
		return "unknown"
	}
}

func singleStatusLabel(status string) string {
	switch status {
	case "A":
		return "added"
	case "C":
		return "copied"
	case "D":
		return "deleted"
	case "M":
		return "modified"
	case "R":
		return "renamed"
	case "T":
		return "type changed"
	case "U":
		return "unmerged"
	default:
		return "status " + status
	}
}

func sourceForView(repository string, view string, status string, path string) fileSource {
	switch view {
	case viewUncommitted:
		switch {
		case workingPathExists(repository, path):
			return fileSource{mode: sourceWorktree, path: path}
		case status[0] != 'D' && status[0] != ' ' && status != "??":
			return fileSource{mode: sourceIndex, path: path}
		default:
			return fileSource{}
		}
	case viewStaged:
		switch {
		case status[0] == 'D':
			return fileSource{}
		default:
			return fileSource{mode: sourceIndex, path: path}
		}
	case viewUnstaged, viewUntracked, viewConflicted:
		switch {
		case workingPathExists(repository, path):
			return fileSource{mode: sourceWorktree, path: path}
		default:
			return fileSource{}
		}
	default:
		return fileSource{}
	}
}

func workingPathExists(repository string, path string) bool {
	fullPath, pathErr := safeWorkingPath(repository, path)
	switch {
	case pathErr != nil:
		return false
	default:
	}
	_, err := os.Lstat(fullPath)
	return err == nil
}

func workingFileSize(repository string, source fileSource) int64 {
	switch source.mode {
	case sourceWorktree:
		fullPath, pathErr := safeWorkingPath(repository, source.path)
		switch {
		case pathErr != nil:
			return -1
		default:
		}
		info, err := os.Lstat(fullPath)
		switch {
		case err != nil:
			return -1
		case info.Mode()&os.ModeSymlink != 0:
			target, targetErr := os.Readlink(fullPath)
			switch {
			case targetErr != nil:
				return -1
			default:
				return int64(len(target))
			}
		default:
			return info.Size()
		}
	default:
		return -1
	}
}

func splitNUL(raw []byte) []string {
	parts := strings.Split(string(raw), "\x00")
	switch {
	case len(parts) > 0 && parts[len(parts)-1] == "":
		parts = parts[:len(parts)-1]
	default:
	}
	return parts
}

func validObjectID(value string) bool {
	switch len(value) {
	case 40, 64:
	default:
		return false
	}
	for _, char := range value {
		switch {
		case char >= '0' && char <= '9':
		case char >= 'a' && char <= 'f':
		case char >= 'A' && char <= 'F':
		default:
			return false
		}
	}
	return true
}

func isMissingHeadError(err error) bool {
	switch {
	case err == nil:
		return false
	default:
	}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		message := err.Error()
		return strings.Contains(message, "does not have any commits yet") || strings.Contains(message, "unknown revision") || strings.Contains(message, "bad revision 'HEAD'") || strings.Contains(message, "Not a valid object name HEAD") || strings.Contains(message, "ambiguous argument 'HEAD'")
	default:
		return false
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
