package main

import (
	"net/url"
	"os"
	"os/signal"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vng/review-code-agent/internal/config"
	"github.com/vng/review-code-agent/pkg/logger"
)

// redactURL strips credentials from a connection URL for safe logging.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparseable)"
	}
	u.User = nil
	return u.String()
}

func main() {
	cfg := config.Load()
	log := logger.New("info")

	// Redact credentials from URL before logging.
	log.Info("worker starting", "amqp_host", redactURL(cfg.AMQP.URL))

	// Establish AMQP connection (stub — actual consumers added in Phase 09).
	conn, err := amqp.Dial(cfg.AMQP.URL)
	if err != nil {
		// Log and continue — broker may not be running in local dev without compose.
		log.Warn("amqp connection failed", "err", err)
	} else {
		log.Info("worker started, connected to amqp")
		defer conn.Close()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("worker stopped")
}
