package eventbus

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/coohu/goagent/internal/core"
)

func TestEmitAndSubscribe(t *testing.T) {
	bus := New(DefaultConfig())
	defer bus.Shutdown(context.Background())

	received := make(chan core.Event, 1)
	bus.Subscribe(core.EventStart, func(_ context.Context, ev core.Event) {
		received <- ev
	})

	ev := core.Event{
		Type:      core.EventStart,
		SessionID: "s1",
		Payload:   map[string]any{},
		CreatedAt: time.Now(),
	}
	bus.Emit(context.Background(), ev)

	select {
	case got := <-received:
		if got.Type != core.EventStart {
			t.Errorf("expected EventStart, got %s", got.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("event not received in time")
	}
}

func TestDeduplication(t *testing.T) {
	bus := New(DefaultConfig())
	defer bus.Shutdown(context.Background())

	var mu sync.Mutex
	count := 0
	bus.Subscribe(core.EventStart, func(_ context.Context, _ core.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	ev := core.Event{
		Type:      core.EventStart,
		SessionID: "s1",
		Hash:      "fixed-hash",
		Payload:   map[string]any{},
		CreatedAt: time.Now(),
	}
	bus.Emit(context.Background(), ev)
	bus.Emit(context.Background(), ev)
	bus.Emit(context.Background(), ev)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 delivery (dedup), got %d", count)
	}
}

func TestSessionSubscribe(t *testing.T) {
	bus := New(DefaultConfig())
	defer bus.Shutdown(context.Background())

	ch1 := make(chan core.Event, 1)
	ch2 := make(chan core.Event, 1)

	bus.SubscribeSession("session-A", func(_ context.Context, ev core.Event) { ch1 <- ev })
	bus.SubscribeSession("session-B", func(_ context.Context, ev core.Event) { ch2 <- ev })

	bus.Emit(context.Background(), core.Event{
		Type: core.EventStart, SessionID: "session-A",
		Hash: "h1", Payload: map[string]any{}, CreatedAt: time.Now(),
	})

	select {
	case <-ch1:
	case <-time.After(300 * time.Millisecond):
		t.Error("session-A subscriber did not receive event")
	}

	select {
	case <-ch2:
		t.Error("session-B should not have received session-A event")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := New(DefaultConfig())
	defer bus.Shutdown(context.Background())

	var mu sync.Mutex
	count := 0
	unsub := bus.Subscribe(core.EventPlanCreated, func(_ context.Context, _ core.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Emit(context.Background(), core.Event{
		Type: core.EventPlanCreated, Hash: "h1",
		Payload: map[string]any{}, CreatedAt: time.Now(),
	})
	time.Sleep(50 * time.Millisecond)

	unsub()

	bus.Emit(context.Background(), core.Event{
		Type: core.EventPlanCreated, Hash: "h2",
		Payload: map[string]any{}, CreatedAt: time.Now(),
	})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 delivery after unsubscribe, got %d", count)
	}
}

func TestRateLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxReplanPerMin = 2
	bus := New(cfg)
	defer bus.Shutdown(context.Background())

	var mu sync.Mutex
	count := 0
	bus.Subscribe(core.EventReplanNeeded, func(_ context.Context, _ core.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	for i := 0; i < 5; i++ {
		bus.Emit(context.Background(), core.Event{
			Type:      core.EventReplanNeeded,
			Hash:      fmt.Sprintf("h%d", i),
			Payload:   map[string]any{},
			CreatedAt: time.Now(),
		})
	}
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count > cfg.MaxReplanPerMin {
		t.Errorf("rate limit not working: got %d, limit %d", count, cfg.MaxReplanPerMin)
	}
}
