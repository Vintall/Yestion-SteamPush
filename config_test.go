package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{
		"steamApiKey": "abc123",
		"steamId": "76561198000000000",
		"yestionUrl": "http://localhost:3000",
		"yestionApiKey": "yestion_ak_test",
		"ignoredAppIds": [480],
		"pollIntervalSeconds": 60,
		"heartbeatIntervalSeconds": 600,
		"stableReadingsRequired": 2
	}`
	os.WriteFile(path, []byte(data), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SteamAPIKey != "abc123" {
		t.Errorf("SteamAPIKey = %q, want %q", cfg.SteamAPIKey, "abc123")
	}
	if cfg.PollInterval != 60 {
		t.Errorf("PollInterval = %d, want 60", cfg.PollInterval)
	}
	if len(cfg.IgnoredAppIDs) != 1 || cfg.IgnoredAppIDs[0] != 480 {
		t.Errorf("IgnoredAppIDs = %v, want [480]", cfg.IgnoredAppIDs)
	}
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Missing steamApiKey
	data := `{
		"steamId": "76561198000000000",
		"yestionUrl": "http://localhost:3000",
		"yestionApiKey": "yestion_ak_test"
	}`
	os.WriteFile(path, []byte(data), 0644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing steamApiKey")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{
		"steamApiKey": "abc",
		"steamId": "123",
		"yestionUrl": "http://localhost:3000",
		"yestionApiKey": "yestion_ak_x"
	}`
	os.WriteFile(path, []byte(data), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollInterval != 120 {
		t.Errorf("PollInterval default = %d, want 120", cfg.PollInterval)
	}
	if cfg.HeartbeatInterval != 1200 {
		t.Errorf("HeartbeatInterval default = %d, want 1200", cfg.HeartbeatInterval)
	}
	if cfg.StableReadingsRequired != 2 {
		t.Errorf("StableReadingsRequired default = %d, want 2", cfg.StableReadingsRequired)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
