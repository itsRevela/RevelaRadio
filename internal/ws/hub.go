// Package ws fans out status updates to connected WebSocket clients.
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Hub broadcasts arbitrary JSON-serialisable payloads to all connected clients.
type Hub struct {
	logger *slog.Logger

	mu      sync.RWMutex
	clients map[*client]struct{}
	last    []byte // most recent broadcast, sent to new clients on connect
}

type client struct {
	conn *websocket.Conn
	send chan []byte
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		logger:  logger,
		clients: make(map[*client]struct{}),
	}
}

// Broadcast serialises payload and sends it to every connected client.
// Slow clients are dropped rather than blocking the hub.
func (h *Hub) Broadcast(payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("ws marshal", "err", err)
		return
	}

	h.mu.Lock()
	h.last = data
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			// Channel full - client is slow. Drop them.
			h.remove(c)
		}
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) add(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// Handler returns an http.Handler that upgrades incoming connections to WebSocket.
func (h *Hub) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			// Caddy + same-origin make CSRF a non-issue here, but be permissive
			// for local-dev cross-origin testing.
			InsecureSkipVerify: true,
		})
		if err != nil {
			h.logger.Warn("ws accept", "err", err)
			return
		}

		c := &client{conn: conn, send: make(chan []byte, 8)}
		h.add(c)
		defer h.remove(c)

		// Send the last broadcast immediately so the UI doesn't sit blank
		// for one poll interval after connecting.
		h.mu.RLock()
		initial := h.last
		h.mu.RUnlock()
		if initial != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			_ = conn.Write(ctx, websocket.MessageText, initial)
			cancel()
		}

		ctx := r.Context()
		// Read pump - we don't expect messages from clients; this exists to
		// detect close.
		go func() {
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					return
				}
			}
		}()

		// Write pump.
		for msg := range c.send {
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				conn.Close(websocket.StatusInternalError, "write failed")
				return
			}
		}

		conn.Close(websocket.StatusNormalClosure, "")
	})
}
