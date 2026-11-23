package ctxwait

import (
	"context"
	"time"
)

// Wait sleeps for d or returns early when ctx is canceled.
func Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
