package retry

import (
	"context"
	"time"
)

type Config struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
}

func DefaultConfig() Config {
	return Config{
		MaxAttempts: 3,
		InitialWait: 500 * time.Millisecond,
		MaxWait:     10 * time.Second,
	}
}

func Do(ctx context.Context, cfg Config, fn func() error) error {
	wait := cfg.InitialWait
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if attempt == cfg.MaxAttempts-1 {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}

		wait *= 2
		if wait > cfg.MaxWait {
			wait = cfg.MaxWait
		}
	}
	return lastErr
}
