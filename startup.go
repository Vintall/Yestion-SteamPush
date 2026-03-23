package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const shortcutName = "YestionSteamTracker.lnk"

func startupDir() (string, error) {
	// Windows startup folder: %APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return "", fmt.Errorf("APPDATA not set")
	}
	return filepath.Join(appdata, "Microsoft", "Windows", "Start Menu", "Programs", "Startup"), nil
}

func installStartup() error {
	dir, err := startupDir()
	if err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get exe path: %w", err)
	}
	// Resolve symlinks to get the real path
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolve exe path: %w", err)
	}

	shortcutPath := filepath.Join(dir, shortcutName)

	// Escape single quotes for PowerShell single-quoted strings (' -> '')
	psEscape := func(s string) string { return strings.ReplaceAll(s, "'", "''") }

	// Use PowerShell to create a .lnk shortcut
	psScript := fmt.Sprintf(`
		$ws = New-Object -ComObject WScript.Shell
		$sc = $ws.CreateShortcut('%s')
		$sc.TargetPath = '%s'
		$sc.WorkingDirectory = '%s'
		$sc.Description = 'Yestion Steam Tracker'
		$sc.Save()
	`, psEscape(shortcutPath), psEscape(exePath), psEscape(filepath.Dir(exePath)))

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create shortcut: %v\n%s", err, out)
	}

	return nil
}

func uninstallStartup() error {
	dir, err := startupDir()
	if err != nil {
		return err
	}

	shortcutPath := filepath.Join(dir, shortcutName)
	if err := os.Remove(shortcutPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already removed
		}
		return fmt.Errorf("remove shortcut: %w", err)
	}
	return nil
}
