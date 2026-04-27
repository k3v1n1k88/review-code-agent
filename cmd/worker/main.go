package main

import (
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/vng/review-code-agent/internal/config"
	"github.com/vng/review-code-agent/pkg/logger"
)

func main() {
	cfg := config.Load()
	log := logger.Default()

	amqpHost := cfg.AMQP.URL
	if u, err := url.Parse(cfg.AMQP.URL); err == nil {
		u.User = nil
		amqpHost = u.String()
	}
	log.Info("worker starting", slog.String("amqp_host", amqpHost))

	// Phase 09 will wire up the RabbitMQ consumer here.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("worker stopped")
}
