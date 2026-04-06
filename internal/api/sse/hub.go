package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type SSEEvent struct {
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"ts"`
}

type Client struct {
	ch     chan []byte
	cursor int
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string][]*Client
	history map[string][][]byte
	maxHist int
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string][]*Client),
		history: make(map[string][][]byte),
		maxHist: 500,
	}
}

func (h *Hub) Register(sessionID string, fromCursor int) *Client {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := &Client{ch: make(chan []byte, 512), cursor: fromCursor}
	h.clients[sessionID] = append(h.clients[sessionID], c)
	return c
}

func (h *Hub) Unregister(sessionID string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.clients[sessionID]
	filtered := list[:0]
	for _, cl := range list {
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
	h.mu.Lock()
	hist := h.history[sessionID]
	hist = append(hist, data)
	if len(hist) > h.maxHist {
		hist = hist[len(hist)-h.maxHist:]
	}
	h.history[sessionID] = hist
	clients := h.clients[sessionID]
	h.mu.Unlock()

	for _, c := range clients {
		select {
		case c.ch <- data:
		default:
		}
	}
}

func (h *Hub) History(sessionID string, fromCursor int) [][]byte {
	h.mu.RLock()
	defer h.mu.RUnlock()
	hist := h.history[sessionID]
	if fromCursor >= len(hist) {
		return nil
	}
	if fromCursor < 0 {
		fromCursor = 0
	}
	out := make([][]byte, len(hist)-fromCursor)
	copy(out, hist[fromCursor:])
	return out
}

func (h *Hub) ClearHistory(sessionID string) {
	h.mu.Lock()
	delete(h.history, sessionID)
	h.mu.Unlock()
}

func (h *Hub) ServeClient(w http.ResponseWriter, r *http.Request, sessionID string, fromCursor int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	for _, past := range h.History(sessionID, fromCursor) {
		fmt.Fprintf(w, "data: %s\n\n", past)
	}
	flusher.Flush()

	client := h.Register(sessionID, fromCursor)
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
