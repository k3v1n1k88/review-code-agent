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

	log.Info("worker starting", slog.String("amqp", cfg.AMQP.URL))

	// Phase 09 will wire up the RabbitMQ consumer here.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("worker stopped")
}
