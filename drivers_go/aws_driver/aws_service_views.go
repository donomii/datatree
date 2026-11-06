package awsdriver

import (
	"encoding/json"
	"fmt"
	"sort"

	"datahumper/drivers_go/driverlib"
)

func listAWSServices(state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	for _, operation := range awsServiceOperations {
		stateBytes, err := encodeMountState(mountState{View: viewServiceQuery, Profile: state.Profile, Region: state.Region, Operation: operation.ID})
		switch {
		case err != nil:
			return nil, nil, err
		default:
		}
		resources = append(resources, driverlib.Resource{Name: operation.Label, Kind: "aws", Size: 0})
		children[operation.Label] = stateBytes
	}
	sort.Slice(resources, func(left int, right int) bool { return resources[left].Name < resources[right].Name })
	return resources, children, nil
}

func listAWSServiceQuery(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyAWS(config, serviceQueryArgs(state.Profile, state.Region, state.Operation))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	switch state.Operation {
	case "ec2-instances":
		return listEC2Instances(out, state)
	case "ec2-security-groups":
		return listEC2SecurityGroups(out, state)
	case "s3-buckets":
		return listS3Buckets(out, state)
	case "iam-roles":
		return listIAMRoles(out, state)
	case "iam-policies":
		return listIAMPolicies(out, state)
	default:
		return rawAWSResponse(out, state.Operation)
	}
}

func rawAWSResponse(out []byte, operation string) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case !json.Valid(out):
		return nil, nil, fmt.Errorf("cannot parse AWS %s output: expected JSON, got %d bytes", operation, len(out))
	default:
		return []driverlib.Resource{{Name: "[response]", Kind: "json", Size: int64(len(out))}}, map[string][]byte{"[response]": out}, nil
	}
}

func detailedAWSResources(state mountState, identifiers []string, labels map[string]string) ([]driverlib.Resource, map[string][]byte, error) {
	sort.Strings(identifiers)
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	for _, identifier := range identifiers {
		switch {
		case !validIdentifier(identifier):
			return nil, nil, fmt.Errorf("cannot list AWS %s resources: invalid identifier %q", state.Operation, identifier)
		default:
		}
		name := labels[identifier]
		stateBytes, err := encodeMountState(mountState{View: viewServiceResource, Profile: state.Profile, Region: state.Region, Operation: state.Operation, Identifier: identifier})
		switch {
		case err != nil:
			return nil, nil, err
		default:
		}
		resources = append(resources, driverlib.Resource{Name: name, Kind: "aws", Size: 0})
		children[name] = stateBytes
	}
	return resources, children, nil
}

func listEC2Instances(out []byte, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	var response struct {
		Reservations []struct {
			Instances []struct {
				InstanceID string `json:"InstanceId"`
				State      struct {
					Name string `json:"Name"`
				} `json:"State"`
				Tags []struct {
					Key   string `json:"Key"`
					Value string `json:"Value"`
				} `json:"Tags"`
			} `json:"Instances"`
		} `json:"Reservations"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse EC2 instances: expected Reservations JSON, got %d bytes with error: %v", len(out), err)
	default:
	}
	identifiers := []string{}
	labels := map[string]string{}
	for _, reservation := range response.Reservations {
		for _, instance := range reservation.Instances {
			name := ""
			for _, tag := range instance.Tags {
				switch {
				case tag.Key == "Name":
					name = tag.Value
				default:
				}
			}
			label := instance.InstanceID + " (" + instance.State.Name + ")"
			switch {
			case name != "":
				label += " " + name
			default:
			}
			identifiers = append(identifiers, instance.InstanceID)
			labels[instance.InstanceID] = label
		}
	}
	return detailedAWSResources(state, identifiers, labels)
}

func listEC2SecurityGroups(out []byte, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	var response struct {
		SecurityGroups []struct {
			GroupID   string `json:"GroupId"`
			GroupName string `json:"GroupName"`
		} `json:"SecurityGroups"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse EC2 security groups: expected SecurityGroups JSON, got %d bytes with error: %v", len(out), err)
	default:
	}
	identifiers := []string{}
	labels := map[string]string{}
	for _, group := range response.SecurityGroups {
		identifiers = append(identifiers, group.GroupID)
		labels[group.GroupID] = group.GroupID + " " + group.GroupName
	}
	return detailedAWSResources(state, identifiers, labels)
}

