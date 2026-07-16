// Package config loads and validates process configuration from the environment.
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Dev-only defaults when OMASTX_DEV=true. Never enable OMASTX_DEV in production.
const DevMasterKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type Config struct {
	ListenAddr         string
	DatabaseURL        string
	MasterKey          []byte // 32 bytes, AES-256-GCM key for secrets at rest (SPEC §2.6)
	SecureCookies      bool
	DevMode            bool
	GitHubClientID     string
	GitHubClientSecret string
	GitHubOrg          string
	BaseURL            string // public origin, e.g. https://omastx.example.com
}

func FromEnv() (Config, error) {
	dev := os.Getenv("OMASTX_DEV") == "true"
	cfg := Config{
		ListenAddr:         envOr("OMASTX_LISTEN_ADDR", ":8484"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		SecureCookies:      os.Getenv("OMASTX_SECURE_COOKIES") == "true",
		DevMode:            dev,
		GitHubClientID:     os.Getenv("OMASTX_GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("OMASTX_GITHUB_CLIENT_SECRET"),
		GitHubOrg:          os.Getenv("OMASTX_GITHUB_ORG"),
		BaseURL:            strings.TrimRight(os.Getenv("OMASTX_BASE_URL"), "/"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	masterKeyRaw := os.Getenv("OMASTX_MASTER_KEY")
	if masterKeyRaw == "" && dev {
		masterKeyRaw = DevMasterKeyHex
	}
	key, err := parseMasterKey(masterKeyRaw)
	if err != nil {
		return cfg, err
	}
	cfg.MasterKey = key
	if !dev {
		if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" {
			return cfg, fmt.Errorf("OMASTX_GITHUB_CLIENT_ID and OMASTX_GITHUB_CLIENT_SECRET are required")
		}
		if cfg.GitHubOrg == "" {
			return cfg, fmt.Errorf("OMASTX_GITHUB_ORG is required")
		}
		if cfg.BaseURL == "" {
			return cfg, fmt.Errorf("OMASTX_BASE_URL is required (public origin for OAuth callback)")
		}
	}
	return cfg, nil
}

// parseMasterKey accepts either a 64-char hex string or a raw 32-byte string.
func parseMasterKey(raw string) ([]byte, error) {
	if raw == "" {
		return nil, fmt.Errorf("OMASTX_MASTER_KEY is required (32 bytes: 64 hex chars, e.g. `openssl rand -hex 32`)")
	}
	if len(raw) == 64 {
		if key, err := hex.DecodeString(raw); err == nil {
			return key, nil
		}
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, fmt.Errorf("OMASTX_MASTER_KEY must be 32 bytes (got %d chars; use `openssl rand -hex 32`)", len(raw))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
