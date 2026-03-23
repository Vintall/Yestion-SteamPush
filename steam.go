package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// ErrSteamAuth is returned when the Steam API responds with 401/403.
// The caller should stop polling — the key is likely invalid.
var ErrSteamAuth = errors.New("steam API auth error")

type SteamClient struct {
	apiKey       string
	steamID      string
	apiBaseURL   string
	storeBaseURL string
	httpClient   *http.Client
}

type AppInfo struct {
	Name     string
	CoverURL string
}

func NewSteamClient(apiKey, steamID, apiBaseURL string) *SteamClient {
	if apiBaseURL == "" {
		apiBaseURL = "https://api.steampowered.com"
	}
	return &SteamClient{
		apiKey:       apiKey,
		steamID:      steamID,
		apiBaseURL:   apiBaseURL,
		storeBaseURL: "https://store.steampowered.com",
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// GetCurrentGame returns the app ID and name of the currently running game.
// Returns (0, "", nil) if the user is not in a game.
// Returns ErrSteamAuth (wrapped) on 401/403 — caller should stop polling.
func (c *SteamClient) GetCurrentGame() (int, string, error) {
	url := fmt.Sprintf("%s/ISteamUser/GetPlayerSummaries/v0002/?key=%s&steamids=%s",
		c.apiBaseURL, c.apiKey, c.steamID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return 0, "", fmt.Errorf("steam API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return 0, "", fmt.Errorf("%w: %d", ErrSteamAuth, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("steam API error: %d", resp.StatusCode)
	}

	var result struct {
		Response struct {
			Players []struct {
				GameID        string `json:"gameid"`
				GameExtraInfo string `json:"gameextrainfo"`
			} `json:"players"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, "", fmt.Errorf("decode steam response: %w", err)
	}

	if len(result.Response.Players) == 0 {
		return 0, "", nil
	}

	player := result.Response.Players[0]
	if player.GameID == "" {
		return 0, "", nil
	}

	appID, err := strconv.Atoi(player.GameID)
	if err != nil {
		return 0, "", fmt.Errorf("parse gameid %q: %w", player.GameID, err)
	}

	return appID, player.GameExtraInfo, nil
}

// ResolveAppInfo fetches game name and header image from the Steam store API.
func (c *SteamClient) ResolveAppInfo(appID int) (*AppInfo, error) {
	url := fmt.Sprintf("%s/api/appdetails?appids=%d", c.storeBaseURL, appID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("steam store request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam store error: %d", resp.StatusCode)
	}

	var result map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Name        string `json:"name"`
			HeaderImage string `json:"header_image"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode store response: %w", err)
	}

	key := strconv.Itoa(appID)
	entry, ok := result[key]
	if !ok || !entry.Success {
		return nil, fmt.Errorf("app %d not found in store", appID)
	}

	return &AppInfo{
		Name:     entry.Data.Name,
		CoverURL: entry.Data.HeaderImage,
	}, nil
}
