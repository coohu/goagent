package eventbus

import (
	"container/heap"
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/coohu/goagent/internal/core"
)

type subscriber struct {
	id        string
	eventType core.EventType
	sessionID string
	handler   core.EventHandler
}

type Bus struct {
	mu          sync.Mutex
	pq          priorityQueue
	subs        []subscriber
	dedup       map[string]time.Time
	rateCount   map[string]int
	rateReset   map[string]time.Time
	pathHistory map[string][]string

	cfg BusConfig

	workCh chan core.Event
	quit   chan struct{}
	wg     sync.WaitGroup
}

type BusConfig struct {
	QueueCapacity      int
	MaxReplanPerMin    int
	MaxToolRetryPerMin int
	DedupWindow        time.Duration
	LoopThreshold      int
}

func DefaultConfig() BusConfig {
	return BusConfig{
		QueueCapacity:      1000,
		MaxReplanPerMin:    3,
		MaxToolRetryPerMin: 2,
		DedupWindow:        30 * time.Second,
		LoopThreshold:      3,
	}
}

func New(cfg BusConfig) *Bus {
	b := &Bus{
		pq:          make(priorityQueue, 0, cfg.QueueCapacity),
		dedup:       make(map[string]time.Time),
		rateCount:   make(map[string]int),
		rateReset:   make(map[string]time.Time),
		pathHistory: make(map[string][]string),
		cfg:         cfg,
		workCh:      make(chan core.Event, cfg.QueueCapacity),
		quit:        make(chan struct{}),
	}
	heap.Init(&b.pq)
	b.wg.Add(1)
	go b.dispatch()
	return b
}

func (b *Bus) Emit(ctx context.Context, ev core.Event) error {
	if ev.Hash == "" {
		ev.Hash = hashEvent(ev)
	}
	if ev.TTL == 0 {
		ev.TTL = defaultTTL(ev.Type)
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.isDup(ev) {
		return nil
	}
	if b.isRateLimited(ev) {
		return nil
	}

	b.dedup[ev.Hash] = time.Now()
	heap.Push(&b.pq, &item{event: ev, priority: ev.Priority})

	select {
	case b.workCh <- ev:
	default:
	}
	return nil
}

func (b *Bus) Subscribe(eventType core.EventType, handler core.EventHandler) func() {
	id := fmt.Sprintf("sub-%d", time.Now().UnixNano())
	b.mu.Lock()
	b.subs = append(b.subs, subscriber{id: id, eventType: eventType, handler: handler})
	b.mu.Unlock()
	return func() { b.unsubscribe(id) }
}

func (b *Bus) SubscribeSession(sessionID string, handler core.EventHandler) func() {
	id := fmt.Sprintf("sess-%s-%d", sessionID, time.Now().UnixNano())
	b.mu.Lock()
	b.subs = append(b.subs, subscriber{id: id, sessionID: sessionID, handler: handler})
	b.mu.Unlock()
	return func() { b.unsubscribe(id) }
}

func (b *Bus) Shutdown(ctx context.Context) error {
	close(b.quit)
	done := make(chan struct{})
	go func() { b.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Bus) dispatch() {
	defer b.wg.Done()
	for {
		select {
		case <-b.quit:
			return
		case ev := <-b.workCh:
			b.mu.Lock()
			subs := make([]subscriber, len(b.subs))
			copy(subs, b.subs)
			b.mu.Unlock()

			for _, s := range subs {
				if s.eventType != "" && s.eventType != ev.Type {
					continue
				}
				if s.sessionID != "" && s.sessionID != ev.SessionID {
					continue
				}
				s.handler(context.Background(), ev)
			}
		}
	}
}

func (b *Bus) unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	filtered := b.subs[:0]
	for _, s := range b.subs {
		if s.id != id {
			filtered = append(filtered, s)
		}
	}
	b.subs = filtered
}

func (b *Bus) isDup(ev core.Event) bool {
	if t, ok := b.dedup[ev.Hash]; ok {
		if time.Since(t) < b.cfg.DedupWindow {
			return true
		}
	}
	return false
}

func (b *Bus) isRateLimited(ev core.Event) bool {
	key := string(ev.Type)
	limit := b.rateLimit(ev.Type)
	if limit == 0 {
		return false
	}
	if reset, ok := b.rateReset[key]; !ok || time.Now().After(reset) {
		b.rateCount[key] = 0
		b.rateReset[key] = time.Now().Add(time.Minute)
	}
	b.rateCount[key]++
	return b.rateCount[key] > limit
}

func (b *Bus) rateLimit(t core.EventType) int {
	switch t {
	case core.EventReplanNeeded:
		return b.cfg.MaxReplanPerMin
	case core.EventToolError:
		return b.cfg.MaxToolRetryPerMin
	}
	return 0
}

func hashEvent(ev core.Event) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s", ev.Type, ev.SessionID)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func defaultTTL(t core.EventType) int {
	switch t {
	case core.EventReplanNeeded:
		return 3
	case core.EventToolError:
		return 2
	case core.EventMemoryUpdate:
		return 5
	}
	return 10
}

type item struct {
	event    core.Event
	priority int
	index    int
}

type priorityQueue []*item

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].priority < pq[j].priority }
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *priorityQueue) Push(x any) {
	it := x.(*item)
	it.index = len(*pq)
	*pq = append(*pq, it)
}
func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	it := old[n-1]
	*pq = old[:n-1]
	return it
}
