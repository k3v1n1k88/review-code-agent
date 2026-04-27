package config

import "time"

// Config holds all application configuration loaded from env/file.
type Config struct {
	HTTP struct {
		Addr         string        `mapstructure:"addr"`
		ReadTimeout  time.Duration `mapstructure:"read_timeout"`
		WriteTimeout time.Duration `mapstructure:"write_timeout"`
	} `mapstructure:"http"`

	DB struct {
		DSN          string `mapstructure:"dsn"`
		MaxOpenConns int    `mapstructure:"max_open_conns"`
	} `mapstructure:"db"`

	AMQP struct {
		URL      string `mapstructure:"url"`
		Prefetch int    `mapstructure:"prefetch"`
	} `mapstructure:"amqp"`

	AI struct {
		AnthropicKey string `mapstructure:"anthropic_key"`
		OpenAIKey    string `mapstructure:"openai_key"`
		DefaultModel string `mapstructure:"default_model"`
	} `mapstructure:"ai"`

	Auth struct {
		JWTSecret    string `mapstructure:"jwt_secret"`
		APIKeyPepper string `mapstructure:"api_key_pepper"`
	} `mapstructure:"auth"`

	Slack struct {
		BotToken string `mapstructure:"bot_token"`
	} `mapstructure:"slack"`

	CodeGraph struct {
		URL   string `mapstructure:"url"`
		Token string `mapstructure:"token"`
	} `mapstructure:"code_graph"`
}
