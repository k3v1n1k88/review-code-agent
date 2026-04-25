package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/vng/review-code-agent/pkg/logger"
)

func main() {
	log := logger.New("info")

	// MCP protocol implementation is added in Phase 08.
	// This stub holds the process alive for Docker Compose / systemd compatibility.
	log.Info("mcp server started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("mcp server stopped")
}
