package awsdriver

import (
	"encoding/json"
	"fmt"
	"strings"

	"datahumper/drivers_go/driverlib"
)

const viewData = "data"

// Each edge turns a named resource in a list response into its data query.
// Arguments are compiled templates, never commands supplied by remote data.
type dataEdge struct {
	id, parent, collection, field, label string
	command                              []string
}

var dataEdges = []dataEdge{
	{"role-inline", "iam-roles", "", "", "Inline policies", []string{"iam", "list-role-policies", "--role-name={id}"}},
	{"role-policy-document", "role-inline", "PolicyNames", "", "Policy document", []string{"iam", "get-role-policy", "--role-name={parent}", "--policy-name={id}"}},
	{"policy-versions", "iam-policies", "", "", "Policy versions", []string{"iam", "list-policy-versions", "--policy-arn={id}"}},
	{"policy-document", "policy-versions", "Versions", "VersionId", "Policy document", []string{"iam", "get-policy-version", "--policy-arn={parent}", "--version-id={id}"}},
	{"user-policies", "iam-users", "Users", "UserName", "Inline policies", []string{"iam", "list-user-policies", "--user-name={id}"}},
	{"user-policy-document", "user-policies", "PolicyNames", "", "Policy document", []string{"iam", "get-user-policy", "--user-name={parent}", "--policy-name={id}"}},
	{"group-policies", "iam-groups", "Groups", "GroupName", "Inline policies", []string{"iam", "list-group-policies", "--group-name={id}"}},
	{"group-policy-document", "group-policies", "PolicyNames", "", "Policy document", []string{"iam", "get-group-policy", "--group-name={parent}", "--policy-name={id}"}},
	{"analyzer-findings", "access-analyzers", "analyzers", "arn", "Findings", []string{"accessanalyzer", "list-findings", "--analyzer-arn={id}"}},
	{"detector-findings", "guardduty-detectors", "DetectorIds", "", "Finding identifiers", []string{"guardduty", "list-findings", "--detector-id={id}"}},
	{"finding-data", "detector-findings", "FindingIds", "", "Finding data", []string{"guardduty", "get-findings", "--detector-id={parent}", "--finding-ids={id}"}},
	{"rule-compliance", "config-rules", "ConfigRules", "ConfigRuleName", "Compliance data", []string{"configservice", "get-compliance-details-by-config-rule", "--config-rule-name={id}"}},
	{"table-items", "dynamodb-tables", "TableNames", "", "Items", []string{"dynamodb", "scan", "--table-name={id}"}},
	{"log-streams", "log-groups", "logGroups", "logGroupName", "Streams", []string{"logs", "describe-log-streams", "--log-group-name={id}"}},
	{"log-events", "log-streams", "logStreams", "logStreamName", "Events", []string{"logs", "get-log-events", "--log-group-name={parent}", "--log-stream-name={id}"}},
	{"zone-records", "route53-zones", "HostedZones", "Id", "DNS records", []string{"route53", "list-resource-record-sets", "--hosted-zone-id={id}"}},
	{"repository-images", "ecr-repositories", "repositories", "repositoryName", "Images", []string{"ecr", "describe-images", "--repository-name={id}"}},
	{"vault-points", "backup-vaults", "BackupVaultList", "BackupVaultName", "Recovery points", []string{"backup", "list-recovery-points-by-backup-vault", "--backup-vault-name={id}"}},
	{"lambda-code", "lambda-functions", "Functions", "FunctionName", "Function configuration and code download", []string{"lambda", "get-function", "--function-name={id}"}},
	{"ecs-tasks", "ecs-clusters", "clusterArns", "", "Tasks", []string{"ecs", "list-tasks", "--cluster={id}"}},
	{"ecs-services", "ecs-clusters", "clusterArns", "", "Services", []string{"ecs", "list-services", "--cluster={id}"}},
	{"ecs-task-detail", "ecs-tasks", "taskArns", "", "Task data", []string{"ecs", "describe-tasks", "--cluster={parent}", "--tasks={id}"}},
	{"ecs-service-detail", "ecs-services", "serviceArns", "", "Service data", []string{"ecs", "describe-services", "--cluster={parent}", "--services={id}"}},
	{"eks-detail", "eks-clusters", "clusters", "", "Cluster data", []string{"eks", "describe-cluster", "--name={id}"}},
	{"state-definition", "step-functions", "stateMachines", "stateMachineArn", "Definition", []string{"stepfunctions", "describe-state-machine", "--state-machine-arn={id}"}},
	{"state-executions", "step-functions", "stateMachines", "stateMachineArn", "Executions", []string{"stepfunctions", "list-executions", "--state-machine-arn={id}"}},
	{"execution-history", "state-executions", "executions", "executionArn", "Execution events", []string{"stepfunctions", "get-execution-history", "--execution-arn={id}"}},
	{"rule-targets", "eventbridge-rules", "Rules", "Name", "Targets", []string{"events", "list-targets-by-rule", "--rule={id}"}},
	{"secret-value", "secrets", "SecretList", "ARN", "Secret value", []string{"secretsmanager", "get-secret-value", "--secret-id={id}"}},
	{"key-detail", "kms-keys", "Keys", "KeyId", "Key data", []string{"kms", "describe-key", "--key-id={id}"}},
	{"topic-attributes", "sns-topics", "Topics", "TopicArn", "Topic attributes", []string{"sns", "get-topic-attributes", "--topic-arn={id}"}},
	{"topic-subscriptions", "sns-topics", "Topics", "TopicArn", "Subscriptions", []string{"sns", "list-subscriptions-by-topic", "--topic-arn={id}"}},
	{"queue-attributes", "sqs-queues", "QueueUrls", "", "Queue attributes", []string{"sqs", "get-queue-attributes", "--queue-url={id}", "--attribute-names", "All"}},
	{"api-resources", "api-gateway-apis", "items", "id", "API resources", []string{"apigateway", "get-resources", "--rest-api-id={id}"}},
	{"certificate-detail", "certificates", "CertificateSummaryList", "CertificateArn", "Certificate data", []string{"acm", "describe-certificate", "--certificate-arn={id}"}},
	{"load-balancer-listeners", "load-balancers", "LoadBalancers", "LoadBalancerArn", "Listeners", []string{"elbv2", "describe-listeners", "--load-balancer-arn={id}"}},
	{"stack-resources", "cloudformation-stacks", "Stacks", "StackName", "Stack resources", []string{"cloudformation", "describe-stack-resources", "--stack-name={id}"}},
	{"stack-template", "cloudformation-stacks", "Stacks", "StackName", "Template", []string{"cloudformation", "get-template", "--stack-name={id}"}},
}

