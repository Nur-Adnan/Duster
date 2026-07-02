package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const maxLogSize = 10 * 1024 * 1024 // 10 MB

var Logger *slog.Logger

func init() {
	Logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// LogDestructiveOperation writes dangerous actions to the persistent operations log file.
func LogDestructiveOperation(command, action, target string, size int64, success bool) {
	if os.Getenv("DU_NO_OPLOG") == "1" {
		return
	}

	logDir := os.Getenv("LOCALAPPDATA")
	if logDir == "" {
		logDir = os.Getenv("USERPROFILE")
	}
	if logDir != "" {
		logDir = filepath.Join(logDir, "Duster")
	}

	if logDir == "" {
		logDir = filepath.Clean("./")
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		Logger.Error("failed to create log directory", "error", err)
		return
	}
	logPath := filepath.Join(logDir, "operations.log")

	rotateIfNeeded(logPath)

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
	if _, err := f.WriteString(entry); err != nil {
		Logger.Error("failed to write operations log entry", "error", err)
	}
}

func rotateIfNeeded(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil || info.Size() < maxLogSize {
		return
	}
	prev := logPath + ".old"
	_ = os.Remove(prev)
	_ = os.Rename(logPath, prev)
}
