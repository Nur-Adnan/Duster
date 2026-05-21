package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var Logger *slog.Logger

func init() {
	// Initialize standard structured logger to Stderr with level Info
	Logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// LogDestructiveOperation writes dangerous actions to the persistent operations log file.
func LogDestructiveOperation(command, action, target string, size int64, success bool) {
	if os.Getenv("DU_NO_OPLOG") == "1" {
		return
	}

	var logDir string
	if runtime.GOOS == "windows" {
		logDir = os.Getenv("LOCALAPPDATA")
		if logDir == "" {
			logDir = os.Getenv("USERPROFILE")
		}
		if logDir != "" {
			logDir = filepath.Join(logDir, "Duster")
		}
	} else {
		logDir = filepath.Clean("./")
	}

	if logDir == "" {
		logDir = filepath.Clean("./")
	}

	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "operations.log")

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		Logger.Error("failed to open operations log", "error", err)
		return
	}
	defer f.Close()

	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	entry := fmt.Sprintf("%s | Command: %s | Action: %s | Target: %s | Size: %d bytes | Status: %s\n",
		timestamp, command, action, target, size, status)
	_, _ = f.WriteString(entry)
}
