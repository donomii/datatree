package githubdriver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func withGitHubLogin(config githubConfig, read func() ([]byte, error)) ([]byte, error) {
	out, err := read()
	switch {
	case err == nil:
		return out, nil
	default:
	}
	message := strings.ToLower(err.Error())
	expired := strings.Contains(message, "http 401") || strings.Contains(message, "bad credentials") || strings.Contains(message, "token has expired")
	switch {
	case !expired || os.Getenv("GH_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "":
		return nil, err
	default:
	}
	host := os.Getenv("GH_HOST")
	switch {
	case host == "":
		host = "github.com"
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// No scopes are added: refresh preserves the active account's scopes.
	cmd := exec.CommandContext(ctx, config.executable, "auth", "refresh", "--hostname", host)
	stdout, stderr := boundedBuffer{limit: config.maxOutputBytes}, boundedBuffer{limit: config.maxOutputBytes}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	switch loginErr := cmd.Run(); {
	case loginErr != nil || stdout.exceeded() || stderr.exceeded():
		return nil, fmt.Errorf("cannot refresh GitHub session for %q after %v: login=%v; complete any provider sign-in prompt and retry; output: %s %s", host, err, loginErr, stdout.data.String(), stderr.data.String())
	default:
		return read()
	}
}
