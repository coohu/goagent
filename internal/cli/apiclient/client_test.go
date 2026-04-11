package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	if err := New(srv.URL).Health(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body RunRequest
		json.NewDecoder(r.Body).Decode(&body)
		if body.Goal != "test goal" {
			t.Errorf("wrong goal: %s", body.Goal)
		}
		json.NewEncoder(w).Encode(RunResponse{SessionID: "sess-123", State: "IDLE"})
	}))
	defer srv.Close()

	resp, err := New(srv.URL).Run(context.Background(), "test goal", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SessionID != "sess-123" {
		t.Errorf("wrong session ID: %s", resp.SessionID)
	}
}

func TestCancel(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			called = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := New(srv.URL).Cancel(context.Background(), "sess-abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("DELETE was not called")
	}
}

func TestListSessions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"sessions": []SessionItem{
				{ID: "s1", Goal: "goal one", State: "DONE"},
				{ID: "s2", Goal: "goal two", State: "PLANNING"},
			},
		})
	}))
	defer srv.Close()

	sessions, err := New(srv.URL).ListSessions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"session not found"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL).Status(context.Background(), "bad-id")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}
