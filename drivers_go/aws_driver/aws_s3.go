package awsdriver

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"datahumper/drivers_go/driverlib"
)

const viewS3Objects = "s3-objects"
const viewS3Object = "s3-object"

func validS3Bucket(bucket string) bool {
	return len(bucket) >= 3 && len(bucket) <= 255 && !strings.ContainsAny(bucket, "/\\\x00\r\n :") && !strings.HasPrefix(bucket, "-")
}

func validS3Key(key string) bool {
	return utf8.ValidString(key) && len(key) <= 1024 && !strings.ContainsRune(key, 0)
}

func validPageToken(token string) bool {
	return len(token) <= 8192 && !strings.ContainsAny(token, "\x00\r\n")
}

func validateS3State(state mountState) error {
	switch {
	case !validProfile(state.Profile) || !validRegion(state.Region) || !validS3Bucket(state.Identifier) || state.TypeName != "" || state.Operation != "s3-buckets" || !validS3Key(state.Key) || !validPageToken(state.Token):
		return fmt.Errorf("invalid S3 browsing state: expected profile, region, bucket, key or prefix and pagination token")
	case state.View == viewS3Object && (state.Key == "" || state.Token != ""):
		return fmt.Errorf("invalid S3 object state: expected a non-empty key and no pagination token")
	default:
		return nil
	}
}

func s3ListArgs(state mountState) []string {
	command := []string{"s3api", "list-objects-v2", "--bucket", state.Identifier, "--delimiter", "/", "--prefix=" + state.Key, "--max-items", "1000"}
	switch {
	case state.Token != "":
		command = append(command, "--starting-token="+state.Token)
	default:
	}
	return awsRemoteArgs(command, state.Profile, state.Region)
}

func s3ReadArgs(state mountState) []string {
	return awsRemoteArgs([]string{"s3", "cp", "s3://" + state.Identifier + "/" + state.Key, "-", "--only-show-errors"}, state.Profile, state.Region)
}

func validS3ReadInvocation(args []string) bool {
	profile, region, ok := validateAWSTail(args)
	switch {
	case !ok || !validProfile(profile) || !validRegion(region):
		return false
	default:
	}
	prefix := args[:len(args)-7]
	switch {
	case len(prefix) == 5 && prefix[0] == "s3" && prefix[1] == "cp" && prefix[3] == "-" && prefix[4] == "--only-show-errors" && strings.HasPrefix(prefix[2], "s3://"):
		bucket, key, found := strings.Cut(strings.TrimPrefix(prefix[2], "s3://"), "/")
		return found && validS3Bucket(bucket) && key != "" && validS3Key(key)
	case (len(prefix) == 9 || len(prefix) == 10) && prefix[0] == "s3api" && prefix[1] == "list-objects-v2" && prefix[2] == "--bucket" && validS3Bucket(prefix[3]) && prefix[4] == "--delimiter" && prefix[5] == "/" && strings.HasPrefix(prefix[6], "--prefix=") && validS3Key(strings.TrimPrefix(prefix[6], "--prefix=")) && prefix[7] == "--max-items" && prefix[8] == "1000":
		return len(prefix) == 9 || (strings.HasPrefix(prefix[9], "--starting-token=") && validPageToken(strings.TrimPrefix(prefix[9], "--starting-token=")))
	default:
		return false
	}
}

func listS3Objects(config awsConfig, state mountState) ([]driverlib.Resource, map[string][]byte, error) {
	switch err := validateS3State(state); {
	case err != nil:
		return nil, nil, err
	default:
	}
	out, err := runReadOnlyAWS(config, s3ListArgs(state))
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var response struct {
		Contents []struct {
			Key  string
			Size *int64
		}
		CommonPrefixes []struct{ Prefix string }
		NextToken      string
	}
	switch err := json.Unmarshal(out, &response); {
	case err != nil || strings.TrimSpace(string(out)) == "null":
		return nil, nil, fmt.Errorf("cannot list S3 bucket %q prefix %q: expected object listing JSON, got %d bytes: %v", state.Identifier, state.Key, len(out), err)
	default:
	}
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	add := func(name string, size int64, next mountState) error {
		raw, err := encodeMountState(next)
		switch {
		case err != nil:
			return err
		default:
			key := fmt.Sprintf("entry-%d", len(resources))
			kind := "aws"
			switch next.View {
			case viewS3Object:
				key = "s3-bytes:" + key
				kind = driverlib.KindForName(next.Key)
			default:
			}
			resources = append(resources, driverlib.Resource{Name: name, Kind: kind, Size: size, Key: key})
			children[key] = raw
			return nil
		}
	}
	for _, prefix := range response.CommonPrefixes {
		switch {
		case !validS3Key(prefix.Prefix) || prefix.Prefix == state.Key || !strings.HasPrefix(prefix.Prefix, state.Key) || !strings.HasSuffix(prefix.Prefix, "/"):
			return nil, nil, fmt.Errorf("invalid S3 prefix %q below %q", prefix.Prefix, state.Key)
		default:
			next := state
			next.View, next.Key, next.Token = viewS3Objects, prefix.Prefix, ""
			switch err := add(strings.TrimPrefix(prefix.Prefix, state.Key), 0, next); {
			case err != nil:
				return nil, nil, err
			default:
			}
		}
	}
	for _, object := range response.Contents {
		switch {
		case object.Key == "" || !validS3Key(object.Key) || !strings.HasPrefix(object.Key, state.Key) || object.Size == nil || *object.Size < 0:
			return nil, nil, fmt.Errorf("invalid S3 object %q: expected key below %q and non-negative size", object.Key, state.Key)
		default:
			next := state
			next.View, next.Key, next.Token = viewS3Object, object.Key, ""
			switch err := add(strings.TrimPrefix(object.Key, state.Key), *object.Size, next); {
			case err != nil:
				return nil, nil, err
			default:
			}
		}
	}
	switch {
	case response.NextToken != "":
		switch {
		case !validPageToken(response.NextToken) || response.NextToken == state.Token:
			return nil, nil, fmt.Errorf("invalid or repeated S3 pagination token for bucket %q", state.Identifier)
		default:
			next := state
			next.View, next.Token = viewS3Objects, response.NextToken
			switch err := add("[next page]", 0, next); {
			case err != nil:
				return nil, nil, err
			default:
			}
		}
	default:
	}
	switch {
	case state.Key == "" && state.Token == "":
		actions := state
		actions.View = viewServiceActions
		raw, err := encodeMountState(actions)
		switch {
		case err != nil:
			return nil, nil, err
		default:
			resources = append(resources, driverlib.Resource{Name: resourceActionsEntryName, Kind: "aws"})
			children[resourceActionsEntryName] = raw
		}
	default:
	}
	return driverlib.RetainSource(resources, children, out)
}
