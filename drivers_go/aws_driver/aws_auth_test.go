package awsdriver

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestExpiredSSOReadRenewsConfiguredProfileAndRetriesOnce(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "aws")
	script := `#!/bin/sh
case "$*" in
 "configure get sso_session --profile default") printf 'existing-session' ;;
 "sso login --profile default --no-cli-pager") exit 0 ;;
 *) exit 97 ;;
esac
`
	switch err := os.WriteFile(executable, []byte(script), 0700); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	calls := 0
	raw, err := withAWSLogin(awsConfig{executable: executable, maxOutputBytes: 4096}, awsRemoteArgs([]string{"s3api", "list-buckets"}, "default", "us-east-1"), func() ([]byte, error) {
		calls++
		switch calls {
		case 1:
			return nil, fmt.Errorf("The SSO session associated with this profile has expired")
		default:
			return []byte("actual bytes"), nil
		}
	})
	switch {
	case err != nil || string(raw) != "actual bytes" || calls != 2:
		t.Fatalf("renewal did not resume read: %q calls=%d err=%v", raw, calls, err)
	default:
	}
}

func TestPermissionErrorsDoNotTriggerLoginOrRetry(t *testing.T) {
	calls := 0
	_, err := withAWSLogin(awsConfig{}, awsRemoteArgs([]string{"s3api", "list-buckets"}, "default", "us-east-1"), func() ([]byte, error) {
		calls++
		return nil, fmt.Errorf("AccessDenied")
	})
	switch {
	case err == nil || calls != 1:
		t.Fatalf("unexpected permission retry: calls=%d err=%v", calls, err)
	default:
	}
}
