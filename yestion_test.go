package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestYestionClient_LookupBySteamID_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/games/by-steam-id/440" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer testkey" {
			t.Errorf("missing auth header")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "game1", "name": "TF2", "steamAppId": 440,
		})
	}))
	defer srv.Close()

	client := NewYestionClient(srv.URL, "testkey")
	game, err := client.LookupBySteamID(440)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if game == nil {
		t.Fatal("expected game, got nil")
	}
	if game.ID != "game1" {
		t.Errorf("ID = %q, want %q", game.ID, "game1")
	}
}

func TestYestionClient_LookupBySteamID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Game not found"})
	}))
	defer srv.Close()

	client := NewYestionClient(srv.URL, "testkey")
	game, err := client.LookupBySteamID(99999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if game != nil {
		t.Errorf("expected nil, got %+v", game)
	}
}

func TestYestionClient_CreateGame(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/games" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		if payload["name"] != "TF2" {
			t.Errorf("name = %v", payload["name"])
		}
		if int(payload["steamAppId"].(float64)) != 440 {
			t.Errorf("steamAppId = %v", payload["steamAppId"])
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": "game1", "name": "TF2", "steamAppId": 440})
	}))
	defer srv.Close()

	client := NewYestionClient(srv.URL, "testkey")
	game, err := client.CreateGame("TF2", "https://img/cover.jpg", 440)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if game.ID != "game1" {
		t.Errorf("ID = %q, want %q", game.ID, "game1")
	}
}

func TestYestionClient_CreateGame_Merge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server returns 200 (merged into existing) instead of 201
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"id": "existing1", "name": "TF2", "steamAppId": 440,
		})
	}))
	defer srv.Close()

	client := NewYestionClient(srv.URL, "testkey")
	game, err := client.CreateGame("TF2", "", 440)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if game.ID != "existing1" {
		t.Errorf("ID = %q, want %q", game.ID, "existing1")
	}
}

func TestYestionClient_CreateSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/game-sessions" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		if payload["gameId"] != "game1" {
			t.Errorf("gameId = %v", payload["gameId"])
		}
		if payload["source"] != "steam" {
			t.Errorf("source = %v", payload["source"])
		}
		if int(payload["duration"].(float64)) != 0 {
			t.Errorf("duration = %v, want 0", payload["duration"])
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id": "sess-1", "gameId": "game1", "duration": 0, "source": "steam",
		})
	}))
	defer srv.Close()

	client := NewYestionClient(srv.URL, "testkey")
	session, err := client.CreateSession("game1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.ID != "sess-1" {
		t.Errorf("ID = %q, want %q", session.ID, "sess-1")
	}
	if session.GameID != "game1" {
		t.Errorf("GameID = %q, want %q", session.GameID, "game1")
	}
}

func TestYestionClient_UpdateSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.URL.Path != "/game-sessions/sess-1" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		if int(payload["duration"].(float64)) != 2700 {
			t.Errorf("duration = %v, want 2700", payload["duration"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewYestionClient(srv.URL, "testkey")
	err := client.UpdateSession("sess-1", 2700)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
