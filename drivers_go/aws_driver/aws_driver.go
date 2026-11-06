package awsdriver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"datahumper/drivers_go/driverlib"
)

type mountState struct {
	View       string `json:"view,omitempty"`
	Profile    string `json:"profile,omitempty"`
	Region     string `json:"region,omitempty"`
	TypeName   string `json:"typeName,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Operation  string `json:"operation,omitempty"`
	Key        string `json:"key,omitempty"`
	Token      string `json:"token,omitempty"`
}

type actionTarget struct {
	Profile    string `json:"profile"`
	Region     string `json:"region,omitempty"`
	TypeName   string `json:"typeName,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Setting    string `json:"setting,omitempty"`
	Operation  string `json:"operation,omitempty"`
}

const viewRoot = "root"
const viewProfile = "profile"
const viewRegions = "regions"
const viewRegion = "region"
const viewSearch = "search"
const viewTypes = "types"
const viewType = "type"
const viewResource = "resource"
const viewResourceActions = "resource-actions"
const viewConfigActions = "config-actions"
const viewServices = "services"
const viewServiceQuery = "service-query"
const viewServiceResource = "service-resource"
const viewServiceActions = "service-actions"
const maxAWSMountStateBytes = 16 * 1024

const regionsEntryName = "[regions and resources]"
const configActionsEntryName = "[configuration actions]"
const searchEntryName = "[Resource Explorer: all indexed resources]"
const typesEntryName = "[Cloud Control resource types]"
const resourceActionsEntryName = "[actions]"
const propertiesEntryName = "[properties]"
const servicesEntryName = "[service catalog]"

const actionCreate = "aws-action-create"
const actionUpdate = "aws-action-update"
const actionDelete = "aws-action-delete"
const actionSetRegion = "aws-action-set-region"
const actionSetOutput = "aws-action-set-output"
const actionEC2Start = "aws-action-ec2-start"
const actionEC2Stop = "aws-action-ec2-stop"
const actionEC2Reboot = "aws-action-ec2-reboot"
const actionEC2Terminate = "aws-action-ec2-terminate"
const actionSGAuthorize = "aws-action-sg-authorize-ingress"
const actionSGRevoke = "aws-action-sg-revoke-ingress"
const actionS3PutPolicy = "aws-action-s3-put-policy"
const actionS3DeletePolicy = "aws-action-s3-delete-policy"
const actionS3Versioning = "aws-action-s3-versioning"
const actionIAMTrust = "aws-action-iam-update-trust"
const actionIAMPolicyVersion = "aws-action-iam-policy-version"

func Run() {
	driverlib.RunWithReader("AWS driver", buildAWSView, readAWSChild)
}

func buildAWSView(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	state, err := parseMountState(raw)
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	config, configErr := resolveAWSConfig()
	switch {
	case configErr != nil:
		return nil, nil, configErr
	default:
	}
	switch state.View {
	case viewS3Objects:
		return listS3Objects(config, state)
	case viewData:
		return listAWSData(config, state)
	case viewRoot:
		return listProfiles(config)
	case viewProfile:
		return listProfile(config, state)
	case viewRegions:
		return listRegions(config, state)
	case viewRegion:
		return listRegion(state)
	case viewSearch:
		return listResourceSearch(config, state)
	case viewTypes:
		return listResourceTypes(config, state)
	case viewType:
		return listResources(config, state)
	case viewResource:
		return listResource(config, state)
	case viewResourceActions:
		return listResourceActions(state)
	case viewConfigActions:
		return listConfigActions(state)
	case viewServices:
		return listAWSServices(state)
	case viewServiceQuery:
		return listAWSServiceQuery(config, state)
	case viewServiceResource:
		return listAWSServiceResource(config, state)
	case viewServiceActions:
		return listAWSServiceActions(state)
	default:
		return nil, nil, fmt.Errorf("cannot browse AWS: unsupported view %q", state.View)
	}
}

