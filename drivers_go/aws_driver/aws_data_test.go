package awsdriver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"datahumper/drivers_go/driverlib"
)

func fakeDataAWS(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aws")
	script := `#!/bin/sh
case "$1 $2" in
 "s3api list-objects-v2")
  case "$7" in
   --prefix=docs/) printf '%s' '{"Contents":[{"Key":"docs/report.json","Size":20}]}' ;;
   --prefix=denied/) echo 'AccessDenied: bucket listing denied' >&2; exit 1 ;;
   --prefix=invalid/) printf '%s' '{"Contents":[{"Key":"invalid/a"}]}' ;;
   *)
    case "${10}" in
     --starting-token=page-two) printf '%s' '{"Contents":[{"Key":"last.txt","Size":1}]}' ;;
     *) printf '%s' '{"CommonPrefixes":[{"Prefix":"docs/"}],"Contents":[{"Key":"[actions]","Size":0},{"Key":"plain.txt","Size":5}],"NextToken":"page-two"}' ;;
    esac ;;
  esac ;;
 "s3 cp")
  case "$3" in
   s3://example-bucket/docs/report.json) printf '%s' '{"answer":42}' ;;
   s3://example-bucket/plain.txt) printf 'hello' ;;
   s3://example-bucket/empty.txt) printf '' ;;
   s3://example-bucket/large.txt) printf '0123456789' ;;
   *) echo 'AccessDenied: object read denied' >&2; exit 1 ;;
  esac ;;
 "dynamodb scan") printf '%s' '{"Items":[{"message":{"S":"actual table data"}}]}' ;;
 "iam list-policy-versions") printf '%s' '{"Versions":[{"VersionId":"v1"}]}' ;;
 "iam get-policy-version") printf '%s' '{"PolicyVersion":{"Document":{"Statement":[{"Action":"s3:GetObject","Effect":"Allow","Resource":"*"}]}}}' ;;
 "logs describe-log-streams") printf '%s' '{"logStreams":[{"logStreamName":"application"}]}' ;;
 "logs get-log-events") printf '%s' '{"events":[{"message":"actual log line","timestamp":1}],"nextForwardToken":"forward-page"}' ;;
 *) echo "unexpected invocation: $*" >&2; exit 97 ;;
esac
`
	switch err := os.WriteFile(path, []byte(script), 0700); {
	case err != nil:
		t.Fatal(err)
	default:
		t.Setenv("DATATREE_AWS", path)
	}
}

func dataChild(t *testing.T, resources []driverlib.Resource, children map[string][]byte, name string) []byte {
	t.Helper()
	for _, resource := range resources {
		switch {
		case resource.Name == name:
			key := resource.Key
			switch {
			case key == "":
				key = resource.Name
			default:
			}
			return children[key]
		default:
		}
	}
	t.Fatalf("missing child %q in %+v", name, resources)
	return nil
}

func TestS3BucketPrefixesObjectsAndReadableBytes(t *testing.T) {
	fakeDataAWS(t)
	state := mountState{View: viewServiceResource, Profile: "default", Region: "us-east-1", Operation: "s3-buckets", Identifier: "example-bucket"}
	raw, _ := encodeMountState(state)
	resources, children := mustBuildAWSView(t, raw)
	prefix := dataChild(t, resources, children, "docs/")
	objects, objectChildren := mustBuildAWSView(t, prefix)
	object := dataChild(t, objects, objectChildren, "report.json")
	content := objects
	bytes, readErr := readAWSChild(objects[0].Key, object, 0, -1)
	switch {
	case readErr != nil:
		t.Fatal(readErr)
	default:
	}
	switch {
	case string(bytes) != `{"answer":42}` || content[0].Kind != "json":
		t.Fatalf("expected actual JSON object bytes, got %q with %+v", bytes, content)
	default:
	}
	dataChild(t, resources, children, "[actions]")
	dataChild(t, resources, children, resourceActionsEntryName)
	var page mountState
	switch err := json.Unmarshal(dataChild(t, resources, children, "[next page]"), &page); {
	case err != nil:
		t.Fatal(err)
	case page.Token != "page-two":
		t.Fatalf("missing continuation: %+v", page)
	default:
	}
	last, lastChildren := mustBuildAWSView(t, dataChild(t, resources, children, "[next page]"))
	dataChild(t, last, lastChildren, "last.txt")
	for _, test := range []struct{ key, expected, kind string }{{"plain.txt", "hello", "text"}, {"empty.txt", "", "text"}} {
		state.View, state.Key = viewS3Object, test.key
		raw, _ := encodeMountState(state)
		data, err := readAWSChild("s3-bytes:test", raw, 0, -1)
		switch {
		case err != nil || string(data) != test.expected:
			t.Fatalf("wrong content for %q: %q %v", test.key, data, err)
		default:
		}
	}
}

