package awsdriver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"datahumper/drivers_go/driverlib"
)

func listResourceSearch(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyAWS(config, resourceSearchArgs(state.Profile, state.Region))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var response struct {
		Resources []json.RawMessage `json:"Resources"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse AWS Resource Explorer output for profile %q region %q: expected Resources JSON, got %d bytes with error: %v", state.Profile, state.Region, len(out), err)
	default:
	}
	type namedResource struct {
		name string
		raw  []byte
	}
	named := []namedResource{}
	for _, raw := range response.Resources {
		var resource struct {
			ARN          string `json:"Arn"`
			ResourceType string `json:"ResourceType"`
			Region       string `json:"Region"`
		}
		switch err := json.Unmarshal(raw, &resource); {
		case err != nil:
			return nil, nil, fmt.Errorf("cannot parse one AWS Resource Explorer result: expected Arn, ResourceType and Region, got %q with error: %v", string(raw), err)
		case resource.ARN == "":
			return nil, nil, fmt.Errorf("cannot parse one AWS Resource Explorer result: expected a non-empty Arn, got %q", string(raw))
		default:
		}
		name := resource.ResourceType + " " + resource.ARN
		named = append(named, namedResource{name: name, raw: raw})
	}
	sort.Slice(named, func(left int, right int) bool { return named[left].name < named[right].name })
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	for _, resource := range named {
		var summary struct {
			Arn          string
			ResourceType string
		}
		switch err := json.Unmarshal(resource.raw, &summary); {
		case err != nil:
			return nil, nil, fmt.Errorf("invalid Resource Explorer entry: %v", err)
		default:
		}
		arn := strings.SplitN(summary.Arn, ":", 6)
		switch {
		case summary.ResourceType == "s3:bucket" && len(arn) == 6 && arn[2] == "s3" && validS3Bucket(arn[5]):
			bucket, err := encodeMountState(mountState{View: viewS3Objects, Profile: state.Profile, Region: state.Region, Operation: "s3-buckets", Identifier: arn[5]})
			switch {
			case err != nil:
				return nil, nil, err
			default:
				resources = append(resources, driverlib.Resource{Name: resource.name, Kind: "aws"})
				children[resource.name] = bucket
			}
		default:
			resources = append(resources, driverlib.Resource{Name: resource.name, Kind: "json", Size: int64(len(resource.raw))})
			children[resource.name] = resource.raw
		}
	}
	return resources, children, nil
}

func listResourceTypes(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyAWS(config, resourceTypeListArgs(state.Profile, state.Region))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var response struct {
		TypeSummaries []struct {
			TypeName         string `json:"TypeName"`
			ProvisioningType string `json:"ProvisioningType"`
		} `json:"TypeSummaries"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse AWS CloudFormation type list for profile %q region %q: expected TypeSummaries JSON, got %d bytes with error: %v", state.Profile, state.Region, len(out), err)
	default:
	}
	sort.Slice(response.TypeSummaries, func(left int, right int) bool {
		return response.TypeSummaries[left].TypeName < response.TypeSummaries[right].TypeName
	})
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	for _, summary := range response.TypeSummaries {
		switch {
		case !validTypeName(summary.TypeName):
			return nil, nil, fmt.Errorf("cannot list AWS resource types: received invalid type name %q", summary.TypeName)
		case summary.ProvisioningType == "NON_PROVISIONABLE":
			continue
		default:
		}
		stateBytes, stateErr := encodeMountState(mountState{View: viewType, Profile: state.Profile, Region: state.Region, TypeName: summary.TypeName})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: summary.TypeName, Kind: "aws", Size: 0})
		children[summary.TypeName] = stateBytes
	}
	return resources, children, nil
}

