// Package config loads and validates all environment variables required by the bot.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration.
type Config struct {
	// Telegram userbot credentials
	TelegramAppID   int
	TelegramAppHash string
	TelegramPhone   string
	// Channel to listen on (numeric ID, e.g. -1001234567890)
	TelegramChannelID int64

	// Anthropic (Claude) credentials
	AnthropicAPIKey string

	// OpenAlgo trading platform
	OpenAlgoURL    string
	OpenAlgoAPIKey string

	// Session storage path for gotd/td
	SessionFilePath string

	// Feature flags
	DryRun bool

	// Fixed trading parameters
	TradeLotSize     int
	OpeningSLPercent float64
}

// Load reads .env (if present) and then environment variables, returning a validated Config.
func Load() (*Config, error) {
	// Best-effort .env load — not fatal if file is missing (production envs inject vars directly).
	if err := godotenv.Load(); err != nil {
		slog.Warn("no .env file found, using environment variables only")
	}

	cfg := &Config{}

	// --- Telegram ---
	appIDStr := requireEnv("TELEGRAM_APP_ID")
	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		return nil, fmt.Errorf("config: TELEGRAM_APP_ID must be an integer: %w", err)
	}
	cfg.TelegramAppID = appID
	cfg.TelegramAppHash = requireEnv("TELEGRAM_APP_API_HASH")
	cfg.TelegramPhone = requireEnv("TELEGRAM_PHONE")

	chanIDStr := requireEnv("TELEGRAM_CHANNEL_ID")
	chanID, err := strconv.ParseInt(chanIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("config: TELEGRAM_CHANNEL_ID must be an integer: %w", err)
	}
	cfg.TelegramChannelID = chanID

	// --- Anthropic ---
	cfg.AnthropicAPIKey = requireEnv("ANTHROPIC_API_KEY")

	// --- OpenAlgo ---
	cfg.OpenAlgoURL = envWithDefault("OPENALGO_URL", "http://127.0.0.1:5000")
	cfg.OpenAlgoAPIKey = requireEnv("OPENALGO_API_KEY")

	// --- Session ---
	cfg.SessionFilePath = envWithDefault("SESSION_FILE_PATH", "session.json")

	// --- Feature flags ---
	cfg.DryRun = envBool("DRY_RUN", true) // default true — safe during development

	// --- Trading parameters ---
	cfg.TradeLotSize = envInt("TRADE_LOT_SIZE", 1)             // default 1 lot
	cfg.OpeningSLPercent = envFloat("OPENING_SL_PERCENT", 15.0) // default 15%

	return cfg, nil
}

// requireEnv returns the env var value or panics with a descriptive error.
func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("config: required environment variable %q is not set", key))
	}
	return v
}

// envWithDefault returns the env var value or a fallback.
func envWithDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envBool parses a boolean env var, returning the default on parse failure.
func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		slog.Warn("config: invalid bool value, using default", "key", key, "default", def)
		return def
	}
	return b
}

// envInt parses an integer env var, returning the default on parse failure.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("config: invalid int value, using default", "key", key, "default", def)
		return def
	}
	return i
}

// envFloat parses a float env var, returning the default on parse failure.
func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		slog.Warn("config: invalid float value, using default", "key", key, "default", def)
		return def
	}
	return f
}
