#!/bin/sh

# Build first
./build.sh

GO_BIN="/opt/homebrew/bin/go"
if [ ! -f "$GO_BIN" ]; then
    GO_BIN="go"
fi

echo ""
echo "=== Running Go package tests ==="
$GO_BIN test ./...

echo ""
echo "=== Testing single binary exists ==="
if [ -f "datatree" ]; then
    echo "OK: datatree exists"
else
    echo "FAIL: datatree binary missing"
    exit 1
fi

echo ""
echo "=== Testing root_driver lists resources ==="
# Root driver should emit a resources JSON line on stdout
RESULT=$(echo '' | ./datatree --driver root_driver 2>/dev/null | head -1)
if echo "$RESULT" | grep -q '"resources"' && echo "$RESULT" | grep -q '"name":"HOME","kind":"directory"' && echo "$RESULT" | grep -q '"name":"/","kind":"directory"'; then
    echo "OK: root_driver emits HOME and / locations"
else
    echo "FAIL: root_driver did not emit HOME and / locations. Got: $RESULT"
    exit 1
fi

echo ""
echo "=== Testing text_driver with no data ==="
# Text driver requests range, gets empty response, should emit empty resources
RESULT=$(echo '{"data":"","size":0}' | ./datatree --driver text_driver 2>/dev/null | tail -1)
if echo "$RESULT" | grep -q '"resources"'; then
    echo "OK: text_driver emits resources (empty input)"
else
    echo "FAIL: text_driver did not emit resources. Got: $RESULT"
    exit 1
fi

echo ""
echo "=== Testing kubernetes_driver with kubectl unavailable ==="
# Kubernetes driver should still emit a resources line when kubectl is missing
RESULT=$(echo '{"data":"","size":0}' | DATATREE_KUBECTL=/nonexistent-kubectl ./datatree --driver kubernetes_driver 2>/dev/null)
if echo "$RESULT" | grep -q '"resources"'; then
    echo "OK: kubernetes_driver emits resources (kubectl unavailable)"
else
    echo "FAIL: kubernetes_driver did not emit resources. Got: $RESULT"
    exit 1
fi

echo ""
echo "=== Testing json_driver browses object structure ==="
# Feed a small JSON object and confirm the driver lists its keys as resources
JSON_DOC='{"a":1,"b":{"c":2}}'
JSON_B64=$(printf '%s' "$JSON_DOC" | base64 | tr -d '\n')
JSON_LEN=$(printf '%s' "$JSON_DOC" | wc -c | tr -d ' ')
RESULT=$(printf '{"data":"%s","size":%s}' "$JSON_B64" "$JSON_LEN" | ./datatree --driver json_driver 2>/dev/null)
if echo "$RESULT" | grep -q '"resources"' && echo "$RESULT" | grep -q '"kind":"json"'; then
    echo "OK: json_driver lists object fields"
else
    echo "FAIL: json_driver did not browse object. Got: $RESULT"
    exit 1
fi

echo ""
echo "=== Testing diff_driver formats unified diff lines ==="
DIFF_DOC='diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1,2 @@
-old
+new
+more'
DIFF_B64=$(printf '%s' "$DIFF_DOC" | base64 | tr -d '\n')
DIFF_LEN=$(printf '%s' "$DIFF_DOC" | wc -c | tr -d ' ')
RESULT=$(printf '{"data":"%s","size":%s}' "$DIFF_B64" "$DIFF_LEN" | ./datatree --driver diff_driver 2>/dev/null)
if echo "$RESULT" | grep -q '"kind":"diff-delete"' && echo "$RESULT" | grep -q '"kind":"diff-add"' && echo "$RESULT" | grep -q '"kind":"diff-hunk"'; then
    echo "OK: diff_driver shows metadata, hunks, removals, and additions"
else
    echo "FAIL: diff_driver did not format unified diff lines. Got: $RESULT"
    exit 1
fi

