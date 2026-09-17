package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

func SetupLogging(serviceName string) (io.Closer, error) {
	var logDir string = "log"

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s_%s.log", serviceName, timestamp))

	rotator := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    50,   // In MB, before rotating
		MaxBackups: 10,   // Keep the last 10 rotated files
		MaxAge:     14,   // TTL for the log files
		Compress:   true, // Gzip old logs

	}

	logger := slog.New(slog.NewTextHandler(rotator, nil))
	slog.SetDefault(logger)
	return rotator, nil
}
