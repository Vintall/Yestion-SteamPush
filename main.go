package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

func main() {
	install := flag.Bool("install", false, "Add to Windows startup")
	uninstall := flag.Bool("uninstall", false, "Remove from Windows startup")
	flag.Parse()

	if *install {
		if err := installStartup(); err != nil {
			fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Added to Windows startup.")
		return
	}
	if *uninstall {
		if err := uninstallStartup(); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Removed from Windows startup.")
		return
	}

	// Determine exe directory for config and log paths
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine exe path: %v\n", err)
		os.Exit(1)
	}
	exeDir := filepath.Dir(exePath)

	// Setup logging to file next to exe
	logPath := filepath.Join(exeDir, "steam-tracker.log")
	log.SetOutput(&lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    5, // MB
		MaxBackups: 1,
	})
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("=== steam-tracker starting ===")

	// Load config
	configPath := filepath.Join(exeDir, "config.json")
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	log.Printf("config loaded: poll=%ds, heartbeat=%ds, stableReadings=%d, ignored=%v",
		cfg.PollInterval, cfg.HeartbeatInterval, cfg.StableReadingsRequired, cfg.IgnoredAppIDs)

	// Initialize clients
	steam := NewSteamClient(cfg.SteamAPIKey, cfg.SteamID, "")
	yestion := NewYestionClient(cfg.YestionURL, cfg.YestionAPIKey)
	tracker := NewTracker(steam, yestion, cfg)

	// Signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	log.Println("poll loop started")

	// Initial poll
	tracker.Poll()

	for {
		select {
		case <-ticker.C:
			tracker.Poll()
		case sig := <-sigCh:
			log.Printf("received signal %v, shutting down", sig)
			tracker.Shutdown()
			log.Println("=== steam-tracker stopped ===")
			return
		}
	}
}
