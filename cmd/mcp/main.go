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
	log := logger.Default()

	log.Info("mcp server starting", slog.String("model", cfg.AI.DefaultModel))

	// Phase 08 will wire up stdio/SSE MCP transport here.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("mcp server stopped")
}
