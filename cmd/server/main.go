// Command server is the VinylStream backend: serves the webapp, proxies
// status from Icecast, and broadcasts live updates over WebSocket.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/securitygate/vinylstream/internal/api"
	"github.com/securitygate/vinylstream/internal/config"
	"github.com/securitygate/vinylstream/internal/history"
	"github.com/securitygate/vinylstream/internal/icecast"
	"github.com/securitygate/vinylstream/internal/ws"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store, err := history.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	cache := &api.LatestCache{}
	hub := ws.NewHub(logger, cfg.AllowedOrigins)
	client := icecast.NewClient(cfg.IcecastURL, cfg.IcecastMount, cfg.IcecastAdminUser, cfg.IcecastAdminPass)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Poll Icecast on a ticker. Each tick: fetch status, persist a snapshot,
	// update the shared cache, broadcast over WS.
	go pollLoop(ctx, logger, client, store, cache, hub, cfg.IcecastPollEvery)

	mux := http.NewServeMux()
	meta := api.StreamMeta{
		Name:        cfg.StreamName,
		Description: cfg.StreamDescription,
		Genre:       cfg.StreamGenre,
		MountPath:   cfg.IcecastMount,
		BitDepth:    cfg.StreamBitDepth,
	}
	api.New(cache, store, meta).Register(mux)
	mux.Handle("GET /ws", hub.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Reverse-proxy the audio mount to Icecast so the page can hit it
	// same-origin. Lets the upstream reverse proxy (NPM, Caddy, nginx)
	// forward everything to a single backend.
	flacProxy, err := newIcecastProxy(cfg.IcecastURL, logger)
	if err != nil {
		return err
	}
	mux.Handle(cfg.IcecastMount, flacProxy)
	mux.Handle(cfg.IcecastMount+".m3u", flacProxy)
	mux.Handle(cfg.IcecastMount+".xspf", flacProxy)

	// Static files (the SPA-less frontend).
	mux.Handle("/", http.FileServer(http.Dir("web")))

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		_ = srv.Shutdown(shutCtx)
	}()

	logger.Info("starting", "listen", cfg.Listen, "icecast", cfg.IcecastURL, "mount", cfg.IcecastMount)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type broadcastPayload struct {
	Stream icecast.Status `json:"stream"`
}

// newIcecastProxy returns a reverse proxy that forwards listener requests
// (audio mount, playlist files) to Icecast without buffering, so listeners
// get audio chunks as fast as they arrive.
func newIcecastProxy(icecastURL string, logger *slog.Logger) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(icecastURL)
	if err != nil {
		return nil, fmt.Errorf("parse icecast url %q: %w", icecastURL, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// FlushInterval = -1 means "flush after every Write". Chunked audio
	// becomes available to the listener immediately instead of waiting for
	// the default buffer to fill.
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Warn("icecast proxy error", "path", r.URL.Path, "err", err)
		http.Error(w, "stream unavailable", http.StatusBadGateway)
	}
	return proxy, nil
}

func pollLoop(
	ctx context.Context,
	logger *slog.Logger,
	client *icecast.Client,
	store *history.Store,
	cache *api.LatestCache,
	hub *ws.Hub,
	every time.Duration,
) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	tick := func() {
		fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		status, err := client.Fetch(fetchCtx)
		if err != nil {
			logger.Warn("icecast fetch failed", "err", err)
			// Even on failure, record an "offline" snapshot so the history
			// graph reflects reality.
			status = icecast.Status{ObservedAt: time.Now().UTC()}
		}

		cache.Set(status)

		if err := store.Insert(fetchCtx, history.Snapshot{
			ObservedAt: status.ObservedAt,
			Listeners:  status.Listeners,
			Online:     status.Online,
		}); err != nil {
			logger.Warn("history insert failed", "err", err)
		}

		hub.Broadcast(broadcastPayload{Stream: status})
	}

	tick() // immediate first poll so /api/status isn't empty on boot
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}
