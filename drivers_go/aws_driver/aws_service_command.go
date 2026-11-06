package awsdriver

type awsServiceOperation struct {
	ID       string
	Label    string
	Command  []string
	Detailed bool
}

var awsServiceOperations = []awsServiceOperation{
	{ID: "ec2-instances", Label: "EC2 instances", Command: []string{"ec2", "describe-instances"}, Detailed: true},
	{ID: "ec2-security-groups", Label: "EC2 security groups", Command: []string{"ec2", "describe-security-groups"}, Detailed: true},
	{ID: "ec2-vpcs", Label: "EC2 VPCs", Command: []string{"ec2", "describe-vpcs"}},
	{ID: "ec2-subnets", Label: "EC2 subnets", Command: []string{"ec2", "describe-subnets"}},
	{ID: "ec2-volumes", Label: "EC2 volumes", Command: []string{"ec2", "describe-volumes"}},
	{ID: "ec2-snapshots", Label: "EC2 snapshots owned by this account", Command: []string{"ec2", "describe-snapshots", "--owner-ids", "self"}},
	{ID: "ec2-images", Label: "EC2 images owned by this account", Command: []string{"ec2", "describe-images", "--owners", "self"}},
	{ID: "ec2-addresses", Label: "EC2 elastic IP addresses", Command: []string{"ec2", "describe-addresses"}},
	{ID: "ec2-network-interfaces", Label: "EC2 network interfaces", Command: []string{"ec2", "describe-network-interfaces"}},
	{ID: "ec2-route-tables", Label: "EC2 route tables", Command: []string{"ec2", "describe-route-tables"}},
	{ID: "ec2-key-pairs", Label: "EC2 key pairs", Command: []string{"ec2", "describe-key-pairs"}},
	{ID: "s3-buckets", Label: "S3 buckets", Command: []string{"s3api", "list-buckets"}, Detailed: true},
	{ID: "iam-account", Label: "IAM account authorization details", Command: []string{"iam", "get-account-authorization-details"}},
	{ID: "iam-summary", Label: "IAM account summary", Command: []string{"iam", "get-account-summary"}},
	{ID: "iam-users", Label: "IAM users", Command: []string{"iam", "list-users"}},
	{ID: "iam-groups", Label: "IAM groups", Command: []string{"iam", "list-groups"}},
	{ID: "iam-roles", Label: "IAM roles", Command: []string{"iam", "list-roles"}, Detailed: true},
	{ID: "iam-policies", Label: "IAM customer-managed policies", Command: []string{"iam", "list-policies", "--scope", "Local"}, Detailed: true},
	{ID: "access-analyzers", Label: "Access Analyzer analyzers", Command: []string{"accessanalyzer", "list-analyzers"}},
	{ID: "securityhub-findings", Label: "Security Hub findings", Command: []string{"securityhub", "get-findings", "--max-results", "100"}},
	{ID: "guardduty-detectors", Label: "GuardDuty detectors", Command: []string{"guardduty", "list-detectors"}},
	{ID: "cloudtrail-trails", Label: "CloudTrail trails", Command: []string{"cloudtrail", "describe-trails"}},
	{ID: "config-recorders", Label: "AWS Config recorders", Command: []string{"configservice", "describe-configuration-recorders"}},
	{ID: "config-rules", Label: "AWS Config rules", Command: []string{"configservice", "describe-config-rules"}},
	{ID: "lambda-functions", Label: "Lambda functions", Command: []string{"lambda", "list-functions"}},
	{ID: "rds-instances", Label: "RDS database instances", Command: []string{"rds", "describe-db-instances"}},
	{ID: "rds-clusters", Label: "RDS database clusters", Command: []string{"rds", "describe-db-clusters"}},
	{ID: "ecs-clusters", Label: "ECS clusters", Command: []string{"ecs", "list-clusters"}},
	{ID: "eks-clusters", Label: "EKS clusters", Command: []string{"eks", "list-clusters"}},
	{ID: "dynamodb-tables", Label: "DynamoDB tables", Command: []string{"dynamodb", "list-tables"}},
	{ID: "sqs-queues", Label: "SQS queues", Command: []string{"sqs", "list-queues"}},
	{ID: "sns-topics", Label: "SNS topics", Command: []string{"sns", "list-topics"}},
	{ID: "cloudformation-stacks", Label: "CloudFormation stacks", Command: []string{"cloudformation", "describe-stacks"}},
	{ID: "log-groups", Label: "CloudWatch log groups", Command: []string{"logs", "describe-log-groups"}},
	{ID: "cloudwatch-alarms", Label: "CloudWatch alarms", Command: []string{"cloudwatch", "describe-alarms"}},
	{ID: "api-gateway-apis", Label: "API Gateway REST APIs", Command: []string{"apigateway", "get-rest-apis"}},
	{ID: "elasticache-clusters", Label: "ElastiCache clusters", Command: []string{"elasticache", "describe-cache-clusters"}},
	{ID: "load-balancers", Label: "Elastic Load Balancing v2 load balancers", Command: []string{"elbv2", "describe-load-balancers"}},
	{ID: "route53-zones", Label: "Route 53 hosted zones", Command: []string{"route53", "list-hosted-zones"}},
	{ID: "secrets", Label: "Secrets Manager secret metadata", Command: []string{"secretsmanager", "list-secrets"}},
	{ID: "kms-keys", Label: "KMS keys", Command: []string{"kms", "list-keys"}},
	{ID: "ecr-repositories", Label: "ECR repositories", Command: []string{"ecr", "describe-repositories"}},
	{ID: "backup-vaults", Label: "AWS Backup vaults", Command: []string{"backup", "list-backup-vaults"}},
	{ID: "step-functions", Label: "Step Functions state machines", Command: []string{"stepfunctions", "list-state-machines"}},
	{ID: "eventbridge-rules", Label: "EventBridge rules", Command: []string{"events", "list-rules"}},
	{ID: "autoscaling-groups", Label: "EC2 Auto Scaling groups", Command: []string{"autoscaling", "describe-auto-scaling-groups"}},
	{ID: "certificates", Label: "Certificate Manager certificates", Command: []string{"acm", "list-certificates"}},
	{ID: "waf-web-acls", Label: "WAF regional web ACLs", Command: []string{"wafv2", "list-web-acls", "--scope", "REGIONAL"}},
	{ID: "organizations-accounts", Label: "Organizations accounts", Command: []string{"organizations", "list-accounts"}},
	{ID: "systems-manager-nodes", Label: "Systems Manager managed nodes", Command: []string{"ssm", "describe-instance-information"}},
}

