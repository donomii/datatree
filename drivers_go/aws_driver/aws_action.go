package awsdriver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const maxAWSActionBodyBytes = 256 * 1024

func ExecuteAction(kind string, rawTarget []byte, body string) ([]byte, error) {
	var target actionTarget
	switch err := json.Unmarshal(rawTarget, &target); {
	case err != nil:
		return nil, fmt.Errorf("cannot execute AWS action %q: expected a JSON action target, got %d bytes with error: %v", kind, len(rawTarget), err)
	default:
	}
	args, description, err := awsActionArgs(kind, target, body)
	switch {
	case err != nil:
		return nil, err
	default:
	}
	config, configErr := resolveAWSConfig()
	switch {
	case configErr != nil:
		return nil, configErr
	default:
	}
	switch err := validateAWSActionInvocation(args); {
	case err != nil:
		return nil, err
	default:
	}
	out, runErr := runAWSCommand(config, args, description, false)
	switch {
	case runErr != nil:
		return nil, runErr
	case len(strings.TrimSpace(string(out))) == 0:
		return []byte("AWS action completed: " + description), nil
	default:
		return []byte(strings.TrimSpace(string(out))), nil
	}
}

func awsActionArgs(kind string, target actionTarget, body string) ([]string, string, error) {
	switch {
	case len(body) > maxAWSActionBodyBytes:
		return nil, "", fmt.Errorf("cannot execute AWS action %q: received a %d-byte body, limit is %d bytes", kind, len(body), maxAWSActionBodyBytes)
	case !validProfile(target.Profile):
		return nil, "", fmt.Errorf("cannot execute AWS action %q: invalid profile %q", kind, target.Profile)
	default:
	}
	trimmed := strings.TrimSpace(body)
	switch kind {
	case actionSetRegion:
		switch {
		case target.Setting != "region" || !validRegion(trimmed):
			return nil, "", fmt.Errorf("cannot set AWS profile %q region: expected a region such as us-east-1, got %q", target.Profile, body)
		default:
			return []string{"configure", "set", "region", trimmed, "--profile", target.Profile}, "setting profile " + target.Profile + " default region to " + trimmed, nil
		}
	case actionSetOutput:
		switch trimmed {
		case "json", "text", "table", "yaml", "yaml-stream", "off":
			switch {
			case target.Setting != "output":
				return nil, "", fmt.Errorf("cannot set AWS profile %q output: action target setting is %q", target.Profile, target.Setting)
			default:
				return []string{"configure", "set", "output", trimmed, "--profile", target.Profile}, "setting profile " + target.Profile + " output format to " + trimmed, nil
			}
		default:
			return nil, "", fmt.Errorf("cannot set AWS profile %q output: expected json, text, table, yaml, yaml-stream or off, got %q", target.Profile, body)
		}
	case actionCreate, actionUpdate, actionDelete:
		switch {
		case !validRegion(target.Region) || !validTypeName(target.TypeName):
			return nil, "", fmt.Errorf("cannot execute AWS resource action %q: invalid region %q or type %q", kind, target.Region, target.TypeName)
		case kind != actionCreate && !validIdentifier(target.Identifier):
			return nil, "", fmt.Errorf("cannot execute AWS resource action %q: invalid identifier %q", kind, target.Identifier)
		default:
		}
		token, tokenErr := newClientToken()
		switch {
		case tokenErr != nil:
			return nil, "", tokenErr
		default:
		}
		switch kind {
		case actionCreate:
			var desired map[string]json.RawMessage
			switch err := json.Unmarshal([]byte(trimmed), &desired); {
			case err != nil || len(desired) == 0:
				return nil, "", fmt.Errorf("cannot create AWS %s resource: expected a non-empty desired-state JSON object, got error: %v", target.TypeName, err)
			default:
			}
			return awsRemoteArgs([]string{"cloudcontrol", "create-resource", "--type-name", target.TypeName, "--desired-state", trimmed, "--client-token", token}, target.Profile, target.Region), "creating " + target.TypeName + " in " + target.Region + " using profile " + target.Profile, nil
		case actionUpdate:
			var patch []json.RawMessage
			switch err := json.Unmarshal([]byte(trimmed), &patch); {
			case err != nil || len(patch) == 0:
				return nil, "", fmt.Errorf("cannot update AWS %s resource %q: expected a non-empty RFC 6902 JSON patch array, got error: %v", target.TypeName, target.Identifier, err)
			default:
			}
			return awsRemoteArgs([]string{"cloudcontrol", "update-resource", "--type-name", target.TypeName, "--identifier", target.Identifier, "--patch-document", trimmed, "--client-token", token}, target.Profile, target.Region), "updating " + target.TypeName + " resource " + target.Identifier + " in " + target.Region + " using profile " + target.Profile, nil
		case actionDelete:
			switch {
			case trimmed != "":
				return nil, "", fmt.Errorf("cannot delete AWS %s resource %q: expected no body, got %d bytes", target.TypeName, target.Identifier, len(body))
			default:
			}
			return awsRemoteArgs([]string{"cloudcontrol", "delete-resource", "--type-name", target.TypeName, "--identifier", target.Identifier, "--client-token", token}, target.Profile, target.Region), "deleting " + target.TypeName + " resource " + target.Identifier + " in " + target.Region + " using profile " + target.Profile, nil
		default:
			return nil, "", fmt.Errorf("cannot execute AWS action kind %q", kind)
		}
	case actionEC2Start, actionEC2Stop, actionEC2Reboot, actionEC2Terminate, actionSGAuthorize, actionSGRevoke, actionS3PutPolicy, actionS3DeletePolicy, actionS3Versioning, actionIAMTrust, actionIAMPolicyVersion:
		return awsServiceActionArgs(kind, target, trimmed)
	default:
		return nil, "", fmt.Errorf("cannot execute unsupported AWS action kind %q", kind)
	}
}

