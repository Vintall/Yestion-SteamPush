package main

import (
	"fmt"
	"testing"
	"time"
)

// Mock Steam client
type mockSteam struct {
	appID int
	name  string
	err   error
	info  *AppInfo
}

func (m *mockSteam) GetCurrentGame() (int, string, error) {
	if m.err != nil {
		return 0, "", m.err
	}
	return m.appID, m.name, nil
}

func (m *mockSteam) ResolveAppInfo(appID int) (*AppInfo, error) {
	if m.info != nil {
		return m.info, nil
	}
	return &AppInfo{Name: m.name, CoverURL: ""}, nil
}

// Mock Yestion client
type mockYestion struct {
	lookupResult    *YestionGame
	createResult    *YestionGame
	sessionResult   *YestionSession
	createErr       error
	sessionErr      error
	updateCalls     []updateCall
	createSessCalls []createSessCall
}

type updateCall struct {
	sessionID string
	duration  int
}

type createSessCall struct {
	gameID    string
	startedAt time.Time
}

func (m *mockYestion) LookupBySteamID(steamAppID int) (*YestionGame, error) {
	return m.lookupResult, nil
}

func (m *mockYestion) CreateGame(name, coverURL string, steamAppID int) (*YestionGame, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.createResult, nil
}

func (m *mockYestion) CreateSession(gameID string, startedAt time.Time) (*YestionSession, error) {
	m.createSessCalls = append(m.createSessCalls, createSessCall{gameID, startedAt})
	if m.sessionErr != nil {
		return nil, m.sessionErr
	}
	if m.sessionResult != nil {
		return m.sessionResult, nil
	}
	return &YestionSession{ID: "sess-1", GameID: gameID, Source: "steam"}, nil
}

func (m *mockYestion) UpdateSession(sessionID string, duration int) error {
	m.updateCalls = append(m.updateCalls, updateCall{sessionID, duration})
	return nil
}

func TestTracker_IdleToPlaying(t *testing.T) {
	steam := &mockSteam{appID: 440, name: "TF2"}
	yestion := &mockYestion{
		lookupResult: &YestionGame{ID: "game1", Name: "TF2", SteamAppID: 440},
	}
	tracker := NewTracker(steam, yestion, &Config{
		StableReadingsRequired: 2,
		IgnoredAppIDs:          []int{480},
	})

	// First poll: not yet stable
	tracker.Poll()
	if tracker.State() != StateIdle {
		t.Errorf("after 1 poll: state = %v, want IDLE", tracker.State())
	}

	// Second poll: stable, transitions to PLAYING
	tracker.Poll()
	if tracker.State() != StatePlaying {
		t.Errorf("after 2 polls: state = %v, want PLAYING", tracker.State())
	}

	// Should have created a session
	if len(yestion.createSessCalls) != 1 {
		t.Fatalf("createSession calls = %d, want 1", len(yestion.createSessCalls))
	}
	if yestion.createSessCalls[0].gameID != "game1" {
		t.Errorf("session gameID = %q, want %q", yestion.createSessCalls[0].gameID, "game1")
	}
}

func TestTracker_PlayingToIdle(t *testing.T) {
	steam := &mockSteam{appID: 440, name: "TF2"}
	yestion := &mockYestion{
		lookupResult: &YestionGame{ID: "game1", Name: "TF2", SteamAppID: 440},
	}
	cfg := &Config{StableReadingsRequired: 1, PollInterval: 60}
	tracker := NewTracker(steam, yestion, cfg)

	// Enter PLAYING
	tracker.Poll()
	if tracker.State() != StatePlaying {
		t.Fatalf("state = %v, want PLAYING", tracker.State())
	}

	// Simulate time passing (5 minutes)
	tracker.sessionStart = time.Now().Add(-5 * time.Minute)
	tracker.lastHeartbeat = time.Now().Add(-5 * time.Minute)

	// Game stops
	steam.appID = 0
	steam.name = ""
	tracker.Poll()
	if tracker.State() != StateIdle {
		t.Errorf("state = %v, want IDLE", tracker.State())
	}

	// Should have final update with accumulated minutes (~5)
	if len(yestion.updateCalls) == 0 {
		t.Fatal("expected update calls")
	}
	lastCall := yestion.updateCalls[len(yestion.updateCalls)-1]
	if lastCall.duration < 4 || lastCall.duration > 6 {
		t.Errorf("final duration = %d, want ~5", lastCall.duration)
	}
}

func TestTracker_IgnoredAppID(t *testing.T) {
	steam := &mockSteam{appID: 480, name: "Spacewar"}
	yestion := &mockYestion{}
	tracker := NewTracker(steam, yestion, &Config{
		StableReadingsRequired: 1,
		IgnoredAppIDs:          []int{480},
	})

	tracker.Poll()
	tracker.Poll()
	if tracker.State() != StateIdle {
		t.Errorf("state = %v, want IDLE (480 is ignored)", tracker.State())
	}
}