func TestS3ErrorsAreVisibleAndOutputIsBounded(t *testing.T) {
	fakeDataAWS(t)
	for _, test := range []struct{ view, key, expected string }{{viewS3Objects, "denied/", "AccessDenied"}, {viewS3Object, "denied.txt", "AccessDenied"}, {viewS3Objects, "invalid/", "non-negative size"}} {
		raw, _ := encodeMountState(mountState{View: test.view, Profile: "default", Region: "us-east-1", Operation: "s3-buckets", Identifier: "example-bucket", Key: test.key})
		var err error
		switch test.view {
		case viewS3Object:
			_, err = readAWSChild("s3-bytes:test", raw, 0, -1)
		default:
			_, _, err = buildAWSView(raw)
		}
		switch {
		case err == nil || !strings.Contains(err.Error(), test.expected):
			t.Fatalf("expected %s for %s, got %v", test.expected, test.key, err)
		default:
		}
	}
	t.Setenv("DATATREE_AWS_MAX_OUTPUT_BYTES", "4")
	raw, _ := encodeMountState(mountState{View: viewS3Object, Profile: "default", Region: "us-east-1", Operation: "s3-buckets", Identifier: "example-bucket", Key: "large.txt"})
	data, err := readAWSChild("s3-bytes:test", raw, 0, -1)
	switch {
	case err != nil || string(data) != "0123456789":
		t.Fatalf("metadata limit must not cut off file bytes, got %q %v", data, err)
	default:
	}
	for _, test := range []struct {
		offset, length int64
		expected       string
	}{{2, 4, "2345"}, {8, -1, "89"}, {50, 4, ""}, {-1, 4, ""}, {0, 0, ""}} {
		part, err := readAWSChild("s3-bytes:test", raw, test.offset, test.length)
		switch {
		case err != nil || string(part) != test.expected:
			t.Fatalf("S3 range offset=%d length=%d: expected %q, got %q: %v", test.offset, test.length, test.expected, part, err)
		default:
		}
	}
}

func TestDataQueriesReachTableItemsAndLogMessages(t *testing.T) {
	fakeDataAWS(t)
	state := mountState{Profile: "default", Region: "us-east-1", Operation: "dynamodb-tables"}
	resources, children, err := listAWSDataResources([]byte(`{"TableNames":["messages"]}`), state)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	items, itemChildren := mustBuildAWSView(t, dataChild(t, resources, children, "messages [Items]"))
	switch body := string(dataChild(t, items, itemChildren, "[response]")); {
	case !strings.Contains(body, "actual table data"):
		t.Fatalf("expected stored data, got %s", body)
	default:
	}
	state.Operation = "log-groups"
	resources, children, err = listAWSDataResources([]byte(`{"logGroups":[{"logGroupName":"/app"}]}`), state)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	streams, streamChildren := mustBuildAWSView(t, dataChild(t, resources, children, "/app [Streams]"))
	events, eventChildren := mustBuildAWSView(t, dataChild(t, streams, streamChildren, "application [Events]"))
	lastEvents, _ := mustBuildAWSView(t, dataChild(t, events, eventChildren, "[next page]"))
	switch {
	case len(lastEvents) != 1:
		t.Fatalf("stable forward token must terminate pagination, got %+v", lastEvents)
	default:
	}
	switch body := string(dataChild(t, events, eventChildren, "[response]")); {
	case !strings.Contains(body, "actual log line"):
		t.Fatalf("expected log data, got %s", body)
	default:
	}
}

func TestDataCommandGatesAndStateValidation(t *testing.T) {
	for _, edge := range dataEdges {
		state := mountState{View: viewData, Profile: "default", Region: "us-east-1", Operation: edge.id, Identifier: "resource with spaces", Key: "parent"}
		switch {
		case !validDataReadInvocation(dataArgs(edge, state)):
			t.Fatalf("gate rejected compiled command %s", edge.id)
		default:
		}
	}
	for _, command := range [][]string{{"s3", "cp", "local.txt", "s3://bucket/key", "--only-show-errors"}, {"s3", "cp", "s3://bucket/key", "/tmp/file", "--only-show-errors"}, {"s3api", "delete-object", "--bucket", "bucket", "--key", "key"}, {"dynamodb", "delete-table", "--table-name=x"}, {"sqs", "receive-message", "--queue-url=x"}} {
		args := awsRemoteArgs(command, "default", "us-east-1")
		switch {
		case validS3ReadInvocation(args) || validDataReadInvocation(args):
			t.Fatalf("accepted unsafe command %v", args)
		default:
		}
	}
	for _, raw := range []string{`{"view":"s3-object","profile":"default","region":"us-east-1","operation":"s3-buckets","identifier":"bucket"}`, `{"view":"data","profile":"default","region":"us-east-1","operation":"delete-table","identifier":"table"}`, `{"view":"root","key":"unexpected"}`} {
		switch _, err := parseMountState([]byte(raw)); {
		case err == nil:
			t.Fatalf("accepted invalid state %s", raw)
		default:
		}
	}
}

func TestPolicyDocumentsAndCloudControlBucketsReachContents(t *testing.T) {
	fakeDataAWS(t)
	state := mountState{Profile: "default", Region: "us-east-1", Operation: "iam-policies", Identifier: "arn:aws:iam::123456789012:policy/example"}
	resources, children, err := listAWSDataResources([]byte(`{"Policy":{"PolicyName":"example"}}`), state)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	versions, versionChildren := mustBuildAWSView(t, dataChild(t, resources, children, state.Identifier+" [Policy versions]"))
	document, documentChildren := mustBuildAWSView(t, dataChild(t, versions, versionChildren, "v1 [Policy document]"))
	switch body := string(dataChild(t, document, documentChildren, "[response]")); {
	case !strings.Contains(body, "s3:GetObject"):
		t.Fatalf("expected actual policy document, got %s", body)
	default:
	}
	raw, _ := encodeMountState(mountState{View: viewResource, Profile: "default", Region: "us-east-1", TypeName: "AWS::S3::Bucket", Identifier: "example-bucket"})
	objects, objectChildren := mustBuildAWSView(t, raw)
	dataChild(t, objects, objectChildren, "docs/")
}
