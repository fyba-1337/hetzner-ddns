package ddns

import (
	"context"
	"time"
)

func retry(ctx context.Context, attempts int, baseDelay, maxDelay time.Duration, fn func(context.Context, int) error) error {
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	delay := baseDelay
	for attempt := 1; attempt <= attempts; attempt++ {
		err := fn(ctx, attempt)
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt == attempts {
			break
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
	return lastErr
}
