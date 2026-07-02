package drift

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		tag        string
		wantOK     bool
		wantPrefix string
		wantFamily string
	}{
		{"v1.2.3", true, "v", ""},
		{"1.2.3", true, "", ""},
		{"1.2", true, "", ""},
		{"1.2.3-alpine3.19", true, "", "alpine"},
		{"v2.0.0-rc1", true, "v", "rc"},
		{"1.5.0-beta.2", true, "", "beta"},
		{"sha-abc123", false, "", ""},
		{"20240115", false, "", ""},
		{"latest", false, "", ""},
		{"edge", false, "", ""},
		{"stable", false, "", ""},
		{"", false, "", ""},
		{"   ", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			_, ch, ok := Parse(tt.tag)
			if ok != tt.wantOK {
				t.Fatalf("Parse(%q) ok = %v, want %v", tt.tag, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if ch.Prefix != tt.wantPrefix || ch.Family != tt.wantFamily {
				t.Errorf("Parse(%q) channel = %+v, want prefix=%q family=%q",
					tt.tag, ch, tt.wantPrefix, tt.wantFamily)
			}
		})
	}
}

func TestFamily(t *testing.T) {
	tests := map[string]string{
		"":           "",
		"alpine3.19": "alpine",
		"rc1":        "rc",
		"beta.1":     "beta",
		"rc-1":       "rc",
		"alpine":     "alpine",
	}
	for pre, want := range tests {
		if got := family(pre); got != want {
			t.Errorf("family(%q) = %q, want %q", pre, got, want)
		}
	}
}

func TestCompute(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		latest    string
		wantClass Class
		wantScore float64
	}{
		{"equal is current", "1.2.3", "1.2.3", Current, 0},
		{"latest older is current", "1.4.0", "1.2.3", Current, 0},
		{"patch drift", "1.2.3", "1.2.5", Patch, 2},
		{"minor drift", "1.2.3", "1.5.0", Minor, 300 - 3},
		{"major drift", "1.2.3", "3.0.0", Major, 2*10000 - 200 - 3},
		{"v prefix patch", "v1.2.3", "v1.2.4", Patch, 1},
		{"alpine channel minor", "1.2.3-alpine3.19", "1.4.0-alpine3.19", Minor, 200 - 3},
		{"non-semver installed unknown", "sha-abc123", "1.2.3", Unknown, 0},
		{"non-semver latest unknown", "1.2.3", "20240115", Unknown, 0},
		{"latest tag unknown", "1.2.3", "latest", Unknown, 0},
		{"date installed unknown", "20240115", "20240201", Unknown, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, score := Compute(tt.installed, tt.latest)
			if class != tt.wantClass {
				t.Errorf("Compute(%q,%q) class = %q, want %q", tt.installed, tt.latest, class, tt.wantClass)
			}
			if score != tt.wantScore {
				t.Errorf("Compute(%q,%q) score = %v, want %v", tt.installed, tt.latest, score, tt.wantScore)
			}
		})
	}
}

func TestSelectLatest(t *testing.T) {
	tests := []struct {
		name       string
		installed  string
		available  []string
		wantLatest string
		wantCands  []string
		wantBehind int
	}{
		{
			name:       "plain semver picks newest",
			installed:  "1.2.3",
			available:  []string{"1.2.3", "1.2.4", "1.3.0", "1.2.2"},
			wantLatest: "1.3.0",
			wantCands:  []string{"1.3.0", "1.2.4", "1.2.3", "1.2.2"},
			wantBehind: 2,
		},
		{
			name:       "same channel only (alpine)",
			installed:  "1.2.3-alpine3.19",
			available:  []string{"1.2.3-alpine3.19", "1.2.4-alpine3.19", "1.3.0", "1.4.0-debian"},
			wantLatest: "1.2.4-alpine3.19",
			wantCands:  []string{"1.2.4-alpine3.19", "1.2.3-alpine3.19"},
			wantBehind: 1,
		},
		{
			name:       "v prefix channel isolation",
			installed:  "v1.2.3",
			available:  []string{"v1.2.4", "1.9.9", "v1.3.0"},
			wantLatest: "v1.3.0",
			wantCands:  []string{"v1.3.0", "v1.2.4"},
			wantBehind: 2,
		},
		{
			name:       "noise tags ignored but real ones kept",
			installed:  "1.2.3",
			available:  []string{"latest", "edge", "sha-deadbeef", "20240115", "1.2.4"},
			wantLatest: "1.2.4",
			wantCands:  []string{"1.2.4"},
			wantBehind: 1,
		},
		{
			name:       "installed absent from registry",
			installed:  "1.2.3",
			available:  []string{"1.2.5", "1.2.6"},
			wantLatest: "1.2.6",
			wantCands:  []string{"1.2.6", "1.2.5"},
			wantBehind: 2,
		},
		{
			name:       "already newest",
			installed:  "2.0.0",
			available:  []string{"1.9.0", "2.0.0"},
			wantLatest: "2.0.0",
			wantCands:  []string{"2.0.0", "1.9.0"},
			wantBehind: 0,
		},
		{
			name:       "non-semver installed not comparable",
			installed:  "sha-abc123",
			available:  []string{"1.2.3", "1.2.4"},
			wantLatest: "",
			wantCands:  nil,
			wantBehind: -1,
		},
		{
			name:       "no in-channel candidates",
			installed:  "1.2.3-alpine3.19",
			available:  []string{"1.2.4", "latest"},
			wantLatest: "",
			wantCands:  nil,
			wantBehind: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectLatest(tt.installed, tt.available)
			if got.Latest != tt.wantLatest {
				t.Errorf("Latest = %q, want %q", got.Latest, tt.wantLatest)
			}
			if got.ReleasesBehind != tt.wantBehind {
				t.Errorf("ReleasesBehind = %d, want %d", got.ReleasesBehind, tt.wantBehind)
			}
			if !reflect.DeepEqual(got.Candidates, tt.wantCands) {
				t.Errorf("Candidates = %v, want %v", got.Candidates, tt.wantCands)
			}
		})
	}
}