func dataEdgeFor(id string) (dataEdge, bool) {
	for _, edge := range dataEdges {
		switch {
		case edge.id == id:
			return edge, true
		default:
		}
	}
	return dataEdge{}, false
}

func validateDataState(state mountState) error {
	edge, ok := dataEdgeFor(state.Operation)
	switch {
	case !ok || !validProfile(state.Profile) || !validRegion(state.Region) || !validIdentifier(state.Identifier) || state.TypeName != "" || !validPageToken(state.Token) || (state.Token != "" && state.Operation != "log-events"):
		return fmt.Errorf("invalid AWS data state: expected profile, region, allowlisted query and resource identifier")
	case strings.Contains(strings.Join(edge.command, " "), "{parent}") && !validIdentifier(state.Key):
		return fmt.Errorf("invalid AWS %s data state: missing parent resource identifier", state.Operation)
	case state.Key != "" && !validIdentifier(state.Key):
		return fmt.Errorf("invalid AWS %s data state: invalid parent resource identifier", state.Operation)
	default:
		return nil
	}
}

func dataArgs(edge dataEdge, state mountState) []string {
	command := make([]string, len(edge.command))
	for index, arg := range edge.command {
		switch {
		case strings.HasSuffix(arg, "{id}"):
			command[index] = strings.TrimSuffix(arg, "{id}") + state.Identifier
		case strings.HasSuffix(arg, "{parent}"):
			command[index] = strings.TrimSuffix(arg, "{parent}") + state.Key
		default:
			command[index] = arg
		}
	}
	switch {
	case edge.id == "log-events":
		command = append(command, "--start-from-head")
		switch {
		case state.Token != "":
			command = append(command, "--next-token="+state.Token)
		default:
		}
	default:
	}
	return awsRemoteArgs(command, state.Profile, state.Region)
}

