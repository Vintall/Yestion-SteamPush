package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	SteamAPIKey            string `json:"steamApiKey"`
	SteamID                string `json:"steamId"`
	YestionURL             string `json:"yestionUrl"`
	YestionAPIKey          string `json:"yestionApiKey"`
	IgnoredAppIDs          []int  `json:"ignoredAppIds"`
	PollInterval           int    `json:"pollIntervalSeconds"`
	HeartbeatInterval      int    `json:"heartbeatIntervalSeconds"`
	StableReadingsRequired int    `json:"stableReadingsRequired"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Validate required fields
	if cfg.SteamAPIKey == "" {
		return nil, fmt.Errorf("config: steamApiKey is required")
	}
	if cfg.SteamID == "" {
		return nil, fmt.Errorf("config: steamId is required")
	}
	if cfg.YestionURL == "" {
		return nil, fmt.Errorf("config: yestionUrl is required")
	}
	if cfg.YestionAPIKey == "" {
		return nil, fmt.Errorf("config: yestionApiKey is required")
	}

	// Apply defaults
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 120
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 1200
	}
	if cfg.StableReadingsRequired <= 0 {
		cfg.StableReadingsRequired = 2
	}

	return &cfg, nil
}
