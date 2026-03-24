# Yestion-SteamPush — Claude Instructions

## Project

Go app that polls Steam for currently playing game and pushes session data to the Yestion backend API. Runs as a Windows service/background process.

## Tech Stack

- Language: Go 1.22
- Build: Docker (Dockerfile.build) — Go is NOT installed locally
- Target: Windows amd64 binary

## Build & Test

Everything runs in Docker. Go is NOT available locally.

```bash
docker build -f Dockerfile.build -t yestion-steampush-build .
docker run --rm yestion-steampush-build
```

This runs `go test -v ./...` then builds the Windows binary.

To just run tests:
```bash
docker run --rm yestion-steampush-build sh -c "go mod tidy && go test -v ./..."
```

## Architecture

```
Steam API  -->  SteamPoller  -->  Tracker (state machine)  -->  YestionPusher  -->  Yestion Backend API
```

- **Tracker**: IDLE <-> PLAYING state machine. Polls Steam every N seconds, requires consecutive stable readings before transitioning.
- **YestionPusher**: HTTP client that talks to Yestion backend (CreateSession, UpdateSession, LookupBySteamID, CreateGame).
- **SteamPoller**: Polls Steam Web API for currently running game.
- **Config**: JSON config file with API keys, polling intervals, ignored app IDs.

## Key Files

| File | Purpose |
|---|---|
| main.go | Entry point, signal handling |
| tracker.go | State machine (IDLE/PLAYING), heartbeat, offline queue |
| yestion.go | Yestion API client (games, sessions) |
| steam.go | Steam API polling |
| config.go | JSON config loading |
| startup.go | Startup checks and validation |

## Important Decisions

- Sessions track total duration in seconds (not delta minutes) — backend handles day overlap
- No day rollover logic in tracker — backend splits sessions across days automatically
- Offline queue retries UpdateSession calls on next poll
- Stable readings required before entering PLAYING state (debounce)
- Steam auth failures permanently disable polling until restart
