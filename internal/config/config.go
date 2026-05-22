package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Listen            string
	DBPath            string
	IcecastURL        string
	IcecastMount      string
	IcecastAdminUser  string
	IcecastAdminPass  string
	IcecastPollEvery  time.Duration
	StreamName        string
	StreamDescription string
	StreamGenre       string
	// AllowedOrigins lists additional WebSocket origin hostnames to accept
	// (comma-separated in VINYLSTREAM_ALLOWED_ORIGINS). Same-origin requests
	// are always accepted by the WS library, so this is only needed if the
	// player ever gets embedded on a different host.
	AllowedOrigins []string
}

func Load() (*Config, error) {
	pollEvery, err := time.ParseDuration(getenv("ICECAST_POLL_INTERVAL", "5s"))
	if err != nil {
		return nil, fmt.Errorf("ICECAST_POLL_INTERVAL: %w", err)
	}

	c := &Config{
		Listen:            getenv("VINYLSTREAM_LISTEN", ":8080"),
		DBPath:            getenv("VINYLSTREAM_DB", "./vinylstream.db"),
		IcecastURL:        getenv("ICECAST_URL", "http://localhost:8000"),
		IcecastMount:      getenv("ICECAST_MOUNT", "/live.flac"),
		IcecastAdminUser:  getenv("ICECAST_ADMIN_USER", "admin"),
		IcecastAdminPass:  os.Getenv("ICECAST_ADMIN_PASSWORD"),
		IcecastPollEvery:  pollEvery,
		StreamName:        getenv("STREAM_NAME", "VinylStream"),
		StreamDescription: getenv("STREAM_DESCRIPTION", "Lossless audio livestream"),
		StreamGenre:       getenv("STREAM_GENRE", "Various"),
		AllowedOrigins:    parseCSV(os.Getenv("VINYLSTREAM_ALLOWED_ORIGINS")),
	}

	if c.IcecastAdminPass == "" {
		return nil, fmt.Errorf("ICECAST_ADMIN_PASSWORD is required")
	}

	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseCSV splits a comma-separated list, trimming whitespace and dropping
// empty entries. Returns nil for an empty input.
func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