func parseMountState(raw []byte) (mountState, error) {
	switch {
	case len(raw) > maxAWSMountStateBytes:
		return mountState{}, fmt.Errorf("cannot parse AWS mount state: received %d bytes, limit is %d bytes", len(raw), maxAWSMountStateBytes)
	default:
	}
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "":
		trimmed = "{}"
	default:
	}
	var state mountState
	switch err := json.Unmarshal([]byte(trimmed), &state); {
	case err != nil:
		return mountState{}, fmt.Errorf("cannot parse AWS mount state: expected a JSON object, got %q with error: %v", trimmed, err)
	default:
	}
	switch state.View {
	case "":
		state.View = viewRoot
	default:
	}
	switch err := validateMountState(state); {
	case err != nil:
		return mountState{}, err
	default:
		return state, nil
	}
}

func validateMountState(state mountState) error {
	switch state.View {
	case viewS3Objects, viewS3Object:
		return validateS3State(state)
	case viewData:
		return validateDataState(state)
	default:
		switch {
		case state.Key != "" || state.Token != "":
			return fmt.Errorf("invalid AWS %s state: unexpected key or pagination token", state.View)
		default:
		}
	}
	switch state.View {
	case viewRoot:
		switch {
		case state.Profile != "" || state.Region != "" || state.TypeName != "" || state.Identifier != "" || state.Operation != "":
			return fmt.Errorf("invalid AWS root state: expected no target fields")
		default:
			return nil
		}
	case viewProfile, viewRegions, viewConfigActions:
		switch {
		case !validProfile(state.Profile) || state.Region != "" || state.TypeName != "" || state.Identifier != "" || state.Operation != "":
			return fmt.Errorf("invalid AWS %s state: expected one valid profile only, got profile=%q region=%q type=%q identifier=%q", state.View, state.Profile, state.Region, state.TypeName, state.Identifier)
		default:
			return nil
		}
	case viewRegion, viewSearch, viewTypes, viewServices:
		switch {
		case !validProfile(state.Profile) || !validRegion(state.Region) || state.TypeName != "" || state.Identifier != "" || state.Operation != "":
			return fmt.Errorf("invalid AWS %s state: expected valid profile and region only", state.View)
		default:
			return nil
		}
	case viewType:
		switch {
		case !validProfile(state.Profile) || !validRegion(state.Region) || !validTypeName(state.TypeName) || state.Identifier != "":
			return fmt.Errorf("invalid AWS type state: expected valid profile, region and resource type")
		default:
			return nil
		}
	case viewResource, viewResourceActions:
		switch {
		case !validProfile(state.Profile) || !validRegion(state.Region) || !validTypeName(state.TypeName) || !validIdentifier(state.Identifier):
			return fmt.Errorf("invalid AWS resource state: expected valid profile, region, resource type and identifier")
		default:
			return nil
		}
	case viewServiceQuery:
		switch {
		case !validProfile(state.Profile) || !validRegion(state.Region) || !validServiceOperation(state.Operation) || state.Identifier != "" || state.TypeName != "":
			return fmt.Errorf("invalid AWS service query state: expected valid profile, region and allowlisted operation")
		default:
			return nil
		}
	case viewServiceResource, viewServiceActions:
		switch {
		case !validProfile(state.Profile) || !validRegion(state.Region) || !validDetailedOperation(state.Operation) || !validIdentifier(state.Identifier) || state.TypeName != "":
			return fmt.Errorf("invalid AWS service resource state: expected valid profile, region, detailed operation and identifier")
		default:
			return nil
		}
	default:
		return fmt.Errorf("invalid AWS view %q", state.View)
	}
}

func listProfiles(config awsConfig) ([]driverlib.Resource, map[string][]byte, error) {
	out, err := runReadOnlyAWS(config, profileListArgs())
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	profiles := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		profile := strings.TrimSpace(line)
		switch {
		case profile == "":
			continue
		default:
			profiles = append(profiles, profile)
		}
	}
	sort.Strings(profiles)
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	for _, profile := range profiles {
		switch {
		case !validProfile(profile):
			return nil, nil, fmt.Errorf("cannot list AWS profiles: profile %q has an unsupported name", profile)
		default:
		}
		state, stateErr := encodeMountState(mountState{View: viewProfile, Profile: profile})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: profile, Kind: "aws", Size: 0})
		children[profile] = state
	}
	switch {
	case len(resources) == 0:
		resources = append(resources, driverlib.Resource{Name: "AWS CLI has no configured profiles", Kind: "error", Size: 0})
	default:
	}
	return resources, children, nil
}

