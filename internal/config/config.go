package config

import (
	"fmt"
	"os"
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
