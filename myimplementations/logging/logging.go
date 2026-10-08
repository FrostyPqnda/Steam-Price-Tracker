package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

func SetupLogging(serviceName string) (io.Closer, error) {
	const logDir = "log"

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, serviceName+".log"),
		MaxSize:    50,   // MB before rotating
		MaxBackups: 10,   // keep the last 10 rotated files
		MaxAge:     14,   // days
		Compress:   true, // gzip rotated files
	}

	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(rotator, &slog.HandlerOptions{Level: level})))
	return rotator, nil
}
