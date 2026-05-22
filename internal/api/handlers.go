// Package api exposes the REST endpoints consumed by the frontend.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/securitygate/vinylstream/internal/history"
	"github.com/securitygate/vinylstream/internal/icecast"
)

// StatusSource is anything that can return the latest Icecast status snapshot.
type StatusSource interface {
	Latest() icecast.Status
}

// StreamMeta is the static info served alongside live status.
type StreamMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Genre       string `json:"genre"`
	MountPath   string `json:"mount_path"`
}

type Handlers struct {
	source StatusSource
	store  *history.Store
	meta   StreamMeta
}

func New(source StatusSource, store *history.Store, meta StreamMeta) *Handlers {
	return &Handlers{source: source, store: store, meta: meta}
}

// Register wires the handlers onto the given mux under /api/.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/status", h.getStatus)
	mux.HandleFunc("GET /api/history", h.getHistory)
	mux.HandleFunc("GET /api/meta", h.getMeta)
}

type statusResponse struct {
	Stream    icecast.Status `json:"stream"`
	Meta      StreamMeta     `json:"meta"`
}

func (h *Handlers) getStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{
		Stream: h.source.Latest(),
		Meta:   h.meta,
	})
}

func (h *Handlers) getMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.meta)
}

type historyResponse struct {
	Window    string               `json:"window"`
	Snapshots []history.Snapshot   `json:"snapshots"`
	Peak      int                  `json:"peak"`
}

// allowedHistoryWindows constrains the range a caller can request, so a
// listener can't ask us to scan years of snapshots at once.
var allowedHistoryWindows = map[string]time.Duration{
	"1h":  1 * time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

func (h *Handlers) getHistory(w http.ResponseWriter, r *http.Request) {
	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "24h"
	}
	window, ok := allowedHistoryWindows[rangeStr]
	if !ok {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	snaps, err := h.store.Recent(ctx, window)
	if err != nil {
		http.Error(w, "failed to query history", http.StatusInternalServerError)
		return
	}
	peak, err := h.store.PeakSince(ctx, time.Now().Add(-window))
	if err != nil {
		http.Error(w, "failed to query peak", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, historyResponse{
		Window:    rangeStr,
		Snapshots: snaps,
		Peak:      peak,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// LatestCache is a small thread-safe holder used by main to share the most
// recent Icecast status between the poller, the API handlers, and the WS hub.
type LatestCache struct {
	mu     sync.RWMutex
	status icecast.Status
}

func (c *LatestCache) Set(s icecast.Status) {
	c.mu.Lock()
	c.status = s
	c.mu.Unlock()
}

func (c *LatestCache) Latest() icecast.Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}
