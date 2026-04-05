package retry

import (
	"context"
	"errors"
	"testing"
)

func TestDoSuccessOnFirstAttempt(t *testing.T) {
	calls := 0
	err := Do(context.Background(), DefaultConfig(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestDoRetriesOnError(t *testing.T) {
	cfg := Config{MaxAttempts: 3, InitialWait: 1, MaxWait: 1}
	calls := 0
	target := errors.New("temporary")

	err := Do(context.Background(), cfg, func() error {
		calls++
		if calls < 3 {
			return target
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestDoExhaustsRetries(t *testing.T) {
	cfg := Config{MaxAttempts: 2, InitialWait: 1, MaxWait: 1}
	permanent := errors.New("permanent failure")

	err := Do(context.Background(), cfg, func() error {
		return permanent
	})
	if !errors.Is(err, permanent) {
		t.Errorf("expected permanent error, got %v", err)
	}
}

func TestDoRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := Config{MaxAttempts: 3, InitialWait: 100, MaxWait: 1000}
	calls := 0
	err := Do(ctx, cfg, func() error {
		calls++
		return errors.New("fail")
	})

	if err == nil {
		t.Error("expected context cancellation error")
	}
}
