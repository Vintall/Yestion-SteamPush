package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSteamClient_GetCurrentGame_Playing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISteamUser/GetPlayerSummaries/v0002/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		key := r.URL.Query().Get("key")
		if key != "testkey" {
			t.Errorf("key = %q, want %q", key, "testkey")
		}
		resp := map[string]any{
			"response": map[string]any{
				"players": []map[string]any{
					{
						"steamid":       "123",
						"gameid":        "440",
						"gameextrainfo": "Team Fortress 2",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewSteamClient("testkey", "123", srv.URL)
	appID, name, err := client.GetCurrentGame()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appID != 440 {
		t.Errorf("appID = %d, want 440", appID)
	}
	if name != "Team Fortress 2" {
		t.Errorf("name = %q, want %q", name, "Team Fortress 2")
	}
}

func TestSteamClient_GetCurrentGame_NotPlaying(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"response": map[string]any{
				"players": []map[string]any{
					{"steamid": "123"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewSteamClient("testkey", "123", srv.URL)
	appID, name, err := client.GetCurrentGame()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appID != 0 {
		t.Errorf("appID = %d, want 0", appID)
	}
	if name != "" {
		t.Errorf("name = %q, want empty", name)
	}
}

func TestSteamClient_GetCurrentGame_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewSteamClient("badkey", "123", srv.URL)
	_, _, err := client.GetCurrentGame()
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !errors.Is(err, ErrSteamAuth) {
		t.Errorf("expected ErrSteamAuth, got: %v", err)
	}
}

func TestSteamClient_ResolveAppInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"440": map[string]any{
				"success": true,
				"data": map[string]any{
					"name":         "Team Fortress 2",
					"header_image": "https://cdn.steam/tf2_header.jpg",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewSteamClient("testkey", "123", "")
	client.storeBaseURL = srv.URL
	info, err := client.ResolveAppInfo(440)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "Team Fortress 2" {
		t.Errorf("Name = %q, want %q", info.Name, "Team Fortress 2")
	}
	if info.CoverURL != "https://cdn.steam/tf2_header.jpg" {
		t.Errorf("CoverURL = %q", info.CoverURL)
	}
}
