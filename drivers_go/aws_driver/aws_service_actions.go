package awsdriver

import "fmt"

func awsServiceActionArgs(kind string, target actionTarget, body string) ([]string, string, error) {
	switch {
	case !validRegion(target.Region) || !validDetailedOperation(target.Operation) || !validIdentifier(target.Identifier):
		return nil, "", fmt.Errorf("cannot execute AWS service action %q: invalid region %q, operation %q or identifier %q", kind, target.Region, target.Operation, target.Identifier)
	default:
	}
	var command []string
	var description string
	switch kind {
	case actionEC2Start, actionEC2Stop, actionEC2Reboot, actionEC2Terminate:
		operation := map[string]string{actionEC2Start: "start-instances", actionEC2Stop: "stop-instances", actionEC2Reboot: "reboot-instances", actionEC2Terminate: "terminate-instances"}[kind]
		switch {
		case target.Operation != "ec2-instances" || body != "":
			return nil, "", fmt.Errorf("cannot execute EC2 instance action %q: expected an EC2 instance target and an empty body", kind)
		default:
		}
		command = []string{"ec2", operation, "--instance-ids", target.Identifier}
		description = operation + " for EC2 instance " + target.Identifier
	case actionSGAuthorize, actionSGRevoke:
		operation := map[string]string{actionSGAuthorize: "authorize-security-group-ingress", actionSGRevoke: "revoke-security-group-ingress"}[kind]
		switch {
		case target.Operation != "ec2-security-groups" || !validJSONArray(body):
			return nil, "", fmt.Errorf("cannot change security group %q ingress: expected a non-empty JSON array of IP permissions", target.Identifier)
		default:
		}
		command = []string{"ec2", operation, "--group-id", target.Identifier, "--ip-permissions", body}
		description = operation + " for security group " + target.Identifier
	case actionS3PutPolicy:
		switch {
		case target.Operation != "s3-buckets" || !validJSONObject(body):
			return nil, "", fmt.Errorf("cannot set S3 bucket %q policy: expected a non-empty JSON policy object", target.Identifier)
		default:
		}
		command = []string{"s3api", "put-bucket-policy", "--bucket", target.Identifier, "--policy", body}
		description = "setting the policy for S3 bucket " + target.Identifier
	case actionS3DeletePolicy:
		switch {
		case target.Operation != "s3-buckets" || body != "":
			return nil, "", fmt.Errorf("cannot delete S3 bucket %q policy: expected an S3 bucket target and an empty body", target.Identifier)
		default:
		}
		command = []string{"s3api", "delete-bucket-policy", "--bucket", target.Identifier}
		description = "deleting the policy for S3 bucket " + target.Identifier
	case actionS3Versioning:
		switch body {
		case "Enabled", "Suspended":
			switch {
			case target.Operation != "s3-buckets":
				return nil, "", fmt.Errorf("cannot set S3 bucket versioning: expected an S3 bucket target, got %q", target.Operation)
			default:
			}
		default:
			return nil, "", fmt.Errorf("cannot set S3 bucket %q versioning: expected Enabled or Suspended, got %q", target.Identifier, body)
		}
		command = []string{"s3api", "put-bucket-versioning", "--bucket", target.Identifier, "--versioning-configuration", `{"Status":"` + body + `"}`}
		description = "setting versioning for S3 bucket " + target.Identifier + " to " + body
	case actionIAMTrust:
		switch {
		case target.Operation != "iam-roles" || !validJSONObject(body):
			return nil, "", fmt.Errorf("cannot update IAM role %q trust policy: expected a non-empty JSON policy object", target.Identifier)
		default:
		}
		command = []string{"iam", "update-assume-role-policy", "--role-name", target.Identifier, "--policy-document", body}
		description = "updating the trust policy for IAM role " + target.Identifier
	case actionIAMPolicyVersion:
		switch {
		case target.Operation != "iam-policies" || !validJSONObject(body):
			return nil, "", fmt.Errorf("cannot create IAM policy %q version: expected a non-empty JSON policy object", target.Identifier)
		default:
		}
		command = []string{"iam", "create-policy-version", "--policy-arn", target.Identifier, "--policy-document", body, "--set-as-default"}
		description = "creating and activating a version of IAM policy " + target.Identifier
	default:
		return nil, "", fmt.Errorf("cannot execute unsupported AWS service action %q", kind)
	}
	return awsRemoteArgs(command, target.Profile, target.Region), description, nil
}

func validAWSServiceActionInvocation(args []string) bool {
	profile, region, tailOK := validateAWSTail(args)
	switch {
	case !tailOK || !validProfile(profile) || !validRegion(region):
		return false
	default:
	}
	prefix := args[:len(args)-7]
	switch {
	case len(prefix) == 4 && prefix[0] == "ec2" && validEC2InstanceMutation(prefix[1]) && prefix[2] == "--instance-ids" && validIdentifier(prefix[3]):
		return true
	case len(prefix) == 6 && prefix[0] == "ec2" && validSecurityGroupMutation(prefix[1]) && prefix[2] == "--group-id" && validIdentifier(prefix[3]) && prefix[4] == "--ip-permissions" && validJSONArray(prefix[5]):
		return true
	case len(prefix) == 6 && prefix[0] == "s3api" && prefix[1] == "put-bucket-policy" && prefix[2] == "--bucket" && validIdentifier(prefix[3]) && prefix[4] == "--policy" && validJSONObject(prefix[5]):
		return true
	case len(prefix) == 4 && prefix[0] == "s3api" && prefix[1] == "delete-bucket-policy" && prefix[2] == "--bucket" && validIdentifier(prefix[3]):
		return true
	case len(prefix) == 6 && prefix[0] == "s3api" && prefix[1] == "put-bucket-versioning" && prefix[2] == "--bucket" && validIdentifier(prefix[3]) && prefix[4] == "--versioning-configuration" && validVersioningDocument(prefix[5]):
		return true
	case len(prefix) == 6 && prefix[0] == "iam" && prefix[1] == "update-assume-role-policy" && prefix[2] == "--role-name" && validIdentifier(prefix[3]) && prefix[4] == "--policy-document" && validJSONObject(prefix[5]):
		return true
	case len(prefix) == 7 && prefix[0] == "iam" && prefix[1] == "create-policy-version" && prefix[2] == "--policy-arn" && validIdentifier(prefix[3]) && prefix[4] == "--policy-document" && validJSONObject(prefix[5]) && prefix[6] == "--set-as-default":
		return true
	default:
		return false
	}
}

func validEC2InstanceMutation(operation string) bool {
	switch operation {
	case "start-instances", "stop-instances", "reboot-instances", "terminate-instances":
		return true
	default:
		return false
	}
}

func validSecurityGroupMutation(operation string) bool {
	switch operation {
	case "authorize-security-group-ingress", "revoke-security-group-ingress":
		return true
	default:
		return false
	}
}

func validVersioningDocument(document string) bool {
	return document == `{"Status":"Enabled"}` || document == `{"Status":"Suspended"}`
}
