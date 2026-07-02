// Package config loads and validates process configuration from the environment.
package config

import (
	"encoding/hex"
	"fmt"
	"os"
)

type Config struct {
	ListenAddr    string
	DatabaseURL   string
	MasterKey     []byte // 32 bytes, AES-256-GCM key for secrets at rest (SPEC §2.6)
	AdminEmail    string
	AdminPassword string
	SecureCookies bool
}

func FromEnv() (Config, error) {
	cfg := Config{
		ListenAddr:    envOr("OMASTX_LISTEN_ADDR", ":8484"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		AdminEmail:    os.Getenv("OMASTX_ADMIN_EMAIL"),
		AdminPassword: os.Getenv("OMASTX_ADMIN_PASSWORD"),
		SecureCookies: os.Getenv("OMASTX_SECURE_COOKIES") == "true",
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	key, err := parseMasterKey(os.Getenv("OMASTX_MASTER_KEY"))
	if err != nil {
		return cfg, err
	}
	cfg.MasterKey = key
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