func validateAWSActionInvocation(args []string) error {
	switch {
	case validConfigureSetInvocation(args):
	case validCloudControlActionInvocation(args, "create-resource"):
	case validCloudControlActionInvocation(args, "update-resource"):
	case validCloudControlActionInvocation(args, "delete-resource"):
	case validAWSServiceActionInvocation(args):
	default:
		return fmt.Errorf("AWS action invocation blocked: expected an exact allowlisted mutation, refusing argument shape with %d elements", len(args))
	}
	return nil
}

func validConfigureSetInvocation(args []string) bool {
	switch {
	case len(args) != 6 || args[0] != "configure" || args[1] != "set" || args[4] != "--profile" || !validProfile(args[5]):
		return false
	case args[2] == "region":
		return validRegion(args[3])
	case args[2] == "output":
		switch args[3] {
		case "json", "text", "table", "yaml", "yaml-stream", "off":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func validCloudControlActionInvocation(args []string, operation string) bool {
	profile, region, tailOK := validateAWSTail(args)
	switch {
	case !tailOK || !validProfile(profile) || !validRegion(region):
		return false
	default:
	}
	prefix := args[:len(args)-7]
	switch operation {
	case "create-resource":
		return len(prefix) == 8 && prefix[0] == "cloudcontrol" && prefix[1] == operation && prefix[2] == "--type-name" && validTypeName(prefix[3]) && prefix[4] == "--desired-state" && validJSONObject(prefix[5]) && prefix[6] == "--client-token" && validClientToken(prefix[7])
	case "update-resource":
		return len(prefix) == 10 && prefix[0] == "cloudcontrol" && prefix[1] == operation && prefix[2] == "--type-name" && validTypeName(prefix[3]) && prefix[4] == "--identifier" && validIdentifier(prefix[5]) && prefix[6] == "--patch-document" && validJSONArray(prefix[7]) && prefix[8] == "--client-token" && validClientToken(prefix[9])
	case "delete-resource":
		return len(prefix) == 8 && prefix[0] == "cloudcontrol" && prefix[1] == operation && prefix[2] == "--type-name" && validTypeName(prefix[3]) && prefix[4] == "--identifier" && validIdentifier(prefix[5]) && prefix[6] == "--client-token" && validClientToken(prefix[7])
	default:
		return false
	}
}

func validJSONObject(raw string) bool {
	var value map[string]json.RawMessage
	err := json.Unmarshal([]byte(raw), &value)
	return err == nil && len(value) > 0
}

func validJSONArray(raw string) bool {
	var value []json.RawMessage
	err := json.Unmarshal([]byte(raw), &value)
	return err == nil && len(value) > 0
}

func newClientToken() (string, error) {
	raw := make([]byte, 16)
	_, err := rand.Read(raw)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot create AWS action idempotency token: expected 16 random bytes, got error: %v", err)
	default:
		return hex.EncodeToString(raw), nil
	}
}

func validClientToken(value string) bool {
	switch decoded, err := hex.DecodeString(value); {
	case err != nil:
		return false
	default:
		return len(decoded) == 16
	}
}
