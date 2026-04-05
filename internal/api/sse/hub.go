package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type Client struct {
	ch chan []byte
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string][]*Client
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string][]*Client)}
}

func (h *Hub) Register(sessionID string) *Client {
	c := &Client{ch: make(chan []byte, 256)}
	h.mu.Lock()
	h.clients[sessionID] = append(h.clients[sessionID], c)
	h.mu.Unlock()
	return c
}

func (h *Hub) Unregister(sessionID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := h.clients[sessionID]
	filtered := clients[:0]
	for _, cl := range clients {
		if cl != c {
			filtered = append(filtered, cl)
		}
	}
	h.clients[sessionID] = filtered
	close(c.ch)
}

func (h *Hub) Broadcast(sessionID string, event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	clients := h.clients[sessionID]
	h.mu.RUnlock()
	for _, c := range clients {
		select {
		case c.ch <- data:
		default:
		}
	}
}

func (h *Hub) ServeClient(w http.ResponseWriter, r *http.Request, sessionID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	client := h.Register(sessionID)
	defer h.Unregister(sessionID, client)

	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-client.ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
