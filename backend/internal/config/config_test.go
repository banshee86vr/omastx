package config

import (
	"strings"
	"testing"
)

func TestParseMasterKey(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantLen int
		wantErr string
	}{
		{"empty", "", 0, "required"},
		{"64 hex chars decodes to 32 bytes", strings.Repeat("ab", 32), 32, ""},
		{"raw 32-byte string", strings.Repeat("k", 32), 32, ""},
		{"64 non-hex chars rejected", strings.Repeat("zz", 32), 0, "32 bytes"},
		{"too short", "shortkey", 0, "32 bytes"},
		{"too long raw", strings.Repeat("k", 48), 0, "32 bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := parseMasterKey(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(key) != tt.wantLen {
				t.Errorf("key length = %d, want %d", len(key), tt.wantLen)
			}
		})
	}
}

func TestFromEnvRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("OMASTX_MASTER_KEY", strings.Repeat("ab", 32))
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want DATABASE_URL error", err)
	}
}

func TestFromEnvDevDefaults(t *testing.T) {
	t.Setenv("OMASTX_DEV", "true")
	t.Setenv("DATABASE_URL", "postgres://localhost/omastx")
	t.Setenv("OMASTX_MASTER_KEY", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !cfg.DevMode {
		t.Fatal("DevMode = false, want true")
	}
	if len(cfg.MasterKey) != 32 {
		t.Fatalf("MasterKey length = %d, want 32", len(cfg.MasterKey))
	}
}

func TestFromEnvRequiresGitHubInProduction(t *testing.T) {
	t.Setenv("OMASTX_DEV", "")
	t.Setenv("DATABASE_URL", "postgres://localhost/omastx")
	t.Setenv("OMASTX_MASTER_KEY", strings.Repeat("ab", 32))
	t.Setenv("OMASTX_GITHUB_CLIENT_ID", "")
	t.Setenv("OMASTX_GITHUB_CLIENT_SECRET", "")
	t.Setenv("OMASTX_GITHUB_ORG", "")
	t.Setenv("OMASTX_BASE_URL", "")

	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "GITHUB") {
		t.Fatalf("err = %v, want GitHub config error", err)
	}
}
