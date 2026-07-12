// Package config loads application settings from environment variables.
// Each field has a hardcoded fallback so the service works out of the box
// without any configuration.
//
// Priority (highest → lowest): CLI flag > environment variable > .env file > default.
package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds all tunable parameters.
type Config struct {
	// DBPath is the path to the SQLite database file.
	// Env: DB_PATH  Default: downloads.db
	DBPath string

	// AMQPURL is the RabbitMQ connection string.
	// Env: AMQP_URL  Default: amqp://guest:guest@localhost:5672/
	AMQPURL string

	// OutDir is the directory where downloaded files are stored.
	// Env: OUT_DIR  Default: ./downloads
	OutDir string

	// FileServerAddr is the address the HTTP server listens on (file server + API).
	// Env: FILE_SERVER_ADDR  Default: :8080
	FileServerAddr string

	// WebhookURL is an optional HTTP endpoint that receives a POST on every
	// download completion. Leave empty to disable.
	// Env: WEBHOOK_URL  Default: (disabled)
	WebhookURL string
}

// Load reads .env (if present), then environment variables, then falls back to defaults.
// Variables already set in the environment take precedence over .env values.
func Load() Config {
	// Overload=false: existing env vars are NOT overwritten by .env.
	_ = godotenv.Load()

	return Config{
		DBPath:         getenv("DB_PATH", "downloads.db"),
		AMQPURL:        getenv("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		OutDir:         getenv("OUT_DIR", "./downloads"),
		FileServerAddr: getenv("FILE_SERVER_ADDR", ":8080"),
		WebhookURL:     getenv("WEBHOOK_URL", ""),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
