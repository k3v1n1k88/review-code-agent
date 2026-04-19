package config

import "time"

type Config struct {
	HTTP struct {
		Addr         string
		ReadTimeout  time.Duration
		WriteTimeout time.Duration
	}
	DB struct {
		DSN         string
		MaxOpenConns int
	}
	AMQP struct {
		URL      string
		Prefetch int
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
	Redis struct {
		URL string
	}
}