func TestTracker_CreatesNewGame(t *testing.T) {
	steam := &mockSteam{
		appID: 440, name: "TF2",
		info: &AppInfo{Name: "Team Fortress 2", CoverURL: "https://cdn/tf2.jpg"},
	}
	yestion := &mockYestion{
		lookupResult: nil, // not found
		createResult: &YestionGame{ID: "new1", Name: "Team Fortress 2", SteamAppID: 440},
	}
	tracker := NewTracker(steam, yestion, &Config{StableReadingsRequired: 1})

	tracker.Poll()
	if tracker.State() != StatePlaying {
		t.Fatalf("state = %v, want PLAYING", tracker.State())
	}
	if len(yestion.createSessCalls) != 1 {
		t.Fatalf("createSession calls = %d, want 1", len(yestion.createSessCalls))
	}
	if yestion.createSessCalls[0].gameID != "new1" {
		t.Errorf("gameID = %q, want %q", yestion.createSessCalls[0].gameID, "new1")
	}
}

func TestTracker_Shutdown(t *testing.T) {
	steam := &mockSteam{appID: 440, name: "TF2"}
	yestion := &mockYestion{
		lookupResult: &YestionGame{ID: "game1", Name: "TF2", SteamAppID: 440},
	}
	tracker := NewTracker(steam, yestion, &Config{StableReadingsRequired: 1})

	// Enter PLAYING
	tracker.Poll()
	if tracker.State() != StatePlaying {
		t.Fatalf("state = %v, want PLAYING", tracker.State())
	}

	// Simulate time passing (10 minutes)
	tracker.sessionStart = time.Now().Add(-10 * time.Minute)
	tracker.lastHeartbeat = time.Now().Add(-10 * time.Minute)

	// Graceful shutdown
	tracker.Shutdown()
	if tracker.State() != StateIdle {
		t.Errorf("state after shutdown = %v, want IDLE", tracker.State())
	}

	// Should have final update with ~10 minutes
	if len(yestion.updateCalls) == 0 {
		t.Fatal("expected update calls")
	}
	lastCall := yestion.updateCalls[len(yestion.updateCalls)-1]
	if lastCall.duration < 9 || lastCall.duration > 11 {
		t.Errorf("final duration = %d, want ~10", lastCall.duration)
	}
}

func TestTracker_OfflineQueueRetry(t *testing.T) {
	steam := &mockSteam{appID: 0, name: ""}
	yestion := &mockYestion{}

	tracker := NewTracker(steam, yestion, &Config{StableReadingsRequired: 1})

	// Manually add a pending action to simulate a failed push
	tracker.pendingQueue = append(tracker.pendingQueue, pendingAction{
		sessionID: "sess-1", duration: 30,
	})

	// Next poll should retry the pending queue
	tracker.Poll()

	// Pending queue should be drained (mock always succeeds)
	if len(tracker.pendingQueue) != 0 {
		t.Errorf("pending queue = %d, want 0", len(tracker.pendingQueue))
	}

	// Should have retried the update
	if len(yestion.updateCalls) != 1 {
		t.Fatalf("update calls = %d, want 1", len(yestion.updateCalls))
	}
	if yestion.updateCalls[0].sessionID != "sess-1" {
		t.Errorf("sessionID = %q, want %q", yestion.updateCalls[0].sessionID, "sess-1")
	}
	if yestion.updateCalls[0].duration != 30 {
		t.Errorf("duration = %d, want 30", yestion.updateCalls[0].duration)
	}
}

func TestTracker_SteamAuthStopsPolling(t *testing.T) {
	steam := &mockSteam{appID: 440, name: "TF2", err: fmt.Errorf("%w: 403", ErrSteamAuth)}
	yestion := &mockYestion{}
	tracker := NewTracker(steam, yestion, &Config{StableReadingsRequired: 1})

	tracker.Poll()
	// Should have set steamDisabled
	if !tracker.steamDisabled {
		t.Error("steamDisabled should be true after auth error")
	}

	// Subsequent polls should be no-ops
	steam.err = nil
	steam.appID = 440
	tracker.Poll()
	if tracker.State() != StateIdle {
		t.Error("should remain IDLE — polling disabled")
	}
}

func TestTracker_HeartbeatUpdatesDuration(t *testing.T) {
	steam := &mockSteam{appID: 440, name: "TF2"}
	yestion := &mockYestion{
		lookupResult: &YestionGame{ID: "game1", Name: "TF2", SteamAppID: 440},
	}
	cfg := &Config{StableReadingsRequired: 1, HeartbeatInterval: 60}
	tracker := NewTracker(steam, yestion, cfg)

	// Enter PLAYING
	tracker.Poll()
	if tracker.State() != StatePlaying {
		t.Fatalf("state = %v, want PLAYING", tracker.State())
	}

	// Simulate time passing past heartbeat interval
	tracker.sessionStart = time.Now().Add(-3 * time.Minute)
	tracker.lastHeartbeat = time.Now().Add(-2 * time.Minute)

	// Poll should trigger heartbeat
	tracker.Poll()

	// Should have an update call with total duration from session start (~3 minutes)
	if len(yestion.updateCalls) == 0 {
		t.Fatal("expected heartbeat update call")
	}
	lastCall := yestion.updateCalls[len(yestion.updateCalls)-1]
	if lastCall.duration < 2 || lastCall.duration > 4 {
		t.Errorf("heartbeat duration = %d, want ~3", lastCall.duration)
	}
	if lastCall.sessionID != "sess-1" {
		t.Errorf("sessionID = %q, want %q", lastCall.sessionID, "sess-1")
	}
}
