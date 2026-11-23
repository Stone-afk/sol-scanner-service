package block

import (
	"context"
	"errors"
	"time"
)

// shortDrainPoll is the non-blocking follow-up timeout inside Drain.
const shortDrainPoll = time.Millisecond

// Polled is one drained queue item (message + offset) before Commit/Seek.
type Polled struct {
	Msg BlockMessage
	Off Offset
}

// Poller is the minimal consumer surface Drain needs (*Consumer satisfies it).
type Poller interface {
	Poll(ctx context.Context) (BlockMessage, Offset, error)
}

// Drain pulls up to max messages. The first Poll may block up to firstTimeout
// (typically pollInterval); follow-ups use shortDrainPoll so a backlog is
// emptied without idle-waiting. max <= 0 yields nil. Empty poll ends the
// drain; parent cancel and non-empty errors are returned (with any msgs
// already collected).
func Drain(ctx context.Context, p Poller, max int, firstTimeout time.Duration) ([]Polled, error) {
	if max <= 0 {
		return nil, nil
	}
	if firstTimeout <= 0 {
		firstTimeout = shortDrainPoll
	}
	out := make([]Polled, 0, max)
	for len(out) < max {
		timeout := shortDrainPoll
		if len(out) == 0 {
			timeout = firstTimeout
		}
		pollCtx, cancel := context.WithTimeout(ctx, timeout)
		msg, off, err := p.Poll(pollCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				if ctx.Err() != nil {
					return out, ctx.Err()
				}
				return out, nil
			}
			return out, err
		}
		out = append(out, Polled{Msg: msg, Off: off})
	}
	return out, nil
}
