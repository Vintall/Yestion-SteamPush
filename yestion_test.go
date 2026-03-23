package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestYestionClient_CreateGame_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "Game already exists"})
	}))
	defer srv.Close()

	client := NewYestionClient(srv.URL, "testkey")
	_, err := client.CreateGame("TF2", "", 440)
	if err == nil {
		t.Fatal("expected error for 409")
	}
}

func TestYestionClient_UpsertDayGame(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/games/day/2026-03-23" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		if payload["gameId"] != "game1" {
			t.Errorf("gameId = %v", payload["gameId"])
		}
		if int(payload["minutesPlayed"].(float64)) != 45 {
			t.Errorf("minutesPlayed = %v", payload["minutesPlayed"])
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer srv.Close()

	client := NewYestionClient(srv.URL, "testkey")
	err := client.UpsertDayGame("game1", "2026-03-23", 45)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
