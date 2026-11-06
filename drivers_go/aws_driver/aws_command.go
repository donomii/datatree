package awsdriver

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type awsConfig struct {
	executable     string
	maxOutputBytes int64
}

type awsBoundedBuffer struct {
	data  bytes.Buffer
	limit int64
	total int64
}

const defaultAWSMaxOutputBytes int64 = 8 * 1024 * 1024
const awsListLimit = 1000

var profilePattern = regexp.MustCompile(`^[A-Za-z0-9_+=,.@ -]{1,128}$`)
var regionPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
var typeNamePattern = regexp.MustCompile(`^[A-Za-z0-9]{2,64}::[A-Za-z0-9]{2,64}::[A-Za-z0-9]{2,64}$`)

func resolveAWSConfig() (awsConfig, error) {
	candidate := os.Getenv("DATATREE_AWS")
	switch {
	case candidate == "":
		candidate = "aws"
	default:
	}
	executable, err := exec.LookPath(candidate)
	switch {
	case err != nil:
		return awsConfig{}, fmt.Errorf("cannot find a usable AWS CLI executable: looked for %q (set DATATREE_AWS to override), got: %v", candidate, err)
	default:
	}
	maxOutputBytes, settingErr := awsPositiveInt64Setting("DATATREE_AWS_MAX_OUTPUT_BYTES", defaultAWSMaxOutputBytes)
	switch {
	case settingErr != nil:
		return awsConfig{}, settingErr
	default:
		return awsConfig{executable: executable, maxOutputBytes: maxOutputBytes}, nil
	}
}

func profileListArgs() []string {
	return []string{"configure", "list-profiles"}
}

func profileSettingArgs(profile string, setting string) []string {
	return []string{"configure", "get", setting, "--profile", profile}
}

func regionListArgs(profile string, bootstrapRegion string) []string {
	return awsRemoteArgs([]string{"ec2", "describe-regions", "--all-regions"}, profile, bootstrapRegion)
}

func resourceSearchArgs(profile string, region string) []string {
	return awsRemoteArgs([]string{"resource-explorer-2", "search", "--query-string", "*", "--max-items", strconv.Itoa(awsListLimit)}, profile, region)
}

func resourceTypeListArgs(profile string, region string) []string {
	return awsRemoteArgs([]string{"cloudformation", "list-types", "--visibility", "PUBLIC", "--type", "RESOURCE", "--filters", "Category=AWS_TYPES", "--max-items", strconv.Itoa(awsListLimit)}, profile, region)
}

func resourceListArgs(profile string, region string, typeName string) []string {
	return awsRemoteArgs([]string{"cloudcontrol", "list-resources", "--type-name", typeName, "--max-items", strconv.Itoa(awsListLimit)}, profile, region)
}

func resourceGetArgs(profile string, region string, typeName string, identifier string) []string {
	return awsRemoteArgs([]string{"cloudcontrol", "get-resource", "--type-name", typeName, "--identifier", identifier}, profile, region)
}

func awsRemoteArgs(operation []string, profile string, region string) []string {
	args := append([]string{}, operation...)
	return append(args, "--profile", profile, "--region", region, "--output", "json", "--no-cli-pager")
}

func runReadOnlyAWS(config awsConfig, args []string) ([]byte, error) {
	assertReadOnlyAWSInvocation(args)
	return runAWSCommand(config, args, "reading AWS data", true)
}

