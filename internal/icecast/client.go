// Package icecast polls an Icecast 2 admin endpoint for live stream status.
package icecast

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Status is the parsed snapshot we expose to the rest of the app.
type Status struct {
	Online        bool      `json:"online"`
	Listeners     int       `json:"listeners"`
	PeakListeners int       `json:"peak_listeners"`
	StreamTitle   string    `json:"stream_title,omitempty"`
	Artist        string    `json:"artist,omitempty"`
	Genre         string    `json:"genre,omitempty"`
	Bitrate       int       `json:"bitrate,omitempty"`
	ContentType   string    `json:"content_type,omitempty"`
	// SampleRate in Hz, parsed from the source's `audio_info` field.
	SampleRate int `json:"sample_rate,omitempty"`
	// Channels (1 = mono, 2 = stereo, etc.).
	Channels  int       `json:"channels,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	// UptimeSeconds is convenience derived from StartedAt at fetch time.
	UptimeSeconds int       `json:"uptime_seconds,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
}

// Client polls Icecast's admin XML endpoint.
type Client struct {
	baseURL   string
	mount     string
	adminUser string
	adminPass string
	http      *http.Client
}

func NewClient(baseURL, mount, user, pass string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		mount:     mount,
		adminUser: user,
		adminPass: pass,
		http: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// adminStats is the XML structure returned by /admin/stats.
type adminStats struct {
	XMLName xml.Name `xml:"icestats"`
	Sources []struct {
		Mount         string `xml:"mount,attr"`
		Listeners     int    `xml:"listeners"`
		PeakListeners int    `xml:"listener_peak"`
		Title         string `xml:"title"`
		Artist        string `xml:"artist"`
		Genre         string `xml:"genre"`
		Bitrate       int    `xml:"bitrate"`
		ContentType   string `xml:"server_type"`
		StreamStart   string `xml:"stream_start_iso8601"`
		// audio_info is a string like "channels=2;samplerate=48000;bitrate=quality"
		// or "ice-channels=2;ice-samplerate=48000" depending on the source.
		AudioInfo  string `xml:"audio_info"`
		Channels   int    `xml:"channels"`
		SampleRate int    `xml:"samplerate"`
	} `xml:"source"`
}

// Fetch returns the current status for the configured mount.
func (c *Client) Fetch(ctx context.Context) (Status, error) {
	now := time.Now().UTC()
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/admin/stats", nil)
	if err != nil {
		return Status{ObservedAt: now}, err
	}
	req.SetBasicAuth(c.adminUser, c.adminPass)

	resp, err := c.http.Do(req)
	if err != nil {
		return Status{ObservedAt: now}, fmt.Errorf("fetch icecast stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Status{ObservedAt: now}, fmt.Errorf("icecast stats: status %d", resp.StatusCode)
	}

	var stats adminStats
	if err := xml.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return Status{ObservedAt: now}, fmt.Errorf("decode icecast stats: %w", err)
	}

	status := Status{ObservedAt: now}
	for _, s := range stats.Sources {
		if s.Mount != c.mount {
			continue
		}
		status.Online = true
		status.Listeners = s.Listeners
		status.PeakListeners = s.PeakListeners
		status.StreamTitle = s.Title
		status.Artist = s.Artist
		status.Genre = s.Genre
		status.Bitrate = s.Bitrate
		status.ContentType = s.ContentType

		// Prefer explicit <channels>/<samplerate> elements when present,
		// fall back to parsing the audio_info semicolon-separated string.
		status.Channels = s.Channels
		status.SampleRate = s.SampleRate
		if status.SampleRate == 0 || status.Channels == 0 {
			ch, sr := parseAudioInfo(s.AudioInfo)
			if status.Channels == 0 {
				status.Channels = ch
			}
			if status.SampleRate == 0 {
				status.SampleRate = sr
			}
		}

		if t, ok := parseStreamStart(s.StreamStart); ok {
			status.StartedAt = t
			if secs := int(now.Sub(t).Seconds()); secs > 0 {
				status.UptimeSeconds = secs
			}
		}
		break
	}
	return status, nil
}

// parseStreamStart accepts the timestamp formats Icecast actually emits for
// `<stream_start_iso8601>`. The version shipping with Debian Bookworm
// (Icecast 2.4.4) writes `+0000` without the colon, which time.RFC3339
// rejects. Try the looser format first; fall back to canonical RFC3339 for
// future versions / non-Debian builds.
var streamStartFormats = []string{
	"2006-01-02T15:04:05-0700",
	time.RFC3339,
	time.RFC3339Nano,
}

func parseStreamStart(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range streamStartFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseAudioInfo extracts channels and sample rate from Icecast's audio_info
// string, which uses semicolon-separated key=value pairs. Accepts both the
// `ice-` prefixed variant and the bare keys.
func parseAudioInfo(s string) (channels, sampleRate int) {
	if s == "" {
		return 0, 0
	}
	for _, part := range strings.Split(s, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimPrefix(strings.ToLower(kv[0]), "ice-")
		val, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			continue
		}
		switch key {
		case "channels":
			channels = val
		case "samplerate":
			sampleRate = val
		}
	}
	return channels, sampleRate
}
