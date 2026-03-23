package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type YestionClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type YestionGame struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SteamAppID int    `json:"steamAppId"`
}

func NewYestionClient(baseURL, apiKey string) *YestionClient {
	return &YestionClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *YestionClient) doRequest(method, path string, body any) (*http.Response, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	var req *http.Request
	var err error
	if bodyReader != nil {
		req, err = http.NewRequest(method, c.baseURL+path, bodyReader)
	} else {
		req, err = http.NewRequest(method, c.baseURL+path, nil)
	}
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// LookupBySteamID finds a game by Steam app ID. Returns nil if not found (404).
func (c *YestionClient) LookupBySteamID(steamAppID int) (*YestionGame, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/games/by-steam-id/%d", steamAppID), nil)
	if err != nil {
		return nil, fmt.Errorf("lookup by steam id: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lookup by steam id: status %d", resp.StatusCode)
	}

	var game YestionGame
	if err := json.NewDecoder(resp.Body).Decode(&game); err != nil {
		return nil, fmt.Errorf("decode game: %w", err)
	}
	return &game, nil
}

// CreateGame creates a new game in Yestion with the given Steam metadata.
func (c *YestionClient) CreateGame(name, coverURL string, steamAppID int) (*YestionGame, error) {
	payload := map[string]any{
		"name":       name,
		"steamAppId": steamAppID,
	}
	if coverURL != "" {
		payload["coverUrl"] = coverURL
	}

	resp, err := c.doRequest("POST", "/games", payload)
	if err != nil {
		return nil, fmt.Errorf("create game: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, fmt.Errorf("create game: conflict (duplicate name or steamAppId)")
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create game: status %d", resp.StatusCode)
	}

	var game YestionGame
	if err := json.NewDecoder(resp.Body).Decode(&game); err != nil {
		return nil, fmt.Errorf("decode created game: %w", err)
	}
	return &game, nil
}

// UpsertDayGame links a game to a day with the given playtime (SET semantics).
func (c *YestionClient) UpsertDayGame(gameID, date string, minutesPlayed int) error {
	payload := map[string]any{
		"gameId":        gameID,
		"minutesPlayed": minutesPlayed,
	}

	resp, err := c.doRequest("POST", fmt.Sprintf("/games/day/%s", date), payload)
	if err != nil {
		return fmt.Errorf("upsert day game: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("upsert day game: status %d", resp.StatusCode)
	}
	return nil
}
