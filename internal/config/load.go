package config

import (
	"time"

	"github.com/spf13/viper"
)

// Load reads configuration from environment variables (and optional config file).
// Environment variables take precedence over file values.
func Load() *Config {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AutomaticEnv()

	setDefaults(v)
	_ = v.ReadInConfig() // config file is optional

	return &Config{
		HTTP: struct {
			Addr         string
			ReadTimeout  time.Duration
			WriteTimeout time.Duration
		}{
			Addr:         v.GetString("HTTP_ADDR"),
			ReadTimeout:  v.GetDuration("HTTP_READ_TIMEOUT"),
			WriteTimeout: v.GetDuration("HTTP_WRITE_TIMEOUT"),
		},
		DB: struct {
			DSN          string
			MaxOpenConns int
		}{
			DSN:          v.GetString("DB_DSN"),
			MaxOpenConns: v.GetInt("DB_MAX_OPEN_CONNS"),
		},
		AMQP: struct {
			URL      string
			Prefetch int
		}{
			URL:      v.GetString("AMQP_URL"),
			Prefetch: v.GetInt("AMQP_PREFETCH"),
		},
		AI: struct {
			AnthropicKey string
			OpenAIKey    string
			DefaultModel string
		}{
			AnthropicKey: v.GetString("ANTHROPIC_API_KEY"),
			OpenAIKey:    v.GetString("OPENAI_API_KEY"),
			DefaultModel: v.GetString("AI_DEFAULT_MODEL"),
		},
		Auth: struct {
			JWTSecret    string
			APIKeyPepper string
		}{
			JWTSecret:    v.GetString("JWT_SECRET"),
			APIKeyPepper: v.GetString("API_KEY_PEPPER"),
		},
		Slack: struct {
			BotToken string
		}{
			BotToken: v.GetString("SLACK_BOT_TOKEN"),
		},
		CodeGraph: struct {
			URL   string
			Token string
		}{
			URL:   v.GetString("CODEGRAPH_URL"),
			Token: v.GetString("CODEGRAPH_TOKEN"),
		},
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("HTTP_ADDR", ":8080")
	v.SetDefault("HTTP_READ_TIMEOUT", "30s")
	v.SetDefault("HTTP_WRITE_TIMEOUT", "30s")
	v.SetDefault("DB_MAX_OPEN_CONNS", 25)
	v.SetDefault("AMQP_PREFETCH", 10)
	v.SetDefault("AI_DEFAULT_MODEL", "claude-sonnet-4-6")
}
