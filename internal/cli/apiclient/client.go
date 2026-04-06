package apiclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	base   string
	http   *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// ── Agent ──────────────────────────────────────────────────────────

type RunRequest struct {
	Goal   string         `json:"goal"`
	Config map[string]any `json:"config,omitempty"`
}

type RunResponse struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	StreamURL string `json:"stream_url"`
}

func (c *Client) Run(ctx context.Context, goal string, cfg map[string]any) (*RunResponse, error) {
	var resp RunResponse
	return &resp, c.post(ctx, "/api/v1/agent/run", RunRequest{Goal: goal, Config: cfg}, &resp)
}

func (c *Client) Continue(ctx context.Context, sessionID, goal string) (*RunResponse, error) {
	var resp RunResponse
	return &resp, c.post(ctx, "/api/v1/agent/"+sessionID+"/continue", map[string]any{"goal": goal}, &resp)
}

func (c *Client) Status(ctx context.Context, sessionID string) (map[string]any, error) {
	var resp map[string]any
	return resp, c.get(ctx, "/api/v1/agent/"+sessionID+"/status", &resp)
}

func (c *Client) Cancel(ctx context.Context, sessionID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.base+"/api/v1/agent/"+sessionID, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) Approve(ctx context.Context, sessionID string, approved bool, comment string) error {
	return c.post(ctx, "/api/v1/agent/"+sessionID+"/approve",
		map[string]any{"approved": approved, "comment": comment}, nil)
}

func (c *Client) UpdateConfig(ctx context.Context, sessionID string, patch map[string]any) error {
	return c.put(ctx, "/api/v1/agent/"+sessionID+"/config", patch, nil)
}

// ── Sessions ───────────────────────────────────────────────────────

type SessionItem struct {
	ID        string    `json:"id"`
	Goal      string    `json:"goal"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *Client) ListSessions(ctx context.Context) ([]SessionItem, error) {
	var body struct {
		Sessions []SessionItem `json:"sessions"`
	}
	return body.Sessions, c.get(ctx, "/api/v1/sessions", &body)
}

// ── Events ─────────────────────────────────────────────────────────

type SSEEvent struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
	Ts      time.Time      `json:"ts"`
}

func (c *Client) Events(ctx context.Context, sessionID string, cursor int) ([]SSEEvent, int, error) {
	var body struct {
		Events []string `json:"events"`
		Cursor int      `json:"cursor"`
	}
	url := fmt.Sprintf("/api/v1/agent/%s/events?cursor=%d", sessionID, cursor)
	if err := c.get(ctx, url, &body); err != nil {
		return nil, 0, err
	}
	events := make([]SSEEvent, 0, len(body.Events))
	for _, raw := range body.Events {
		var ev SSEEvent
		if err := json.Unmarshal([]byte(raw), &ev); err == nil {
			events = append(events, ev)
		}
	}
	return events, body.Cursor, nil
}

// StreamEvents opens an SSE connection and sends parsed events to ch until ctx is done.
func (c *Client) StreamEvents(ctx context.Context, sessionID string, cursor int, ch chan<- SSEEvent) error {
	url := fmt.Sprintf("%s/api/v1/agent/%s/stream?cursor=%d", c.base, sessionID, cursor)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	sseClient := &http.Client{Timeout: 0}
	resp, err := sseClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var ev SSEEvent
		if err := json.Unmarshal([]byte(data), &ev); err == nil {
			select {
			case ch <- ev:
			case <-ctx.Done():
				return nil
			}
		}
	}
	return scanner.Err()
}

// ── Models / Tools ─────────────────────────────────────────────────

type ModelInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Default  bool   `json:"default"`
}

func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	var body struct {
		Models []ModelInfo `json:"models"`
	}
	return body.Models, c.get(ctx, "/api/v1/models", &body)
}

// ── Files ──────────────────────────────────────────────────────────

type UploadResult struct {
	Uploaded  []string `json:"uploaded"`
	Workspace string   `json:"workspace"`
}

func (c *Client) Upload(ctx context.Context, sessionID string, localPaths []string) (*UploadResult, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	for _, lp := range localPaths {
		f, err := os.Open(lp)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", lp, err)
		}
		part, err := mw.CreateFormFile("files", filepath.Base(lp))
		if err != nil {
			f.Close()
			return nil, err
		}
		io.Copy(part, f)
		f.Close()
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/api/v1/files/"+sessionID+"/upload", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result UploadResult
	return &result, json.NewDecoder(resp.Body).Decode(&result)
}

func (c *Client) Download(ctx context.Context, sessionID, remotePath, localDest string) error {
	url := fmt.Sprintf("%s/api/v1/files/%s/download?path=%s", c.base, sessionID, remotePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.Create(localDest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func (c *Client) Health(ctx context.Context) error {
	var body map[string]any
	return c.get(ctx, "/health", &body)
}

// ── helpers ────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) put(ctx context.Context, path string, body, out any) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
