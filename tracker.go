package main

import (
	"errors"
	"log"
	"time"
)

type TrackerState int

const (
	StateIdle    TrackerState = iota
	StatePlaying
)

func (s TrackerState) String() string {
	switch s {
	case StateIdle:
		return "IDLE"
	case StatePlaying:
		return "PLAYING"
	default:
		return "UNKNOWN"
	}
}

// Interfaces for testing
type SteamPoller interface {
	GetCurrentGame() (int, string, error)
	ResolveAppInfo(appID int) (*AppInfo, error)
}

type YestionPusher interface {
	LookupBySteamID(steamAppID int) (*YestionGame, error)
	CreateGame(name, coverURL string, steamAppID int) (*YestionGame, error)
	CreateSession(gameID string, startedAt time.Time) (*YestionSession, error)
	UpdateSession(sessionID string, duration int) error
}

type Tracker struct {
	steam   SteamPoller
	yestion YestionPusher
	config  *Config

	state         TrackerState
	steamDisabled bool // set on auth failure, stops polling
	currentAppID  int
	currentGame   *YestionGame
	sessionID     string
	sessionStart  time.Time
	lastHeartbeat time.Time

	// Stable reading tracking
	consecutiveAppID int
	consecutiveCount int

	// Offline queue
	pendingQueue []pendingAction
}

type pendingAction struct {
	sessionID string
	duration  int
}

func NewTracker(steam SteamPoller, yestion YestionPusher, config *Config) *Tracker {
	return &Tracker{
		steam:   steam,
		yestion: yestion,
		config:  config,
		state:   StateIdle,
	}
}

func (t *Tracker) State() TrackerState {
	return t.state
}

func (t *Tracker) Poll() {
	if t.steamDisabled {
		return
	}

	// Retry pending queue first
	t.retryPending()

	appID, name, err := t.steam.GetCurrentGame()
	if err != nil {
		if errors.Is(err, ErrSteamAuth) {
			log.Printf("steam auth failed: %v — polling stopped until restart", err)
			t.steamDisabled = true
			return
		}
		log.Printf("steam poll error: %v", err)
		return
	}

	// Filter ignored app IDs
	if t.isIgnored(appID) {
		appID = 0
		name = ""
	}

	switch t.state {
	case StateIdle:
		t.pollIdle(appID, name)
	case StatePlaying:
		t.pollPlaying(appID)
	}
}

func (t *Tracker) pollIdle(appID int, name string) {
	if appID == 0 {
		t.consecutiveAppID = 0
		t.consecutiveCount = 0
		return
	}

	// Track consecutive readings
	if appID == t.consecutiveAppID {
		t.consecutiveCount++
	} else {
		t.consecutiveAppID = appID
		t.consecutiveCount = 1
	}

	if t.consecutiveCount < t.config.StableReadingsRequired {
		return
	}

	// Stable reading achieved — transition to PLAYING
	t.enterPlaying(appID, name)
}

func (t *Tracker) pollPlaying(appID int) {
	if appID == t.currentAppID {
		// Still playing — check heartbeat
		if t.config.HeartbeatInterval > 0 &&
			time.Since(t.lastHeartbeat).Seconds() >= float64(t.config.HeartbeatInterval) {
			t.heartbeat()
		}
		return
	}

	// Game changed or stopped — exit PLAYING
	t.exitPlaying()

	// If new game detected, start tracking consecutive readings for it
	if appID != 0 {
		t.consecutiveAppID = appID
		t.consecutiveCount = 1
	}
}

func (t *Tracker) enterPlaying(appID int, name string) {
	log.Printf("state: IDLE -> PLAYING (appId=%d, name=%q)", appID, name)

	// Resolve or create game in Yestion
	game, err := t.yestion.LookupBySteamID(appID)
	if err != nil {
		log.Printf("lookup by steam id failed: %v", err)
		return
	}

	if game == nil {
		// Resolve from Steam store
		info, err := t.steam.ResolveAppInfo(appID)
		if err != nil {
			log.Printf("resolve app info failed: %v, using poll name", err)
			info = &AppInfo{Name: name}
		}
		if info.Name == "" {
			info.Name = name
		}

		game, err = t.yestion.CreateGame(info.Name, info.CoverURL, appID)
		if err != nil {
			log.Printf("create game failed: %v", err)
			return
		}
		log.Printf("created game %q (id=%s)", game.Name, game.ID)
	}

	now := time.Now()

	// Create session in Yestion
	session, err := t.yestion.CreateSession(game.ID, now)
	if err != nil {
		log.Printf("create session failed: %v", err)
		return
	}

	t.state = StatePlaying
	t.currentAppID = appID
	t.currentGame = game
	t.sessionID = session.ID
	t.sessionStart = now
	t.lastHeartbeat = now
	t.consecutiveAppID = 0
	t.consecutiveCount = 0
}

func (t *Tracker) exitPlaying() {
	duration := int(time.Since(t.sessionStart).Minutes())
	log.Printf("state: PLAYING -> IDLE (game=%q, duration=%dm)", t.currentGame.Name, duration)

	if err := t.yestion.UpdateSession(t.sessionID, duration); err != nil {
		log.Printf("final update failed, queueing: %v", err)
		t.queuePending(t.sessionID, duration)
	}

	t.state = StateIdle
	t.currentAppID = 0
	t.currentGame = nil
	t.sessionID = ""
}

func (t *Tracker) heartbeat() {
	duration := int(time.Since(t.sessionStart).Minutes())
	log.Printf("heartbeat: game=%q, duration=%dm", t.currentGame.Name, duration)
	t.lastHeartbeat = time.Now()

	if err := t.yestion.UpdateSession(t.sessionID, duration); err != nil {
		log.Printf("heartbeat update failed, queueing: %v", err)
		t.queuePending(t.sessionID, duration)
	}
}

// Shutdown performs a final push if currently playing.
func (t *Tracker) Shutdown() {
	if t.state == StatePlaying {
		t.exitPlaying()
	}
	t.retryPending()
}

// queuePending adds or replaces a pending action for a session.
// Only the latest (largest) duration is kept per sessionID to prevent
// stale heartbeat retries from regressing the server-side value.
func (t *Tracker) queuePending(sessionID string, duration int) {
	for i, p := range t.pendingQueue {
		if p.sessionID == sessionID {
			t.pendingQueue[i].duration = duration
			return
		}
	}
	t.pendingQueue = append(t.pendingQueue, pendingAction{sessionID, duration})
}

func (t *Tracker) retryPending() {
	if len(t.pendingQueue) == 0 {
		return
	}

	var remaining []pendingAction
	for _, p := range t.pendingQueue {
		if err := t.yestion.UpdateSession(p.sessionID, p.duration); err != nil {
			log.Printf("retry failed for session %s: %v", p.sessionID, err)
			remaining = append(remaining, p)
		} else {
			log.Printf("retry succeeded for session %s (%dm)", p.sessionID, p.duration)
		}
	}
	t.pendingQueue = remaining
}

func (t *Tracker) isIgnored(appID int) bool {
	for _, id := range t.config.IgnoredAppIDs {
		if id == appID {
			return true
		}
	}
	return false
}
