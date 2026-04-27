package logger

import (
	"log/slog"
	"os"
)

// New returns a JSON-format slog.Logger writing to stdout.
func New(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// Default returns a logger at Info level.
func Default() *slog.Logger {
	return New(slog.LevelInfo)
}