func listS3Buckets(out []byte, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	var response struct {
		Buckets []struct {
			Name string `json:"Name"`
		} `json:"Buckets"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse S3 buckets: expected Buckets JSON, got %d bytes with error: %v", len(out), err)
	default:
	}
	identifiers := []string{}
	labels := map[string]string{}
	for _, bucket := range response.Buckets {
		identifiers = append(identifiers, bucket.Name)
		labels[bucket.Name] = bucket.Name
	}
	return detailedAWSResources(state, identifiers, labels)
}

func listIAMRoles(out []byte, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	var response struct {
		Roles []struct {
			RoleName string `json:"RoleName"`
		} `json:"Roles"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse IAM roles: expected Roles JSON, got %d bytes with error: %v", len(out), err)
	default:
	}
	identifiers := []string{}
	labels := map[string]string{}
	for _, role := range response.Roles {
		identifiers = append(identifiers, role.RoleName)
		labels[role.RoleName] = role.RoleName
	}
	return detailedAWSResources(state, identifiers, labels)
}

func listIAMPolicies(out []byte, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	var response struct {
		Policies []struct {
			ARN        string `json:"Arn"`
			PolicyName string `json:"PolicyName"`
		} `json:"Policies"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse IAM policies: expected Policies JSON, got %d bytes with error: %v", len(out), err)
	default:
	}
	identifiers := []string{}
	labels := map[string]string{}
	for _, policy := range response.Policies {
		identifiers = append(identifiers, policy.ARN)
		labels[policy.ARN] = policy.PolicyName + " " + policy.ARN
	}
	return detailedAWSResources(state, identifiers, labels)
}

func listAWSServiceResource(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyAWS(config, serviceResourceArgs(state.Profile, state.Region, state.Operation, state.Identifier))
	switch {
	case err != nil:
		return nil, nil, err
	case !json.Valid(out):
		return nil, nil, fmt.Errorf("cannot parse AWS %s resource %q: expected JSON, got %d bytes", state.Operation, state.Identifier, len(out))
	default:
	}
	actionsState, stateErr := encodeMountState(mountState{View: viewServiceActions, Profile: state.Profile, Region: state.Region, Operation: state.Operation, Identifier: state.Identifier})
	switch {
	case stateErr != nil:
		return nil, nil, stateErr
	default:
	}
	resources := []driverlib.Resource{{Name: resourceActionsEntryName, Kind: "aws", Size: 0}, {Name: "[details]", Kind: "json", Size: int64(len(out))}}
	children := map[string][]byte{resourceActionsEntryName: actionsState, "[details]": out}
	return resources, children, nil
}

func listAWSServiceActions(state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	target, err := encodeActionTarget(actionTarget{Profile: state.Profile, Region: state.Region, Operation: state.Operation, Identifier: state.Identifier})
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	add := func(name string, kind string) {
		resources = append(resources, driverlib.Resource{Name: name, Kind: kind, Size: int64(len(target))})
		children[name] = target
	}
	switch state.Operation {
	case "ec2-instances":
		add("Start instance", actionEC2Start)
		add("Stop instance", actionEC2Stop)
		add("Reboot instance", actionEC2Reboot)
		add("Terminate instance", actionEC2Terminate)
	case "ec2-security-groups":
		add("Authorize ingress", actionSGAuthorize)
		add("Revoke ingress", actionSGRevoke)
	case "s3-buckets":
		add("Set bucket policy", actionS3PutPolicy)
		add("Delete bucket policy", actionS3DeletePolicy)
		add("Set bucket versioning", actionS3Versioning)
	case "iam-roles":
		add("Update role trust policy", actionIAMTrust)
	case "iam-policies":
		add("Create and activate policy version", actionIAMPolicyVersion)
	default:
		return nil, nil, fmt.Errorf("cannot list AWS actions for unsupported operation %q", state.Operation)
	}
	return resources, children, nil
}
