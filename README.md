# Yestion-SteamPush

Background Windows app that detects Steam game launches and pushes play sessions to your Yestion instance.

## Setup

1. Download `yestion-steam-tracker.exe` and `config.example.json` from the [latest release](https://github.com/Vintall/Yestion-SteamPush/releases/latest).
2. Place both files in a folder (e.g. `C:\Users\You\YestionStuff\SteamTracker\`).
3. Copy `config.example.json` to `config.json` and fill in your values:

```json
{
  "steamApiKey": "your Steam Web API key (https://steamcommunity.com/dev/apikey)",
  "steamId": "your 64-bit Steam ID",
  "yestionUrl": "https://your-yestion-instance/api",
  "yestionApiKey": "yestion_ak_your_key",
  "ignoredAppIds": [480],
  "pollIntervalSeconds": 120,
  "heartbeatIntervalSeconds": 1200,
  "stableReadingsRequired": 2
}
```

4. Run `yestion-steam-tracker.exe`. It runs silently in the background and logs to `steam-tracker.log` next to the exe.

## Auto-start with Windows

```
yestion-steam-tracker.exe --install
```

To remove from startup:

```
yestion-steam-tracker.exe --uninstall
```

## Config Reference

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `steamApiKey` | Yes | — | Steam Web API key |
| `steamId` | Yes | — | Your 64-bit Steam ID |
| `yestionUrl` | Yes | — | Yestion backend URL |
| `yestionApiKey` | Yes | — | Yestion API key |
| `ignoredAppIds` | No | `[]` | Steam app IDs to skip (e.g. 480 = Spacewar) |
| `pollIntervalSeconds` | No | `120` | How often to check Steam (seconds) |
| `heartbeatIntervalSeconds` | No | `1200` | How often to push playtime while playing |
| `stableReadingsRequired` | No | `2` | Consecutive readings before confirming a game launch |
