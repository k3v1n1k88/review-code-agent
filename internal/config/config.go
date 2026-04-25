package config

import "time"

// Config holds all application configuration loaded from env vars or config file.
type Config struct {
	HTTP struct {
		Addr         string
		ReadTimeout  time.Duration
		WriteTimeout time.Duration
	}
	DB struct {
		DSN          string
		MaxOpenConns int
	}
	AMQP struct {
		URL      string
		Prefetch int
	}
	Redis struct {
		Addr string
	}
	AI struct {
		AnthropicKey string
		OpenAIKey    string
		DefaultModel string
	}
	Auth struct {
		JWTSecret    string
		APIKeyPepper string
	}
	Slack struct {
		BotToken string
	}
	CodeGraph struct {
		URL   string
		Token string
	}
}
