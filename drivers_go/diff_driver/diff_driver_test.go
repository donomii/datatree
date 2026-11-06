package diffdriver

import "testing"

func TestListDiffLines(t *testing.T) {
	raw := []byte("diff --git a/main.go b/main.go\nindex 1111111..2222222 100644\n--- a/main.go\n+++ b/main.go\n@@ -10,3 +10,4 @@ func main() {\n context\n-old line\n+new line\n+another line\n}")
	resources, children, err := listDiffLines(raw)
	switch {
	case err != nil:
		t.Fatalf("diff parse failed: %v", err)
	case len(children) != 0:
		t.Fatalf("diff line view unexpectedly served children: %+v", children)
	case len(resources) != 10:
		t.Fatalf("diff resource count mismatch: got %d expected 10: %+v", len(resources), resources)
	default:
	}
	expected := []struct {
		index int
		name  string
		kind  string
	}{
		{index: 4, name: "@@ -10,3 +10,4 @@ func main() {", kind: "diff-hunk"},
		{index: 5, name: "      10     10 | context", kind: "diff-context"},
		{index: 6, name: "-     11        | old line", kind: "diff-delete"},
		{index: 7, name: "+            11 | new line", kind: "diff-add"},
		{index: 8, name: "+            12 | another line", kind: "diff-add"},
	}
	for _, want := range expected {
		got := resources[want.index]
		switch {
		case got.Name != want.name || got.Kind != want.kind:
			t.Errorf("diff row %d mismatch: got %+v expected name %q kind %q", want.index, got, want.name, want.kind)
		default:
		}
	}
}

func TestListDiffLinesRejectsOverflowingLineNumber(t *testing.T) {
	raw := []byte("@@ -999999999999999999999999999999 +1 @@\n-old\n+new")
	_, _, err := listDiffLines(raw)
	switch {
	case err == nil:
		t.Fatal("overflowing unified-diff line number unexpectedly parsed without an error")
	default:
	}
}
