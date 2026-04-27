package config

import (
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Load reads configuration from environment variables and optional config file.
// Environment variables take precedence over file values.
func Load() *Config {
	v := viper.New()

	// Defaults
	v.SetDefault("http.addr", ":8080")
	v.SetDefault("http.read_timeout", 30*time.Second)
	v.SetDefault("http.write_timeout", 30*time.Second)
	v.SetDefault("db.max_open_conns", 25)
	v.SetDefault("amqp.prefetch", 10)
	v.SetDefault("ai.default_model", "claude-sonnet-4-6")

	// Config file (optional)
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/review-code-agent")
	_ = v.ReadInConfig() // not required; ignore if absent

	// Env override: e.g. HTTP_ADDR → http.addr
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Explicit env bindings for flat env vars from .env.example
	_ = v.BindEnv("http.addr", "HTTP_ADDR")
	_ = v.BindEnv("db.dsn", "DB_DSN")
	_ = v.BindEnv("amqp.url", "AMQP_URL")
	_ = v.BindEnv("ai.anthropic_key", "ANTHROPIC_API_KEY")
	_ = v.BindEnv("ai.openai_key", "OPENAI_API_KEY")
	_ = v.BindEnv("auth.jwt_secret", "JWT_SECRET")
	_ = v.BindEnv("auth.api_key_pepper", "API_KEY_PEPPER")
	_ = v.BindEnv("slack.bot_token", "SLACK_BOT_TOKEN")
	_ = v.BindEnv("code_graph.url", "CODEGRAPH_URL")
	_ = v.BindEnv("code_graph.token", "CODEGRAPH_TOKEN")

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		log.Fatalf("config: unmarshal failed: %v", err)
	}
	return cfg
}
