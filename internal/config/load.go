package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Load reads configuration from environment variables and an optional config.yaml file.
// Environment variables take precedence over file values.
func Load() *Config {
	v := viper.New()

	// Optional config file (config.yaml in working directory)
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	_ = v.ReadInConfig() // ignore missing file

	// Environment variable binding — replace dots with underscores automatically
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Defaults
	v.SetDefault("http.addr", ":8080")
	v.SetDefault("http.read_timeout", "30s")
	v.SetDefault("http.write_timeout", "30s")
	v.SetDefault("db.max_open_conns", 25)
	v.SetDefault("amqp.prefetch", 10)
	v.SetDefault("ai.default_model", "claude-opus-4-7")

	// Bind explicit env var names that differ from key path convention
	_ = v.BindEnv("http.addr", "HTTP_ADDR")
	_ = v.BindEnv("http.read_timeout", "HTTP_READ_TIMEOUT")
	_ = v.BindEnv("http.write_timeout", "HTTP_WRITE_TIMEOUT")
	_ = v.BindEnv("db.dsn", "DB_DSN")
	_ = v.BindEnv("db.max_open_conns", "DB_MAX_OPEN_CONNS")
	_ = v.BindEnv("amqp.url", "AMQP_URL")
	_ = v.BindEnv("amqp.prefetch", "AMQP_PREFETCH")
	_ = v.BindEnv("redis.addr", "REDIS_ADDR")
	_ = v.BindEnv("ai.anthropic_key", "ANTHROPIC_API_KEY")
	_ = v.BindEnv("ai.openai_key", "OPENAI_API_KEY")
	_ = v.BindEnv("ai.default_model", "AI_DEFAULT_MODEL")
	_ = v.BindEnv("auth.jwt_secret", "JWT_SECRET")
	_ = v.BindEnv("auth.api_key_pepper", "API_KEY_PEPPER")
	_ = v.BindEnv("slack.bot_token", "SLACK_BOT_TOKEN")
	_ = v.BindEnv("codegraph.url", "CODEGRAPH_URL")
	_ = v.BindEnv("codegraph.token", "CODEGRAPH_TOKEN")

	cfg := &Config{}

	cfg.HTTP.Addr = v.GetString("http.addr")
	cfg.HTTP.ReadTimeout = parseDuration(v.GetString("http.read_timeout"), 30*time.Second)
	cfg.HTTP.WriteTimeout = parseDuration(v.GetString("http.write_timeout"), 30*time.Second)

	cfg.DB.DSN = v.GetString("db.dsn")
	cfg.DB.MaxOpenConns = v.GetInt("db.max_open_conns")

	cfg.AMQP.URL = v.GetString("amqp.url")
	cfg.AMQP.Prefetch = v.GetInt("amqp.prefetch")

	cfg.Redis.Addr = v.GetString("redis.addr")

	cfg.AI.AnthropicKey = v.GetString("ai.anthropic_key")
	cfg.AI.OpenAIKey = v.GetString("ai.openai_key")
	cfg.AI.DefaultModel = v.GetString("ai.default_model")

	cfg.Auth.JWTSecret = v.GetString("auth.jwt_secret")
	cfg.Auth.APIKeyPepper = v.GetString("auth.api_key_pepper")

	cfg.Slack.BotToken = v.GetString("slack.bot_token")

	cfg.CodeGraph.URL = v.GetString("codegraph.url")
	cfg.CodeGraph.Token = v.GetString("codegraph.token")

	return cfg
}

// parseDuration parses a duration string, returning the fallback on error.
func parseDuration(s string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return fallback
}
