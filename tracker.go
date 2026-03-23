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
	UpsertDayGame(gameID, date string, minutesPlayed int) error
}

type Tracker struct {
	steam   SteamPoller
	yestion YestionPusher
	config  *Config

	state         TrackerState
	steamDisabled bool // set on auth failure, stops polling
	currentAppID  int
	currentGame   *YestionGame
	sessionStart  time.Time
	activeDate    string
	lastHeartbeat time.Time

	// Stable reading tracking
	consecutiveAppID int
	consecutiveCount int

	// Offline queue
	pendingQueue []pendingAction
}

type pendingAction struct {
	gameID  string
	date    string
	minutes int
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
	now := time.Now()
	today := now.Format("2006-01-02")

	// Check day rollover
	if today != t.activeDate {
		t.dayRollover(today)
	}

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
	t.state = StatePlaying
	t.currentAppID = appID
	t.currentGame = game
	t.sessionStart = now
	t.activeDate = now.Format("2006-01-02")
	t.lastHeartbeat = now
	t.consecutiveAppID = 0
	t.consecutiveCount = 0

	// Initial link with 0 minutes
	if err := t.yestion.UpsertDayGame(game.ID, t.activeDate, 0); err != nil {
		log.Printf("initial upsert failed, queueing: %v", err)
		t.pendingQueue = append(t.pendingQueue, pendingAction{game.ID, t.activeDate, 0})
	}
}

func (t *Tracker) exitPlaying() {
	minutes := int(time.Since(t.lastHeartbeat).Minutes())
	log.Printf("state: PLAYING -> IDLE (game=%q, delta=%d)", t.currentGame.Name, minutes)

	if err := t.yestion.UpsertDayGame(t.currentGame.ID, t.activeDate, minutes); err != nil {
		log.Printf("final upsert failed, queueing: %v", err)
		t.pendingQueue = append(t.pendingQueue, pendingAction{t.currentGame.ID, t.activeDate, minutes})
	}

	t.state = StateIdle
	t.currentAppID = 0
	t.currentGame = nil
}

func (t *Tracker) heartbeat() {
	minutes := int(time.Since(t.lastHeartbeat).Minutes())
	log.Printf("heartbeat: game=%q, delta=%d", t.currentGame.Name, minutes)
	t.lastHeartbeat = time.Now()

	if err := t.yestion.UpsertDayGame(t.currentGame.ID, t.activeDate, minutes); err != nil {
		log.Printf("heartbeat upsert failed, queueing: %v", err)
		t.pendingQueue = append(t.pendingQueue, pendingAction{t.currentGame.ID, t.activeDate, minutes})
	}
}

func (t *Tracker) dayRollover(newDate string) {
	oldDate := t.activeDate
	minutes := int(time.Since(t.lastHeartbeat).Minutes())
	log.Printf("day rollover: %s -> %s (game=%q, delta=%d)", oldDate, newDate, t.currentGame.Name, minutes)

	// Final push for old day
	if err := t.yestion.UpsertDayGame(t.currentGame.ID, oldDate, minutes); err != nil {
		log.Printf("rollover final upsert failed, queueing: %v", err)
		t.pendingQueue = append(t.pendingQueue, pendingAction{t.currentGame.ID, oldDate, minutes})
	}

	// Reset for new day
	t.activeDate = newDate
	t.sessionStart = time.Now()
	t.lastHeartbeat = time.Now()

	// Initial link for new day
	if err := t.yestion.UpsertDayGame(t.currentGame.ID, newDate, 0); err != nil {
		log.Printf("rollover new day upsert failed, queueing: %v", err)
		t.pendingQueue = append(t.pendingQueue, pendingAction{t.currentGame.ID, newDate, 0})
	}
}

// Shutdown performs a final push if currently playing.
func (t *Tracker) Shutdown() {
	if t.state == StatePlaying {
		t.exitPlaying()
	}
	t.retryPending()
}

func (t *Tracker) retryPending() {
	if len(t.pendingQueue) == 0 {
		return
	}

	var remaining []pendingAction
	for _, p := range t.pendingQueue {
		if err := t.yestion.UpsertDayGame(p.gameID, p.date, p.minutes); err != nil {
			log.Printf("retry failed for %s/%s: %v", p.gameID, p.date, err)
			remaining = append(remaining, p)
		} else {
			log.Printf("retry succeeded for %s/%s (%d min)", p.gameID, p.date, p.minutes)
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
