package api

import (
	"strings"
	"testing"
)

func TestCsvSafe(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"normal", "normal"},
		{"=SUM(A1)", "'=SUM(A1)"},
		{"+1234", "'+1234"},
		{"-formula", "'-formula"},
		{"@evil", "'@evil"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := csvSafe(tt.in); got != tt.want {
			t.Errorf("csvSafe(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSortArtifactDTOs(t *testing.T) {
	items := []artifactDTO{
		{ID: "b", Identity: "z", DriftScore: 0.5},
		{ID: "a", Identity: "a", DriftScore: 0.8},
		{ID: "c", Identity: "m", DriftScore: 0.8},
	}
	sortArtifactDTOs(items, "drift_score_desc")
	if items[0].ID != "a" || items[1].ID != "c" {
		t.Errorf("drift_score_desc order: %+v", items)
	}
	sortArtifactDTOs(items, "identity_asc")
	if items[0].Identity != "a" {
		t.Errorf("identity_asc order: %+v", items)
	}
}

func TestParseArtifactFilters(t *testing.T) {
	_, err := parseArtifactFilters(map[string][]string{"sort": {"bogus"}})
	if err == nil || !strings.Contains(err.Error(), "sort") {
		t.Fatalf("expected sort error, got %v", err)
	}
}
