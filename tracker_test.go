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
	lookupResult *YestionGame
	createResult *YestionGame
	upsertCalls  []upsertCall
	createErr    error
}

type upsertCall struct {
	gameID  string
	date    string
	minutes int
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

func (m *mockYestion) UpsertDayGame(gameID, date string, minutesPlayed int) error {
	m.upsertCalls = append(m.upsertCalls, upsertCall{gameID, date, minutesPlayed})
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

	// Should have upserted day-game with 0 minutes
	if len(yestion.upsertCalls) != 1 {
		t.Fatalf("upsert calls = %d, want 1", len(yestion.upsertCalls))
	}
	if yestion.upsertCalls[0].minutes != 0 {
		t.Errorf("initial minutesPlayed = %d, want 0", yestion.upsertCalls[0].minutes)
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

	// Simulate time passing
	tracker.sessionStart = time.Now().Add(-5 * time.Minute)

	// Game stops
	steam.appID = 0
	steam.name = ""
	tracker.Poll()
	if tracker.State() != StateIdle {
		t.Errorf("state = %v, want IDLE", tracker.State())
	}

	// Should have final upsert with accumulated minutes
	lastCall := yestion.upsertCalls[len(yestion.upsertCalls)-1]
	if lastCall.minutes < 4 || lastCall.minutes > 6 {
		t.Errorf("final minutesPlayed = %d, want ~5", lastCall.minutes)
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
	if len(yestion.upsertCalls) != 1 {
		t.Fatalf("upsert calls = %d, want 1", len(yestion.upsertCalls))
	}
	if yestion.upsertCalls[0].gameID != "new1" {
		t.Errorf("gameID = %q, want %q", yestion.upsertCalls[0].gameID, "new1")
	}
}

func TestTracker_DayRollover(t *testing.T) {
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

	// Simulate day change: set activeDate to yesterday
	tracker.activeDate = "2026-03-22"
	tracker.sessionStart = time.Now().Add(-30 * time.Minute)
	initialCalls := len(yestion.upsertCalls)

	// Poll again — should trigger day rollover
	tracker.Poll()
	if tracker.State() != StatePlaying {
		t.Errorf("state after rollover = %v, want PLAYING", tracker.State())
	}

	// Should have 2 new upsert calls: final for old date + initial for new date
	newCalls := yestion.upsertCalls[initialCalls:]
	if len(newCalls) != 2 {
		t.Fatalf("new upsert calls = %d, want 2", len(newCalls))
	}
	if newCalls[0].date != "2026-03-22" {
		t.Errorf("first call date = %q, want old date", newCalls[0].date)
	}
	if newCalls[0].minutes < 29 {
		t.Errorf("first call minutes = %d, want ~30", newCalls[0].minutes)
	}
	today := time.Now().Format("2006-01-02")
	if newCalls[1].date != today {
		t.Errorf("second call date = %q, want %q", newCalls[1].date, today)
	}
	if newCalls[1].minutes != 0 {
		t.Errorf("second call minutes = %d, want 0", newCalls[1].minutes)
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

	// Simulate time passing
	tracker.sessionStart = time.Now().Add(-10 * time.Minute)

	// Graceful shutdown
	tracker.Shutdown()
	if tracker.State() != StateIdle {
		t.Errorf("state after shutdown = %v, want IDLE", tracker.State())
	}

	// Should have final upsert with ~10 minutes
	lastCall := yestion.upsertCalls[len(yestion.upsertCalls)-1]
	if lastCall.minutes < 9 || lastCall.minutes > 11 {
		t.Errorf("final minutesPlayed = %d, want ~10", lastCall.minutes)
	}
}

func TestTracker_OfflineQueueRetry(t *testing.T) {
	steam := &mockSteam{appID: 0, name: ""}
	yestion := &mockYestion{}

	tracker := NewTracker(steam, yestion, &Config{StableReadingsRequired: 1})

	// Manually add a pending action to simulate a failed push
	tracker.pendingQueue = append(tracker.pendingQueue, pendingAction{
		gameID: "game1", date: "2026-03-23", minutes: 30,
	})

	// Next poll should retry the pending queue
	steam.appID = 0 // not playing anymore
	tracker.Poll()

	// Pending queue should be drained (mock always succeeds)
	if len(tracker.pendingQueue) != 0 {
		t.Errorf("pending queue = %d, want 0", len(tracker.pendingQueue))
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