func serviceOperation(id string) (awsServiceOperation, bool) {
	for _, operation := range awsServiceOperations {
		switch {
		case operation.ID == id:
			return operation, true
		default:
		}
	}
	return awsServiceOperation{}, false
}

func validServiceOperation(id string) bool {
	_, ok := serviceOperation(id)
	return ok
}

func validDetailedOperation(id string) bool {
	operation, ok := serviceOperation(id)
	return ok && operation.Detailed
}

func serviceQueryArgs(profile string, region string, operation string) []string {
	definition, ok := serviceOperation(operation)
	switch {
	case !ok:
		panic("unsupported AWS service operation " + operation)
	default:
		return awsRemoteArgs(definition.Command, profile, region)
	}
}

func serviceResourceArgs(profile string, region string, operation string, identifier string) []string {
	var command []string
	switch operation {
	case "ec2-instances":
		command = []string{"ec2", "describe-instances", "--instance-ids", identifier}
	case "ec2-security-groups":
		command = []string{"ec2", "describe-security-groups", "--group-ids", identifier}
	case "s3-buckets":
		command = []string{"s3api", "get-bucket-location", "--bucket", identifier}
	case "iam-roles":
		command = []string{"iam", "get-role", "--role-name", identifier}
	case "iam-policies":
		command = []string{"iam", "get-policy", "--policy-arn", identifier}
	default:
		panic("unsupported detailed AWS service operation " + operation)
	}
	return awsRemoteArgs(command, profile, region)
}

func validServiceReadInvocation(args []string) bool {
	profile, region, tailOK := validateAWSTail(args)
	switch {
	case !tailOK || !validProfile(profile) || !validRegion(region):
		return false
	default:
	}
	prefix := args[:len(args)-7]
	for _, operation := range awsServiceOperations {
		switch {
		case stringSlicesEqual(prefix, operation.Command):
			return true
		default:
		}
	}
	return validDetailedServiceReadPrefix(prefix)
}

func validDetailedServiceReadPrefix(prefix []string) bool {
	switch {
	case len(prefix) != 4 || !validIdentifier(prefix[3]):
		return false
	case prefix[0] == "ec2" && prefix[1] == "describe-instances" && prefix[2] == "--instance-ids":
		return true
	case prefix[0] == "ec2" && prefix[1] == "describe-security-groups" && prefix[2] == "--group-ids":
		return true
	case prefix[0] == "s3api" && prefix[1] == "get-bucket-location" && prefix[2] == "--bucket":
		return true
	case prefix[0] == "iam" && prefix[1] == "get-role" && prefix[2] == "--role-name":
		return true
	case prefix[0] == "iam" && prefix[1] == "get-policy" && prefix[2] == "--policy-arn":
		return true
	default:
		return false
	}
}
