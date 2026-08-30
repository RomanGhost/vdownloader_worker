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

	// RabbitURL is the RabbitMQ connection URL. The worker consumes job
	// requests from the "video.jobs" queue and publishes completions to
	// "video.completed" (queue names are constants in internal/mq).
	// Env: RABBITMQ_URL  Default: amqp://guest:guest@localhost:5672/
	RabbitURL string

	// OutDir is the directory where downloaded files are stored.
	// Env: OUT_DIR  Default: ./downloads
	OutDir string

	// FileServerAddr is the address the HTTP server listens on (file server + API).
	// Env: FILE_SERVER_ADDR  Default: :8080
	FileServerAddr string
}

// Load reads .env (if present), then environment variables, then falls back to defaults.
// Variables already set in the environment take precedence over .env values.
func Load() Config {
	// Overload=false: existing env vars are NOT overwritten by .env.
	_ = godotenv.Load()

	return Config{
		DBPath:         getenv("DB_PATH", "downloads.db"),
		RabbitURL:      getenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		OutDir:         getenv("OUT_DIR", "./downloads"),
		FileServerAddr: getenv("FILE_SERVER_ADDR", ":8080"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
