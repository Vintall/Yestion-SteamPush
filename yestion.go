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

type YestionSession struct {
	ID        string `json:"id"`
	GameID    string `json:"gameId"`
	StartedAt string `json:"startedAt"`
	Duration  int    `json:"duration"`
	Source    string `json:"source"`
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
// Returns the game on 201 (created) or 200 (merged into existing by steamAppId).
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

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create game: status %d", resp.StatusCode)
	}

	var game YestionGame
	if err := json.NewDecoder(resp.Body).Decode(&game); err != nil {
		return nil, fmt.Errorf("decode created game: %w", err)
	}
	return &game, nil
}

// CreateSession creates a new game session. Returns the session with its ID.
func (c *YestionClient) CreateSession(gameID string, startedAt time.Time) (*YestionSession, error) {
	payload := map[string]any{
		"gameId":    gameID,
		"startedAt": startedAt.UTC().Format(time.RFC3339),
		"duration":  0,
		"source":    "steam",
	}

	resp, err := c.doRequest("POST", "/game-sessions", payload)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create session: status %d", resp.StatusCode)
	}

	var session YestionSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("decode session: %w", err)
	}
	return &session, nil
}

// UpdateSession updates the duration of an existing game session.
func (c *YestionClient) UpdateSession(sessionID string, duration int) error {
	payload := map[string]any{
		"duration": duration,
	}

	resp, err := c.doRequest("PATCH", fmt.Sprintf("/game-sessions/%s", sessionID), payload)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update session: status %d", resp.StatusCode)
	}
	return nil
}
