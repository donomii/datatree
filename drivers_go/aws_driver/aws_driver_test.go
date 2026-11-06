package awsdriver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"datahumper/drivers_go/driverlib"
)

func TestAWSHierarchyAndActionsUseExactCommands(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "fake-aws")
	script := `#!/bin/sh
case "$*" in
  "configure list-profiles") printf 'default\n' ;;
  "configure get region --profile default") printf 'us-east-1\n' ;;
  "configure get output --profile default") printf 'json\n' ;;
  "configure get "*" --profile default") exit 1 ;;
  "ec2 describe-regions --all-regions --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Regions":[{"RegionName":"us-east-1","OptInStatus":"opt-in-not-required"}]}' ;;
  "resource-explorer-2 search --query-string * --max-items 1000 --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Resources":[{"Arn":"arn:aws:logs:us-east-1:123456789012:log-group:example","Region":"us-east-1","ResourceType":"logs:log-group","Service":"logs"}]}' ;;
  "cloudformation list-types --visibility PUBLIC --type RESOURCE --filters Category=AWS_TYPES --max-items 1000 --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"TypeSummaries":[{"TypeName":"AWS::Logs::LogGroup","ProvisioningType":"FULLY_MUTABLE"}]}' ;;
  "cloudcontrol list-resources --type-name AWS::Logs::LogGroup --max-items 1000 --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"ResourceDescriptions":[{"Identifier":"example","Properties":"{\"LogGroupName\":\"example\",\"RetentionInDays\":30}"}]}' ;;
  "cloudcontrol get-resource --type-name AWS::Logs::LogGroup --identifier example --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"TypeName":"AWS::Logs::LogGroup","ResourceDescription":{"Identifier":"example","Properties":"{\"LogGroupName\":\"example\",\"RetentionInDays\":30}"}}' ;;
  "configure set output yaml --profile default") printf 'configured' ;;
  "cloudcontrol update-resource --type-name AWS::Logs::LogGroup --identifier example --patch-document "*" --client-token "*" --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"ProgressEvent":{"OperationStatus":"IN_PROGRESS","Operation":"UPDATE","Identifier":"example","RequestToken":"request-1"}}' ;;
  *) printf 'unexpected fake AWS command: %s\n' "$*" >&2; exit 97 ;;
esac
`
	switch err := os.WriteFile(fakePath, []byte(script), 0700); {
	case err != nil:
		t.Fatalf("cannot write fake AWS CLI %s: %v", fakePath, err)
	default:
	}
	t.Setenv("DATATREE_AWS", fakePath)

	rootResources, rootChildren := mustBuildAWSView(t, []byte("{}"))
	profileState := mustFindAWSChild(t, rootResources, rootChildren, "default", "aws")
	profileResources, profileChildren := mustBuildAWSView(t, profileState)
	regionsState := mustFindAWSChild(t, profileResources, profileChildren, regionsEntryName, "aws")
	regions, regionChildren := mustBuildAWSView(t, regionsState)
	regionState := mustFindAWSChild(t, regions, regionChildren, "us-east-1 (opt-in-not-required)", "aws")
	regionResources, regionChildren := mustBuildAWSView(t, regionState)
	searchState := mustFindAWSChild(t, regionResources, regionChildren, searchEntryName, "aws")
	searchResources, _ := mustBuildAWSView(t, searchState)
	mustFindAWSResource(t, searchResources, "logs:log-group arn:aws:logs:us-east-1:123456789012:log-group:example", "json")
	typesState := mustFindAWSChild(t, regionResources, regionChildren, typesEntryName, "aws")
	types, typeChildren := mustBuildAWSView(t, typesState)
	typeState := mustFindAWSChild(t, types, typeChildren, "AWS::Logs::LogGroup", "aws")
	resources, resourceChildren := mustBuildAWSView(t, typeState)
	mustFindAWSResource(t, resources, "Create resource", actionCreate)
	resourceState := mustFindAWSChild(t, resources, resourceChildren, "example", "aws")
	detail, detailChildren := mustBuildAWSView(t, resourceState)
	mustFindAWSResource(t, detail, propertiesEntryName, "json")
	actionsState := mustFindAWSChild(t, detail, detailChildren, resourceActionsEntryName, "aws")
	actions, actionChildren := mustBuildAWSView(t, actionsState)
	updateTarget := mustFindAWSChild(t, actions, actionChildren, "Update resource", actionUpdate)

	result, err := ExecuteAction(actionUpdate, updateTarget, `[{"op":"replace","path":"/RetentionInDays","value":90}]`)
	switch {
	case err != nil:
		t.Fatalf("fake AWS update failed: %v", err)
	case !strings.Contains(string(result), "IN_PROGRESS"):
		t.Fatalf("fake AWS update result mismatch: got %q", string(result))
	default:
	}
	configState := mustFindAWSChild(t, profileResources, profileChildren, configActionsEntryName, "aws")
	configActions, configChildren := mustBuildAWSView(t, configState)
	outputTarget := mustFindAWSChild(t, configActions, configChildren, "Set output format", actionSetOutput)
	switch _, err := ExecuteAction(actionSetOutput, outputTarget, "yaml"); {
	case err != nil:
		t.Fatalf("fake AWS profile output update failed: %v", err)
	default:
	}
}

