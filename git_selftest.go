package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func findResourceContaining(resources []Resource, text string, kind string) (Resource, error) {
	for _, resource := range resources {
		if strings.Contains(resource.Name, text) && (kind == "" || resource.Kind == kind) {
			return resource, nil
		}
	}
	return Resource{}, fmt.Errorf("resource not found: expected name containing %q kind %s in %d resources", text, kind, len(resources))
}

func findResourceHandleContaining(store *Store, resources []Resource, text string, kind string) (ChildHandle, error) {
	resource, err := findResourceContaining(resources, text, kind)
	if err != nil {
		return ChildHandle{}, err
	}
	if resource.Handle == "" {
		return ChildHandle{}, fmt.Errorf("resource containing %q kind %s did not include a handle", text, kind)
	}
	handle, ok := store.getHandle(resource.Handle)
	if !ok {
		return ChildHandle{}, fmt.Errorf("resource containing %q kind %s referenced unknown handle %s", text, kind, resource.Handle)
	}
	return handle, nil
}

func runFixtureGit(repository string, args ...string) error {
	commandArgs := append([]string{"-C", repository}, args...)
	cmd := exec.Command("git", commandArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot prepare Git self-test repository %s: git %s failed with %v and output %q", repository, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func prepareGitFixture(fixtureDir string) (string, error) {
	repository := filepath.Join(fixtureDir, "repository")
	if err := os.Mkdir(repository, 0700); err != nil {
		return "", fmt.Errorf("cannot create Git self-test repository %s: %v", repository, err)
	}
	if err := runFixtureGit(repository, "-c", "init.defaultBranch=main", "init", "--quiet"); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(repository, "committed.txt"), []byte("committed version\n"), 0600); err != nil {
		return "", fmt.Errorf("cannot write committed Git fixture file: %v", err)
	}
	if err := runFixtureGit(repository, "add", "committed.txt"); err != nil {
		return "", err
	}
	if err := runFixtureGit(repository, "-c", "user.name=Datatree Self Test", "-c", "user.email=datatree@example.invalid", "commit", "--quiet", "-m", "Initial Git fixture"); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(repository, "history.txt"), []byte("second commit\n"), 0600); err != nil {
		return "", fmt.Errorf("cannot write second committed Git fixture file: %v", err)
	}
	if err := runFixtureGit(repository, "add", "history.txt"); err != nil {
		return "", err
	}
	if err := runFixtureGit(repository, "-c", "user.name=Datatree Self Test", "-c", "user.email=datatree@example.invalid", "commit", "--quiet", "-m", "Second Git fixture"); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(repository, "staged.txt"), []byte("staged version\n"), 0600); err != nil {
		return "", fmt.Errorf("cannot write staged Git fixture file: %v", err)
	}
	if err := runFixtureGit(repository, "add", "staged.txt"); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(repository, "committed.txt"), []byte("unstaged version\n"), 0600); err != nil {
		return "", fmt.Errorf("cannot write unstaged Git fixture file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repository, "untracked.txt"), []byte("untracked version\n"), 0600); err != nil {
		return "", fmt.Errorf("cannot write untracked Git fixture file: %v", err)
	}
	unbornRepository := filepath.Join(fixtureDir, "unborn")
	if err := os.Mkdir(unbornRepository, 0700); err != nil {
		return "", fmt.Errorf("cannot create unborn Git self-test repository %s: %v", unbornRepository, err)
	}
	if err := runFixtureGit(unbornRepository, "-c", "init.defaultBranch=main", "init", "--quiet"); err != nil {
		return "", err
	}
	return repository, nil
}

