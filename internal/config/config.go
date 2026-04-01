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

	// AMQPUrl is the RabbitMQ connection string.
	// Env: AMQP_URL  Default: amqp://guest:guest@localhost:5672/
	AMQPUrl string

	// OutDir is the directory where downloaded files are stored.
	// Env: OUT_DIR  Default: ./downloads
	OutDir string

	// FileServerAddr is the address the HTTP file server listens on.
	// Env: FILE_SERVER_ADDR  Default: :8080
	FileServerAddr string

	// CookiesFromBrowser passes --cookies-from-browser to yt-dlp.
	// Set to a browser name such as "chrome", "firefox", or "safari".
	// Env: COOKIES_FROM_BROWSER  Default: ""
	CookiesFromBrowser string

	// CookiesFile passes --cookies to yt-dlp.
	// Set to the path of a Netscape-format cookies.txt file.
	// Env: COOKIES_FILE  Default: ""
	CookiesFile string
}

// Load reads .env (if present), then environment variables, then falls back to defaults.
// Variables already set in the environment take precedence over .env values.
func Load() Config {
	// Overload=false: existing env vars are NOT overwritten by .env.
	_ = godotenv.Load()

	return Config{
		DBPath:             getenv("DB_PATH", "downloads.db"),
		AMQPUrl:            getenv("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		OutDir:             getenv("OUT_DIR", "./downloads"),
		FileServerAddr:     getenv("FILE_SERVER_ADDR", ":8080"),
		CookiesFromBrowser: getenv("COOKIES_FROM_BROWSER", ""),
		CookiesFile:        getenv("COOKIES_FILE", ""),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