func TestAWSServiceCatalogDetailedResourcesAndActions(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "fake-aws-services")
	script := `#!/bin/sh
case "$*" in
  "ec2 describe-instances --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Reservations":[{"Instances":[{"InstanceId":"i-123","State":{"Name":"stopped"},"Tags":[{"Key":"Name","Value":"web"}]}]}]}' ;;
  "ec2 describe-instances --instance-ids i-123 --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Reservations":[{"Instances":[{"InstanceId":"i-123","State":{"Name":"stopped"}}]}]}' ;;
  "ec2 start-instances --instance-ids i-123 --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"StartingInstances":[{"InstanceId":"i-123"}]}' ;;
  "ec2 describe-security-groups --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"SecurityGroups":[{"GroupId":"sg-123","GroupName":"web"}]}' ;;
  "ec2 describe-security-groups --group-ids sg-123 --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"SecurityGroups":[{"GroupId":"sg-123","GroupName":"web","IpPermissions":[]}]}' ;;
  "ec2 authorize-security-group-ingress --group-id sg-123 --ip-permissions "*" --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Return":true}' ;;
  "s3api list-buckets --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Buckets":[{"Name":"example-bucket"}]}' ;;
  "s3api get-bucket-location --bucket example-bucket --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"LocationConstraint":"us-east-1"}' ;;
  "s3api put-bucket-versioning --bucket example-bucket --versioning-configuration {\"Status\":\"Enabled\"} --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{}' ;;
  "iam list-roles --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Roles":[{"RoleName":"ExampleRole"}]}' ;;
  "iam get-role --role-name ExampleRole --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Role":{"RoleName":"ExampleRole"}}' ;;
  "iam update-assume-role-policy --role-name ExampleRole --policy-document "*" --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{}' ;;
  "iam list-policies --scope Local --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Policies":[{"PolicyName":"ExamplePolicy","Arn":"arn:aws:iam::123456789012:policy/ExamplePolicy"}]}' ;;
  "iam get-policy --policy-arn arn:aws:iam::123456789012:policy/ExamplePolicy --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Policy":{"PolicyName":"ExamplePolicy","DefaultVersionId":"v1"}}' ;;
  "iam create-policy-version --policy-arn arn:aws:iam::123456789012:policy/ExamplePolicy --policy-document "*" --set-as-default --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"PolicyVersion":{"VersionId":"v2","IsDefaultVersion":true}}' ;;
  "lambda list-functions --profile default --region us-east-1 --output json --no-cli-pager") printf '%s' '{"Functions":[{"FunctionName":"example"}]}' ;;
  *) printf 'unexpected fake AWS service command: %s\n' "$*" >&2; exit 97 ;;
esac
`
	switch err := os.WriteFile(fakePath, []byte(script), 0700); {
	case err != nil:
		t.Fatalf("cannot write fake AWS service CLI %s: %v", fakePath, err)
	default:
	}
	t.Setenv("DATATREE_AWS", fakePath)

	serviceResources, serviceChildren := mustBuildAWSView(t, []byte(`{"view":"services","profile":"default","region":"us-east-1"}`))
	instanceQuery := mustFindAWSChild(t, serviceResources, serviceChildren, "EC2 instances", "aws")
	instances, instanceChildren := mustBuildAWSView(t, instanceQuery)
	instanceState := mustFindAWSChild(t, instances, instanceChildren, "i-123 (stopped) web", "aws")
	instanceDetail, instanceDetailChildren := mustBuildAWSView(t, instanceState)
	instanceActionsState := mustFindAWSChild(t, instanceDetail, instanceDetailChildren, resourceActionsEntryName, "aws")
	instanceActions, instanceActionChildren := mustBuildAWSView(t, instanceActionsState)
	startTarget := mustFindAWSChild(t, instanceActions, instanceActionChildren, "Start instance", actionEC2Start)
	switch _, err := ExecuteAction(actionEC2Start, startTarget, ""); {
	case err != nil:
		t.Fatalf("fake EC2 start failed: %v", err)
	default:
	}

	securityQuery := mustFindAWSChild(t, serviceResources, serviceChildren, "EC2 security groups", "aws")
	groups, groupChildren := mustBuildAWSView(t, securityQuery)
	groupState := mustFindAWSChild(t, groups, groupChildren, "sg-123 web", "aws")
	groupDetail, groupDetailChildren := mustBuildAWSView(t, groupState)
	groupActionsState := mustFindAWSChild(t, groupDetail, groupDetailChildren, resourceActionsEntryName, "aws")
	groupActions, groupActionChildren := mustBuildAWSView(t, groupActionsState)
	authorizeTarget := mustFindAWSChild(t, groupActions, groupActionChildren, "Authorize ingress", actionSGAuthorize)
	permissions := `[{"IpProtocol":"tcp","FromPort":443,"ToPort":443,"IpRanges":[{"CidrIp":"203.0.113.0/24"}]}]`
	switch _, err := ExecuteAction(actionSGAuthorize, authorizeTarget, permissions); {
	case err != nil:
		t.Fatalf("fake security group ingress authorization failed: %v", err)
	default:
	}

	bucketQuery := mustFindAWSChild(t, serviceResources, serviceChildren, "S3 buckets", "aws")
	buckets, bucketChildren := mustBuildAWSView(t, bucketQuery)
	bucketState := mustFindAWSChild(t, buckets, bucketChildren, "example-bucket", "aws")
	bucketDetail, bucketDetailChildren := mustBuildAWSView(t, bucketState)
	bucketActionsState := mustFindAWSChild(t, bucketDetail, bucketDetailChildren, resourceActionsEntryName, "aws")
	bucketActions, bucketActionChildren := mustBuildAWSView(t, bucketActionsState)
	versionTarget := mustFindAWSChild(t, bucketActions, bucketActionChildren, "Set bucket versioning", actionS3Versioning)
	switch _, err := ExecuteAction(actionS3Versioning, versionTarget, "Enabled"); {
	case err != nil:
		t.Fatalf("fake S3 versioning update failed: %v", err)
	default:
	}

	roleQuery := mustFindAWSChild(t, serviceResources, serviceChildren, "IAM roles", "aws")
	roles, roleChildren := mustBuildAWSView(t, roleQuery)
	roleState := mustFindAWSChild(t, roles, roleChildren, "ExampleRole", "aws")
	roleDetail, roleDetailChildren := mustBuildAWSView(t, roleState)
	roleActionsState := mustFindAWSChild(t, roleDetail, roleDetailChildren, resourceActionsEntryName, "aws")
	roleActions, roleActionChildren := mustBuildAWSView(t, roleActionsState)
	trustTarget := mustFindAWSChild(t, roleActions, roleActionChildren, "Update role trust policy", actionIAMTrust)
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	switch _, err := ExecuteAction(actionIAMTrust, trustTarget, policy); {
	case err != nil:
		t.Fatalf("fake IAM trust policy update failed: %v", err)
	default:
	}

	policyQuery := mustFindAWSChild(t, serviceResources, serviceChildren, "IAM customer-managed policies", "aws")
	policies, policyChildren := mustBuildAWSView(t, policyQuery)
	policyState := mustFindAWSChild(t, policies, policyChildren, "ExamplePolicy arn:aws:iam::123456789012:policy/ExamplePolicy", "aws")
	policyDetail, policyDetailChildren := mustBuildAWSView(t, policyState)
	policyActionsState := mustFindAWSChild(t, policyDetail, policyDetailChildren, resourceActionsEntryName, "aws")
	policyActions, policyActionChildren := mustBuildAWSView(t, policyActionsState)
	versionPolicyTarget := mustFindAWSChild(t, policyActions, policyActionChildren, "Create and activate policy version", actionIAMPolicyVersion)
	switch _, err := ExecuteAction(actionIAMPolicyVersion, versionPolicyTarget, policy); {
	case err != nil:
		t.Fatalf("fake IAM managed policy version failed: %v", err)
	default:
	}

	lambdaQuery := mustFindAWSChild(t, serviceResources, serviceChildren, "Lambda functions", "aws")
	lambdaResources, _ := mustBuildAWSView(t, lambdaQuery)
	mustFindAWSResource(t, lambdaResources, "[response]", "json")
}