func listResources(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyAWS(config, resourceListArgs(state.Profile, state.Region, state.TypeName))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var response struct {
		ResourceDescriptions []struct {
			Identifier string `json:"Identifier"`
		} `json:"ResourceDescriptions"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse AWS Cloud Control resource list for type %q: expected ResourceDescriptions JSON, got %d bytes with error: %v", state.TypeName, len(out), err)
	default:
	}
	sort.Slice(response.ResourceDescriptions, func(left int, right int) bool {
		return response.ResourceDescriptions[left].Identifier < response.ResourceDescriptions[right].Identifier
	})
	createTarget, createErr := encodeActionTarget(actionTarget{Profile: state.Profile, Region: state.Region, TypeName: state.TypeName})
	switch {
	case createErr != nil:
		return nil, nil, createErr
	default:
	}
	resources := []driverlib.Resource{{Name: "Create resource", Kind: actionCreate, Size: int64(len(createTarget))}}
	children := map[string][]byte{"Create resource": createTarget}
	for _, description := range response.ResourceDescriptions {
		switch {
		case !validIdentifier(description.Identifier):
			return nil, nil, fmt.Errorf("cannot list AWS resources for type %q: received invalid identifier %q", state.TypeName, description.Identifier)
		default:
		}
		stateBytes, stateErr := encodeMountState(mountState{View: viewResource, Profile: state.Profile, Region: state.Region, TypeName: state.TypeName, Identifier: description.Identifier})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: description.Identifier, Kind: "aws", Size: 0})
		children[description.Identifier] = stateBytes
	}
	return resources, children, nil
}

func listResource(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	switch state.TypeName {
	case "AWS::S3::Bucket":
		state.View, state.Operation, state.TypeName = viewS3Objects, "s3-buckets", ""
		return listS3Objects(config, state)
	default:
	}
	out, err := runReadOnlyAWS(config, resourceGetArgs(state.Profile, state.Region, state.TypeName, state.Identifier))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var response struct {
		ResourceDescription struct {
			Identifier string `json:"Identifier"`
			Properties string `json:"Properties"`
		} `json:"ResourceDescription"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse AWS Cloud Control resource detail for type %q identifier %q: got %d bytes with error: %v", state.TypeName, state.Identifier, len(out), err)
	case response.ResourceDescription.Identifier != state.Identifier:
		return nil, nil, fmt.Errorf("AWS Cloud Control resource detail mismatch for type %q: requested identifier %q, got %q", state.TypeName, state.Identifier, response.ResourceDescription.Identifier)
	default:
	}
	properties := []byte(response.ResourceDescription.Properties)
	switch {
	case !json.Valid(properties):
		return nil, nil, fmt.Errorf("cannot parse AWS Cloud Control properties for type %q identifier %q: expected valid JSON, got %q", state.TypeName, state.Identifier, response.ResourceDescription.Properties)
	default:
	}
	actionsState, actionsErr := encodeMountState(mountState{View: viewResourceActions, Profile: state.Profile, Region: state.Region, TypeName: state.TypeName, Identifier: state.Identifier})
	switch {
	case actionsErr != nil:
		return nil, nil, actionsErr
	default:
	}
	resources := []driverlib.Resource{
		{Name: resourceActionsEntryName, Kind: "aws", Size: 0},
		{Name: "identifier: " + state.Identifier, Kind: "kv", Size: int64(len(state.Identifier))},
		{Name: "type: " + state.TypeName, Kind: "kv", Size: int64(len(state.TypeName))},
		{Name: "region: " + state.Region, Kind: "kv", Size: int64(len(state.Region))},
		{Name: propertiesEntryName, Kind: "json", Size: int64(len(properties))},
	}
	children := map[string][]byte{resourceActionsEntryName: actionsState, propertiesEntryName: properties}
	return resources, children, nil
}

func listResourceActions(state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	target, err := encodeActionTarget(actionTarget{Profile: state.Profile, Region: state.Region, TypeName: state.TypeName, Identifier: state.Identifier})
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	resources := []driverlib.Resource{
		{Name: "Update resource", Kind: actionUpdate, Size: int64(len(target))},
		{Name: "Delete resource", Kind: actionDelete, Size: int64(len(target))},
	}
	children := map[string][]byte{"Update resource": target, "Delete resource": target}
	return resources, children, nil
}

func listConfigActions(state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	regionTarget, regionErr := encodeActionTarget(actionTarget{Profile: state.Profile, Setting: "region"})
	outputTarget, outputErr := encodeActionTarget(actionTarget{Profile: state.Profile, Setting: "output"})
	switch {
	case regionErr != nil:
		return nil, nil, regionErr
	case outputErr != nil:
		return nil, nil, outputErr
	default:
	}
	resources := []driverlib.Resource{
		{Name: "Set default region", Kind: actionSetRegion, Size: int64(len(regionTarget))},
		{Name: "Set output format", Kind: actionSetOutput, Size: int64(len(outputTarget))},
	}
	children := map[string][]byte{"Set default region": regionTarget, "Set output format": outputTarget}
	return resources, children, nil
}
