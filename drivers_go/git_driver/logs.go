package gitdriver

import (
	"fmt"
	"os"
)

func logGitStartFailure(err error) {
	fmt.Fprintf(os.Stderr, "Git driver could not start: expected a repository path or mount state from the parent driver, got: %v\n", err)
}

func logGitReadFailure(repository string, view string, err error) {
	fmt.Fprintf(os.Stderr, "Git driver could not read repository %q view %q: %v\n", repository, view, err)
}

func logGitResourceEncodeFailure(repository string, view string, resourceCount int, err error) {
	fmt.Fprintf(os.Stderr, "Git driver could not encode %d resources for repository %q view %q: %v\n", resourceCount, repository, view, err)
}

func logGitFileReadFailure(repository string, source string, path string, err error) {
	fmt.Fprintf(os.Stderr, "Git driver could not serve repository %q file %q from %s: %v\n", repository, path, source, err)
}