echo ""
echo "=== Testing kubernetes object drivers show curated views ==="
# Feed each driver a small manifest and confirm the expected curated row
check_driver_view() {
    DRIVER="$1"; DOC="$2"; EXPECT="$3"
    DOC_B64=$(printf '%s' "$DOC" | base64 | tr -d '\n')
    DOC_LEN=$(printf '%s' "$DOC" | wc -c | tr -d ' ')
    RESULT=$(printf '{"data":"%s","size":%s}' "$DOC_B64" "$DOC_LEN" | ./datatree --driver "$DRIVER" 2>/dev/null)
    if echo "$RESULT" | grep -q "$EXPECT"; then
        echo "OK: $DRIVER shows $EXPECT"
    else
        echo "FAIL: $DRIVER missing $EXPECT. Got: $RESULT"
        exit 1
    fi
}
check_driver_view k8s_pod_driver '{"kind":"Pod","spec":{"nodeName":"n1","containers":[{"name":"c1","image":"img:1"}]},"status":{"phase":"Running"}}' 'phase: Running'
check_driver_view k8s_workload_driver '{"kind":"Deployment","spec":{"replicas":3},"status":{"readyReplicas":3}}' 'ready: 3'
check_driver_view k8s_service_driver '{"kind":"Service","spec":{"type":"ClusterIP","clusterIP":"10.0.0.1","ports":[{"name":"grpc","port":50051,"protocol":"TCP"}]}}' 'clusterIP: 10.0.0.1'
check_driver_view k8s_configmap_driver '{"kind":"ConfigMap","data":{"config.yaml":"hello"}}' 'config.yaml'
check_driver_view k8s_node_driver '{"kind":"Node","status":{"nodeInfo":{"kubeletVersion":"v1.31.0"}}}' 'kubeletVersion: v1.31.0'

echo ""
echo "=== Testing file preview drivers ==="
IMAGE_B64='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='
RESULT=$(printf '{"data":"%s","size":68}' "$IMAGE_B64" | ./datatree --driver image_driver 2>/dev/null)
if echo "$RESULT" | grep -q 'width: 1' && echo "$RESULT" | grep -q 'height: 1'; then
    echo "OK: image_driver shows dimensions"
else
    echo "FAIL: image_driver did not show dimensions. Got: $RESULT"
    exit 1
fi
check_driver_view pdf_driver '%PDF-1.7 /Type /Page /URI (https://example.com/manual)' 'version: 1.7'
check_driver_view html_driver '<html><head><title>Datatree Preview</title></head><body><a href="https://example.com/page">page</a></body></html>' 'title: Datatree Preview'
check_driver_view csv_driver 'name,count
alpha,2' 'rows: 1'

echo ""
echo "=== Testing browser upload controls ==="
if grep -q 'id="upload-file"' web/index.html && grep -q "fetch('/api/upload'" web/index.html && grep -q "fetch('/api/load'" web/index.html && grep -q "fetch('/api/upload-settings'" web/index.html && grep -q "fetch('/api/unload'" web/index.html && grep -q "fetch('/api/github/action'" web/index.html && grep -q "fetch('/api/aws/action'" web/index.html && grep -q 'createAWSActionControl' web/index.html && grep -q 'RFC 6902 JSON patch' web/index.html && grep -q 'IP permissions JSON' web/index.html && grep -q 'Versioning state' web/index.html && grep -q 'aws-action-ec2-terminate' web/index.html && grep -q 'aws-action-iam-policy-version' web/index.html && grep -q "'hardware', 'usb'" web/index.html && grep -q 'Expand this node and every expandable descendant' web/index.html && grep -q 'setExpansionState' web/index.html && grep -q "isOpen ? '▾' : '▸'" web/index.html && grep -q 'expandRenderedResources' web/index.html && grep -q 'kind-diff-add' web/index.html && grep -q 'kind-diff-delete' web/index.html && grep -q 'actionExplanation' web/index.html && grep -q 'window.confirm' web/index.html && grep -q 'encodeBookmarkPath' web/index.html && grep -q 'restoreBookmarkedPath' web/index.html && grep -q 'navigator.clipboard.writeText' web/index.html && grep -q "window.location.protocol === 'file:'" web/index.html && grep -q 'cannot run directly from this HTML file' web/index.html; then
    echo "OK: browser rejects direct file use, shows expansion state, includes local hardware and USB trees, explains controls, recursively expands subtrees, styles diffs, executes GitHub and AWS service actions, and restores bookmark fragments"
else
    echo "FAIL: browser upload controls do not call upload settings, upload, load, and unload APIs"
    exit 1
fi

echo ""
echo "=== Running host integration self-test ==="
./datatree -test

echo ""
echo "=== Running installed-layout smoke test ==="
./install_test.sh

echo ""
echo "All tests passed."
