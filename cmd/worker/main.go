package main

import (
	"context"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vng/review-code-agent/internal/config"
	"github.com/vng/review-code-agent/pkg/logger"
)

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = url.User(u.User.Username())
	return u.String()
}

func main() {
	cfg := config.Load()
	log := logger.Default()

	conn, err := amqp.Dial(cfg.AMQP.URL)
	if err != nil {
		log.Error("amqp: failed to connect", "err", err)
		// Continue running so the container stays healthy in dev without RabbitMQ.
	} else {
		defer conn.Close()
		log.Info("amqp: connected", "url", redactURL(cfg.AMQP.URL))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("worker started — waiting for messages")
	<-ctx.Done()
	log.Info("worker shutting down", "signal", ctx.Err())
}
