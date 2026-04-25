package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New creates a JSON slog.Logger at the given level (debug, info, warn, error).
// Defaults to info if the level string is unrecognised.
func New(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})
	return slog.New(handler)
}