func runAWSCommand(config awsConfig, args []string, task string, tolerateMissingSetting bool) ([]byte, error) {
	cmd := exec.Command(config.executable, args...)
	stdout := awsBoundedBuffer{limit: config.maxOutputBytes}
	stderr := awsBoundedBuffer{limit: config.maxOutputBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	switch {
	case stdout.exceeded():
		return nil, fmt.Errorf("aws %s produced %d stdout bytes while %s, exceeding DATATREE_AWS_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stdout.total, task, config.maxOutputBytes)
	case stderr.exceeded():
		return nil, fmt.Errorf("aws %s produced %d stderr bytes while %s, exceeding DATATREE_AWS_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stderr.total, task, config.maxOutputBytes)
	case runErr != nil && tolerateMissingSetting && validProfileSettingInvocation(args) && stdout.total == 0 && strings.TrimSpace(stderr.data.String()) == "":
		return []byte{}, nil
	case runErr != nil:
		return nil, fmt.Errorf("aws %s failed while %s: expected a zero exit, got %v with %d stdout bytes and stderr: %s", strings.Join(args, " "), task, runErr, stdout.total, strings.TrimSpace(stderr.data.String()))
	default:
		return stdout.data.Bytes(), nil
	}
}

func assertReadOnlyAWSInvocation(args []string) {
	switch {
	case stringSlicesEqual(args, profileListArgs()):
	case validProfileSettingInvocation(args):
	case validRemoteInvocation(args, "ec2", "describe-regions"):
	case validRemoteInvocation(args, "resource-explorer-2", "search"):
	case validRemoteInvocation(args, "cloudformation", "list-types"):
	case validRemoteInvocation(args, "cloudcontrol", "list-resources"):
	case validRemoteInvocation(args, "cloudcontrol", "get-resource"):
	case validServiceReadInvocation(args):
	default:
		panic(fmt.Sprintf("AWS CLI invocation blocked: expected an exact allowlisted read-only command, refusing to run %q", strings.Join(args, " ")))
	}
}

func validProfileSettingInvocation(args []string) bool {
	switch {
	case len(args) != 5 || args[0] != "configure" || args[1] != "get" || args[3] != "--profile":
		return false
	case args[2] != "region" && args[2] != "output" && args[2] != "role_arn" && args[2] != "source_profile" && args[2] != "credential_source" && args[2] != "sso_session" && args[2] != "sso_account_id" && args[2] != "sso_role_name":
		return false
	default:
		return validProfile(args[4])
	}
}

func validRemoteInvocation(args []string, service string, operation string) bool {
	switch {
	case len(args) < 10 || args[0] != service || args[1] != operation:
		return false
	default:
	}
	profile, region, tailOK := validateAWSTail(args)
	switch {
	case !tailOK || !validProfile(profile) || !validRegion(region):
		return false
	default:
	}
	prefix := args[:len(args)-7]
	switch service + " " + operation {
	case "ec2 describe-regions":
		return stringSlicesEqual(prefix, []string{"ec2", "describe-regions", "--all-regions"})
	case "resource-explorer-2 search":
		return stringSlicesEqual(prefix, []string{"resource-explorer-2", "search", "--query-string", "*", "--max-items", strconv.Itoa(awsListLimit)})
	case "cloudformation list-types":
		return stringSlicesEqual(prefix, []string{"cloudformation", "list-types", "--visibility", "PUBLIC", "--type", "RESOURCE", "--filters", "Category=AWS_TYPES", "--max-items", strconv.Itoa(awsListLimit)})
	case "cloudcontrol list-resources":
		return len(prefix) == 6 && prefix[2] == "--type-name" && validTypeName(prefix[3]) && prefix[4] == "--max-items" && prefix[5] == strconv.Itoa(awsListLimit)
	case "cloudcontrol get-resource":
		return len(prefix) == 6 && prefix[2] == "--type-name" && validTypeName(prefix[3]) && prefix[4] == "--identifier" && validIdentifier(prefix[5])
	default:
		return false
	}
}

func validateAWSTail(args []string) (string, string, bool) {
	switch {
	case len(args) < 7:
		return "", "", false
	default:
	}
	tail := args[len(args)-7:]
	switch {
	case tail[0] != "--profile" || tail[2] != "--region" || tail[4] != "--output" || tail[5] != "json" || tail[6] != "--no-cli-pager":
		return "", "", false
	default:
		return tail[1], tail[3], true
	}
}

func validProfile(value string) bool {
	return profilePattern.MatchString(value) && strings.TrimSpace(value) == value
}

func validRegion(value string) bool {
	return regionPattern.MatchString(value)
}

func validTypeName(value string) bool {
	return typeNamePattern.MatchString(value)
}

func validIdentifier(value string) bool {
	return value != "" && len(value) <= 1024 && !strings.ContainsAny(value, "\x00\r\n")
}

func awsPositiveInt64Setting(name string, defaultValue int64) (int64, error) {
	raw := os.Getenv(name)
	switch {
	case raw == "":
		return defaultValue, nil
	default:
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	switch {
	case err != nil || value <= 0:
		return 0, fmt.Errorf("cannot configure AWS browsing: expected %s to be a positive byte count, got %q", name, raw)
	default:
		return value, nil
	}
}

func (buffer *awsBoundedBuffer) Write(data []byte) (int, error) {
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

func (buffer *awsBoundedBuffer) exceeded() bool {
	return buffer.total > buffer.limit
}

func stringSlicesEqual(left []string, right []string) bool {
	switch {
	case len(left) != len(right):
		return false
	default:
	}
	for index := range left {
		switch {
		case left[index] != right[index]:
			return false
		default:
		}
	}
	return true
}
