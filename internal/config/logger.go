package config

import (
	"log/slog"
	"os"
	"strings"
)

type LoggerConfig struct {
	Level string
}

func (l *LoggerConfig) LoadLogger() *slog.Logger {
	var handler slog.Handler

	level := parseLogLevel(l.Level)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug || level == slog.LevelError,
	}

	handler = slog.NewJSONHandler(os.Stdout, opts)

	return slog.New(handler)
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