func checkGitBrowse(store *Store, directoryResources []Resource) error {
	repositoryHandle, err := findResourceHandle(store, directoryResources, "repository", "git")
	if err != nil {
		return err
	}
	repositoryResources, err := store.loadDriverForHandle("git_driver", repositoryHandle)
	if err != nil {
		return fmt.Errorf("Git driver load failed for the self-test repository: %v", err)
	}
	if _, err := findResource(repositoryResources, "branch: main", "kv"); err != nil {
		return err
	}
	uncommittedHandle, err := findResourceHandle(store, repositoryResources, "[all uncommitted changes]", "git")
	if err != nil {
		return err
	}
	uncommittedResources, err := store.loadDriverForHandle("git_driver", uncommittedHandle)
	if err != nil {
		return fmt.Errorf("Git driver could not list all uncommitted changes: %v", err)
	}
	if _, err := findResource(uncommittedResources, "staged added: staged.txt", "text"); err != nil {
		return err
	}
	if _, err := findResource(uncommittedResources, "unstaged modified: committed.txt", "text"); err != nil {
		return err
	}
	if _, err := findResource(uncommittedResources, "untracked: untracked.txt", "text"); err != nil {
		return err
	}
	commitsHandle, err := findResourceHandle(store, repositoryResources, "[commits]", "git")
	if err != nil {
		return err
	}
	commitResources, err := store.loadDriverForHandle("git_driver", commitsHandle)
	if err != nil {
		return fmt.Errorf("Git driver could not list commits: %v", err)
	}
	commitHandle, err := findResourceHandleContaining(store, commitResources, "Second Git fixture", "git")
	if err != nil {
		return err
	}
	commitView, err := store.loadDriverForHandle("git_driver", commitHandle)
	if err != nil {
		return fmt.Errorf("Git driver could not expand the second commit: %v", err)
	}
	if _, err := findResource(commitView, "subject: Second Git fixture", "kv"); err != nil {
		return err
	}
	commitFileHandle, err := findResourceHandle(store, commitView, "added: history.txt", "text")
	if err != nil {
		return err
	}
	commitFile, err := store.serveChildRange(commitFileHandle, 0, -1)
	if err != nil || string(commitFile) != "second commit\n" {
		return fmt.Errorf("Git commit file mismatch: got %q with error %v, expected %q", string(commitFile), err, "second commit\n")
	}
	committedHandle, err := findResourceHandle(store, repositoryResources, "[committed files at HEAD]", "git")
	if err != nil {
		return err
	}
	committedResources, err := store.loadDriverForHandle("git_driver", committedHandle)
	if err != nil {
		return fmt.Errorf("Git driver could not list committed files: %v", err)
	}
	committedFileHandle, err := findResourceHandle(store, committedResources, "committed.txt", "text")
	if err != nil {
		return err
	}
	committedFile, err := store.serveChildRange(committedFileHandle, 0, -1)
	if err != nil || string(committedFile) != "committed version\n" {
		return fmt.Errorf("Git committed file mismatch: got %q with error %v, expected %q", string(committedFile), err, "committed version\n")
	}
	stateCases := []struct {
		section string
		name    string
		content string
	}{
		{section: "[staged changes]", name: "added: staged.txt", content: "staged version\n"},
		{section: "[unstaged changes]", name: "modified: committed.txt", content: "unstaged version\n"},
		{section: "[untracked files]", name: "untracked: untracked.txt", content: "untracked version\n"},
	}
	for _, stateCase := range stateCases {
		stateHandle, stateErr := findResourceHandle(store, repositoryResources, stateCase.section, "git")
		if stateErr != nil {
			return stateErr
		}
		stateResources, stateErr := store.loadDriverForHandle("git_driver", stateHandle)
		if stateErr != nil {
			return fmt.Errorf("Git driver could not list %s: %v", stateCase.section, stateErr)
		}
		fileHandle, stateErr := findResourceHandle(store, stateResources, stateCase.name, "text")
		if stateErr != nil {
			return stateErr
		}
		content, stateErr := store.serveChildRange(fileHandle, 0, -1)
		if stateErr != nil || string(content) != stateCase.content {
			return fmt.Errorf("Git state file %s mismatch: got %q with error %v, expected %q", stateCase.name, string(content), stateErr, stateCase.content)
		}
	}
	conflictedHandle, err := findResourceHandle(store, repositoryResources, "[conflicted files]", "git")
	if err != nil {
		return err
	}
	conflictedResources, err := store.loadDriverForHandle("git_driver", conflictedHandle)
	if err != nil {
		return fmt.Errorf("Git driver could not list conflicted files: %v", err)
	}
	if len(conflictedResources) != 0 {
		return fmt.Errorf("clean Git self-test repository reported conflicted files: %+v", conflictedResources)
	}
	if err := os.Setenv("DATATREE_GIT_MAX_RESULTS", "1"); err != nil {
		return fmt.Errorf("cannot set bounded Git result count: %v", err)
	}
	limitedCommits, err := store.loadDriverForHandle("git_driver", commitsHandle)
	os.Unsetenv("DATATREE_GIT_MAX_RESULTS")
	if err != nil {
		return fmt.Errorf("Git driver failed with a one-result limit: %v", err)
	}
	if len(limitedCommits) != 2 || limitedCommits[1].Kind != "error" || !strings.Contains(limitedCommits[1].Name, "first 1 commits") {
		return fmt.Errorf("Git result limit did not preserve one commit plus a truncation error: got %+v", limitedCommits)
	}
	unbornHandle, err := findResourceHandle(store, directoryResources, "unborn", "git")
	if err != nil {
		return err
	}
	unbornResources, err := store.loadDriverForHandle("git_driver", unbornHandle)
	if err != nil {
		return fmt.Errorf("Git driver could not open the unborn repository: %v", err)
	}
	if _, err := findResource(unbornResources, "HEAD: unborn; this repository has no commits", "kv"); err != nil {
		return err
	}
	unbornCommitsHandle, err := findResourceHandle(store, unbornResources, "[commits]", "git")
	if err != nil {
		return err
	}
	unbornCommits, err := store.loadDriverForHandle("git_driver", unbornCommitsHandle)
	if err != nil || len(unbornCommits) != 0 {
		return fmt.Errorf("unborn Git repository commit view mismatch: got %+v with error %v, expected no commits", unbornCommits, err)
	}
	unbornFilesHandle, err := findResourceHandle(store, unbornResources, "[committed files at HEAD]", "git")
	if err != nil {
		return err
	}
	unbornFiles, err := store.loadDriverForHandle("git_driver", unbornFilesHandle)
	if err != nil || len(unbornFiles) != 0 {
		return fmt.Errorf("unborn Git repository committed-file view mismatch: got %+v with error %v, expected no files", unbornFiles, err)
	}
	return nil
}