func TestAWSCommandGatesRejectUnsafeShapes(t *testing.T) {
	switch {
	case validRemoteInvocation([]string{"s3", "rm", "s3://bucket", "--recursive"}, "s3", "rm"):
		t.Fatal("read-only AWS gate accepted s3 rm")
	case validConfigureSetInvocation([]string{"configure", "set", "aws_secret_access_key", "secret", "--profile", "default"}):
		t.Fatal("AWS action gate accepted a credential write")
	case validCloudControlActionInvocation([]string{"cloudcontrol", "delete-resource", "--type-name", "AWS::Logs::LogGroup", "--identifier", "example", "--force"}, "delete-resource"):
		t.Fatal("AWS action gate accepted an unsafe delete shape")
	case validAWSServiceActionInvocation([]string{"s3api", "delete-bucket", "--bucket", "example", "--profile", "default", "--region", "us-east-1", "--output", "json", "--no-cli-pager"}):
		t.Fatal("AWS service action gate accepted bucket deletion")
	default:
	}
}

func mustBuildAWSView(t *testing.T, state []byte) ([]driverlib.Resource, map[string][]byte) {
	t.Helper()
	resources, children, err := buildAWSView(state)
	switch {
	case err != nil:
		t.Fatalf("cannot build AWS view from %q: %v", string(state), err)
	default:
		return resources, children
	}
	return nil, nil
}

func mustFindAWSChild(t *testing.T, resources []driverlib.Resource, children map[string][]byte, name string, kind string) []byte {
	t.Helper()
	mustFindAWSResource(t, resources, name, kind)
	child, ok := children[name]
	switch {
	case !ok:
		t.Fatalf("AWS resource %q kind %q had no child payload", name, kind)
	default:
		return child
	}
	return nil
}

func mustFindAWSResource(t *testing.T, resources []driverlib.Resource, name string, kind string) {
	t.Helper()
	for _, resource := range resources {
		switch {
		case resource.Name == name && resource.Kind == kind:
			return
		default:
		}
	}
	t.Fatalf("AWS resource %q kind %q not found in %+v", name, kind, resources)
}
