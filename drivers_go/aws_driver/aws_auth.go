package awsdriver

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Reopen only an already configured SSO profile, once per failed read. Normal
// role, credential-process and session refresh remains the AWS CLI's job.
func withAWSLogin(config awsConfig, args []string, read func() ([]byte, error)) ([]byte, error) {
	out, err := read()
	switch {
	case err == nil:
		return out, nil
	default:
	}
	message := strings.ToLower(err.Error())
	profile, _, valid := validateAWSTail(args)
	expired := strings.Contains(message, "expired") || strings.Contains(message, "unauthorizedssotoken") || strings.Contains(message, "error loading sso token")
	switch {
	case !valid || !validProfile(profile) || !expired:
		return nil, err
	default:
	}
	configured, configErr := runAWSCommand(config, profileSettingArgs(profile, "sso_session"), "checking configured SSO session", true)
	switch {
	case !strings.Contains(message, "sso") && (configErr != nil || strings.TrimSpace(string(configured)) == ""):
		return nil, err
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, config.executable, "sso", "login", "--profile", profile, "--no-cli-pager")
	stdout, stderr := awsBoundedBuffer{limit: config.maxOutputBytes}, awsBoundedBuffer{limit: config.maxOutputBytes}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	switch loginErr := cmd.Run(); {
	case loginErr != nil || stdout.exceeded() || stderr.exceeded():
		return nil, fmt.Errorf("cannot renew AWS SSO profile %q after %v: login=%v; complete any provider sign-in prompt and retry; output: %s %s", profile, err, loginErr, stdout.data.String(), stderr.data.String())
	default:
		return read()
	}
}
