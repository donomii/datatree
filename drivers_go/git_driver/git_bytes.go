package gitdriver

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// File payloads are not command metadata and must not inherit its byte cap.
func runGitFile(config gitConfig, args []string) ([]byte, error) {
	assertReadOnlyGitInvocation(args)
	command := []string{"--no-pager", "--no-optional-locks", "-c", "core.fsmonitor=false", "-C", config.repository}
	cmd := exec.Command(config.executable, append(command, args...)...)
	cmd.Env = gitEnvironment()
	var data bytes.Buffer
	stderr := boundedBuffer{limit: config.maxOutputBytes}
	cmd.Stdout, cmd.Stderr = &data, &stderr
	err := cmd.Run()
	switch {
	case err != nil || stderr.exceeded():
		return nil, fmt.Errorf("cannot read Git file using %s in %q: expected complete bytes, got %v with %d stderr bytes (limit %d): %s", strings.Join(args, " "), config.repository, err, stderr.total, config.maxOutputBytes, stderr.data.String())
	default:
		return data.Bytes(), nil
	}
}
