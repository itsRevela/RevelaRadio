// Command server is the VinylStream backend: serves the webapp, proxies
// status from Icecast, and broadcasts live updates over WebSocket.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
	hub := ws.NewHub(logger)
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
	}
	api.New(cache, store, meta).Register(mux)
	mux.Handle("GET /ws", hub.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

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
