package main

import (
	"testing"
)

func TestMatchesSearchTokens(t *testing.T) {
	if !matchesSearchTokens("hidden-treasure.json", "json", []string{"treasure", "json"}) {
		t.Fatalf("expected multi-token match across name and kind")
	} else if matchesSearchTokens("hidden-treasure.json", "json", []string{"treasure", "csv"}) {
		t.Fatalf("unexpected match when second token is absent")
	} else if matchesSearchTokens("anything", "text", nil) {
		t.Fatalf("unexpected match for empty token list")
	} else {
		// Token matching verified.
	}
}

func TestSearchableContainerKindsExcludeExternalNetworkDrivers(t *testing.T) {
	for _, excluded := range []string{"aws", "github", "kubernetes", "hardware", "usb", "url", "raw"} {
		if isSearchableContainerKind(excluded) {
			t.Fatalf("kind %q must not be expanded automatically by search", excluded)
		} else {
			// External/raw kind excluded.
		}
	}
	for _, included := range []string{"archive", "zip", "json", "yaml", "csv", "tsv", "xml"} {
		if !isSearchableContainerKind(included) {
			t.Fatalf("kind %q should be searchable by speedsearch", included)
		} else {
			// Safe container kind included.
		}
	}
}