func validDataReadInvocation(args []string) bool {
	profile, region, ok := validateAWSTail(args)
	switch {
	case !ok || !validProfile(profile) || !validRegion(region):
		return false
	default:
	}
	prefix := args[:len(args)-7]
	for _, edge := range dataEdges {
		candidate := prefix
		switch {
		case edge.id == "log-events":
			switch {
			case len(candidate) == len(edge.command)+2 && strings.HasPrefix(candidate[len(candidate)-1], "--next-token=") && validPageToken(strings.TrimPrefix(candidate[len(candidate)-1], "--next-token=")):
				candidate = candidate[:len(candidate)-1]
			default:
			}
			switch {
			case len(candidate) != len(edge.command)+1 || candidate[len(candidate)-1] != "--start-from-head":
				continue
			default:
				candidate = candidate[:len(candidate)-1]
			}
		default:
		}
		switch {
		case len(candidate) != len(edge.command):
			continue
		default:
		}
		match := true
		for index, template := range edge.command {
			flag, _, parameter := strings.Cut(template, "={")
			switch {
			case parameter:
				value, found := strings.CutPrefix(candidate[index], flag+"=")
				match = match && found && validIdentifier(value)
			default:
				match = match && candidate[index] == template
			}
		}
		switch {
		case match:
			return true
		default:
		}
	}
	return false
}

func listAWSData(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	edge, ok := dataEdgeFor(state.Operation)
	switch {
	case !ok:
		return nil, nil, fmt.Errorf("unsupported AWS data query %q", state.Operation)
	default:
	}
	out, err := runReadOnlyAWS(config, dataArgs(edge, state))
	switch {
	case err != nil:
		return nil, nil, err
	default:
		resources, children, err := listAWSDataResources(out, state)
		switch {
		case err != nil:
			return nil, nil, err
		case edge.id == "log-events":
			var page struct{ NextForwardToken string }
			switch err := json.Unmarshal(out, &page); {
			case err != nil:
				return nil, nil, fmt.Errorf("invalid log event page: %v", err)
			case !validPageToken(page.NextForwardToken):
				return nil, nil, fmt.Errorf("invalid log event continuation token")
			case page.NextForwardToken != "" && page.NextForwardToken != state.Token:
				state.Token = page.NextForwardToken
				raw, err := encodeMountState(state)
				switch {
				case err != nil:
					return nil, nil, err
				default:
					resources = append(resources, driverlib.Resource{Name: "[next page]", Kind: "aws"})
					children["[next page]"] = raw
				}
			default:
			}
		default:
		}
		return resources, children, nil
	}
}

func listAWSDataResources(out []byte, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	resources, children, err := rawAWSResponse(out, state.Operation)
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	for _, edge := range dataEdges {
		switch {
		case edge.parent != state.Operation:
			continue
		default:
		}
		var response map[string]json.RawMessage
		switch err := json.Unmarshal(out, &response); {
		case err != nil || response == nil:
			return nil, nil, fmt.Errorf("cannot browse AWS %s: expected response object: %v", state.Operation, err)
		default:
		}
		var entries []json.RawMessage
		switch {
		case edge.collection == "":
			identifier, err := json.Marshal(state.Identifier)
			switch {
			case err != nil:
				return nil, nil, err
			default:
				entries = []json.RawMessage{identifier}
			}
		default:
			switch err := json.Unmarshal(response[edge.collection], &entries); {
			case err != nil:
				return nil, nil, fmt.Errorf("cannot browse AWS %s: expected %s array: %v", state.Operation, edge.collection, err)
			default:
			}
		}
		for _, entry := range entries {
			value := entry
			switch {
			case edge.field != "":
				var object map[string]json.RawMessage
				switch err := json.Unmarshal(entry, &object); {
				case err != nil:
					return nil, nil, fmt.Errorf("invalid AWS %s resource: expected object: %v", state.Operation, err)
				default:
					value = object[edge.field]
				}
			default:
			}
			var identifier string
			switch err := json.Unmarshal(value, &identifier); {
			case err != nil || !validIdentifier(identifier):
				return nil, nil, fmt.Errorf("invalid AWS %s resource: expected non-empty %s identifier: %v", state.Operation, edge.field, err)
			default:
			}
			next := mountState{View: viewData, Profile: state.Profile, Region: state.Region, Operation: edge.id, Identifier: identifier, Key: state.Identifier}
			raw, err := encodeMountState(next)
			switch {
			case err != nil:
				return nil, nil, err
			default:
				key := fmt.Sprintf("data-%d", len(resources))
				resources = append(resources, driverlib.Resource{Name: identifier + " [" + edge.label + "]", Kind: "aws", Key: key})
				children[key] = raw
			}
		}
	}
	switch state.Operation {
	case "queue-attributes":
		resources = append(resources, driverlib.Resource{Name: "Message bodies require SQS ReceiveMessage, which changes queue visibility; read-only browsing cannot receive messages.", Kind: "error"})
	case "rds-instances", "rds-clusters", "elasticache-clusters":
		resources = append(resources, driverlib.Resource{Name: "Records require a database connection and database credentials; AWS management APIs only return configuration.", Kind: "error"})
	default:
	}
	return resources, children, nil
}
