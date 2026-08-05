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

	// KafkaBrokers is a comma-separated list of Kafka broker addresses.
	// Env: KAFKA_BROKERS  Default: localhost:9092
	KafkaBrokers string

	// KafkaTopic is the topic job completion notifications are published to.
	// Env: KAFKA_TOPIC  Default: video.completed
	KafkaTopic string

	// KafkaJobsTopic is the topic download job requests are consumed from.
	// Env: KAFKA_JOBS_TOPIC  Default: video.jobs
	KafkaJobsTopic string

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
		KafkaBrokers:   getenv("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopic:     getenv("KAFKA_TOPIC", "video.completed"),
		KafkaJobsTopic: getenv("KAFKA_JOBS_TOPIC", "video.jobs"),
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
