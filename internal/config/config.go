package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

// Config is the fully validated runtime configuration of the service. Every
// field is documented here rather than only in .example.env, because this is
// the struct people read when they wonder what a setting does.
type Config struct {
	// BotToken is the Telegram bot token from @BotFather.
	BotToken string `env:"BOT_TOKEN" validate:"required"`
	// TelegramAPIURL is the Bot API base URL. The offline tests point it at a
	// fake server.
	TelegramAPIURL string `env:"TELEGRAM_API_URL" validate:"required,url"`
	// AdminIDs are the Telegram user IDs that may always use the bot and may
	// add or remove other users. The bot is never public: at least one admin
	// is required.
	AdminIDs []int64 `env:"ADMIN_IDS" envSeparator:"," validate:"required,min=1,dive,gt=0"`

	// DBPath is the SQLite file holding users and the allow list. It belongs
	// on persistent storage.
	DBPath string `env:"DB_PATH" validate:"required"`

	// RedisURL locates the cache, as redis://host:port/db.
	RedisURL string `env:"REDIS_URL" validate:"required,url"`
	// CacheTTL is how long a response of the schedule site is reused before
	// the bot asks the site again.
	CacheTTL time.Duration `env:"CACHE_TTL" validate:"required,min=1m"`

	// MKRBaseURL is the schedule site. MKRTimeout bounds one request to it.
	MKRBaseURL string        `env:"MKR_BASE_URL" validate:"required,url"`
	MKRTimeout time.Duration `env:"MKR_TIMEOUT"  validate:"required,min=1s"`

	HTTPAddr string `env:"HTTP_ADDR" validate:"required"`
	LogLevel string `env:"LOG_LEVEL" validate:"required,oneof=debug info warn error"`
}

// Load reads .env when present, overlays the process environment and validates
// the result. Any problem is fatal for the caller: a bot that starts with a
// half-valid config fails later and more confusingly, in front of users.
func Load() (Config, error) {
	_ = godotenv.Load()

	// Defaults are set before parsing, so the environment only has to carry
	// what actually differs from them.
	cfg := Config{
		TelegramAPIURL: "https://api.telegram.org",
		DBPath:         "data/knubaschedulebot.db",
		RedisURL:       "redis://localhost:6379/0",
		CacheTTL:       time.Hour,
		MKRBaseURL:     "https://mkr.knuba.edu.ua",
		MKRTimeout:     20 * time.Second,
		HTTPAddr:       ":8080",
		LogLevel:       "info",
	}

	err := env.Parse(&cfg)
	if err != nil {
		return Config{}, fmt.Errorf("parse env: %w", err)
	}

	err = validator.New().Struct(cfg)
	if err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}