func listProfile(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	settings := []string{"region", "output", "role_arn", "source_profile", "credential_source", "sso_session", "sso_account_id", "sso_role_name"}
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	for _, setting := range settings {
		out, err := runReadOnlyAWS(config, profileSettingArgs(state.Profile, setting))
		switch {
		case err != nil:
			return nil, nil, err
		case strings.TrimSpace(string(out)) != "":
			resources = append(resources, driverlib.Resource{Name: setting + ": " + strings.TrimSpace(string(out)), Kind: "kv", Size: int64(len(strings.TrimSpace(string(out))))})
		default:
		}
	}
	configState, configErr := encodeMountState(mountState{View: viewConfigActions, Profile: state.Profile})
	regionsState, regionsErr := encodeMountState(mountState{View: viewRegions, Profile: state.Profile})
	switch {
	case configErr != nil:
		return nil, nil, configErr
	case regionsErr != nil:
		return nil, nil, regionsErr
	default:
	}
	resources = append(resources, driverlib.Resource{Name: configActionsEntryName, Kind: "aws", Size: 0}, driverlib.Resource{Name: regionsEntryName, Kind: "aws", Size: 0})
	children[configActionsEntryName] = configState
	children[regionsEntryName] = regionsState
	return resources, children, nil
}

func configuredRegion(config awsConfig, profile string) (string, error) {
	out, err := runReadOnlyAWS(config, profileSettingArgs(profile, "region"))
	switch {
	case err != nil:
		return "", err
	case validRegion(strings.TrimSpace(string(out))):
		return strings.TrimSpace(string(out)), nil
	default:
		return "us-east-1", nil
	}
}

func listRegions(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	bootstrapRegion, regionErr := configuredRegion(config, state.Profile)
	switch {
	case regionErr != nil:
		return nil, nil, regionErr
	default:
	}
	out, err := runReadOnlyAWS(config, regionListArgs(state.Profile, bootstrapRegion))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var response struct {
		Regions []struct {
			RegionName  string `json:"RegionName"`
			OptInStatus string `json:"OptInStatus"`
		} `json:"Regions"`
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse AWS region list for profile %q: expected Regions JSON, got %d bytes with error: %v", state.Profile, len(out), err)
	default:
	}
	sort.Slice(response.Regions, func(left int, right int) bool {
		return response.Regions[left].RegionName < response.Regions[right].RegionName
	})
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	for _, region := range response.Regions {
		switch {
		case !validRegion(region.RegionName):
			return nil, nil, fmt.Errorf("cannot list AWS regions: received invalid region name %q", region.RegionName)
		default:
		}
		name := region.RegionName + " (" + region.OptInStatus + ")"
		regionState, stateErr := encodeMountState(mountState{View: viewRegion, Profile: state.Profile, Region: region.RegionName})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: name, Kind: "aws", Size: 0})
		children[name] = regionState
	}
	return resources, children, nil
}

func listRegion(state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	searchState, searchErr := encodeMountState(mountState{View: viewSearch, Profile: state.Profile, Region: state.Region})
	typesState, typesErr := encodeMountState(mountState{View: viewTypes, Profile: state.Profile, Region: state.Region})
	servicesState, servicesErr := encodeMountState(mountState{View: viewServices, Profile: state.Profile, Region: state.Region})
	switch {
	case searchErr != nil:
		return nil, nil, searchErr
	case typesErr != nil:
		return nil, nil, typesErr
	case servicesErr != nil:
		return nil, nil, servicesErr
	default:
	}
	resources := []driverlib.Resource{{Name: servicesEntryName, Kind: "aws", Size: 0}, {Name: searchEntryName, Kind: "aws", Size: 0}, {Name: typesEntryName, Kind: "aws", Size: 0}}
	children := map[string][]byte{servicesEntryName: servicesState, searchEntryName: searchState, typesEntryName: typesState}
	return resources, children, nil
}

func encodeMountState(state mountState) ([]byte, error) {
	out, err := json.Marshal(state)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot encode AWS mount state for view %q: %v", state.View, err)
	default:
		return out, nil
	}
}

func encodeActionTarget(target actionTarget) ([]byte, error) {
	out, err := json.Marshal(target)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot encode AWS action target for profile %q region %q type %q: %v", target.Profile, target.Region, target.TypeName, err)
	default:
		return out, nil
	}
}
