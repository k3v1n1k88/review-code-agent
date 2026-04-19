package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vng/review-code-agent/internal/config"
	"github.com/vng/review-code-agent/pkg/logger"
)

func main() {
	cfg := config.Load()
	log := logger.New(slog.LevelInfo)
	_ = cfg

	log.Info("mcp server starting")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("mcp server shutting down")
}
