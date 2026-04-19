package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

func Load() *Config {
	v := viper.New()

	v.SetDefault("http.addr", ":8080")
	v.SetDefault("http.readtimeout", "15s")
	v.SetDefault("http.writetimeout", "15s")
	v.SetDefault("db.maxopenconns", 25)
	v.SetDefault("amqp.prefetch", 5)
	v.SetDefault("ai.defaultmodel", "claude-sonnet-4-6")
	v.SetDefault("redis.url", "redis://localhost:6379")

	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	_ = v.ReadInConfig()

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	cfg := &Config{}
	cfg.HTTP.Addr = v.GetString("http.addr")
	cfg.HTTP.ReadTimeout = mustDuration(v.GetString("http.readtimeout"))
	cfg.HTTP.WriteTimeout = mustDuration(v.GetString("http.writetimeout"))
	cfg.DB.DSN = v.GetString("db.dsn")
	cfg.DB.MaxOpenConns = v.GetInt("db.maxopenconns")
	cfg.AMQP.URL = v.GetString("amqp.url")
	cfg.AMQP.Prefetch = v.GetInt("amqp.prefetch")
	cfg.AI.AnthropicKey = v.GetString("anthropic_api_key")
	cfg.AI.OpenAIKey = v.GetString("openai_api_key")
	cfg.AI.DefaultModel = v.GetString("ai.defaultmodel")
	cfg.Auth.JWTSecret = v.GetString("jwt_secret")
	cfg.Auth.APIKeyPepper = v.GetString("api_key_pepper")
	cfg.Slack.BotToken = v.GetString("slack_bot_token")
	cfg.CodeGraph.URL = v.GetString("codegraph_url")
	cfg.CodeGraph.Token = v.GetString("codegraph_token")
	cfg.Redis.URL = v.GetString("redis_url")
	return cfg
}

func mustDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 15 * time.Second
	}
	return d
}
