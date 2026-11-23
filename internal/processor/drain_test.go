package processor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
)

// shortDrainDeadline is below a normal pollInterval timeout so tests can tell
// drain follow-up polls (short) from the first blocking poll (long).
const shortDrainDeadline = 20 * time.Millisecond

// deadlineGatedConsumer returns at most one message per long-timeout Poll.
// Further messages are only visible to short-timeout Polls (drain follow-ups).
// wait() resets the long-poll budget — so without drain, Apply idle-waits
// between consecutive queued slots even though they were already available.
type deadlineGatedConsumer struct {
	mu            sync.Mutex
	queue         []queuedMsg
	commits       []block.Offset
	seeks         []block.Offset
	pollN         int
	longPollsUsed int
}

func (c *deadlineGatedConsumer) enqueue(msg block.BlockMessage, off block.Offset) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queue = append(c.queue, queuedMsg{msg: msg, off: off})
}

func (c *deadlineGatedConsumer) Poll(ctx context.Context) (block.BlockMessage, block.Offset, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pollN++

	remain := time.Duration(0)
	short := false
	if deadline, ok := ctx.Deadline(); ok {
		remain = time.Until(deadline)
		short = remain > 0 && remain <= shortDrainDeadline
	}

	if short {
		c.longPollsUsed = 0
		if len(c.queue) == 0 {
			return block.BlockMessage{}, block.Offset{}, errFakeEmpty
		}
		item := c.queue[0]
		c.queue = c.queue[1:]
		return item.msg, item.off, nil
	}

	if c.longPollsUsed >= 1 {
		return block.BlockMessage{}, block.Offset{}, errFakeEmpty
	}
	if len(c.queue) == 0 {
		if err := ctx.Err(); err != nil {
			return block.BlockMessage{}, block.Offset{}, err
		}
		return block.BlockMessage{}, block.Offset{}, errFakeEmpty
	}
	c.longPollsUsed++
	item := c.queue[0]
	c.queue = c.queue[1:]
	return item.msg, item.off, nil
}

func (c *deadlineGatedConsumer) Commit(_ context.Context, off block.Offset) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commits = append(c.commits, off)
	return nil
}

func (c *deadlineGatedConsumer) Seek(_ context.Context, off block.Offset) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seeks = append(c.seeks, off)
	return nil
}

func (c *deadlineGatedConsumer) resetLongBudget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.longPollsUsed = 0
}

func TestApplyDrainDoesNotIdleWaitWhenApplicableWorkQueued(t *testing.T) {
	// FR-T01 / AC-T05 subset: when consecutive apply slots are already queued,
	// Apply must drain them without idle-waiting pollInterval between slots.
	t.Parallel()

	cpHash := hashForSlot(100, 1)
	cons := &deadlineGatedConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
	alerter := &fakeAlerter{}

	prev := cpHash
	var offN int64
	for slot := uint64(101); slot <= 103; slot++ {
		bh := hashForSlot(slot, byte(slot))
		msg, o := makeBlockMsg(t, slot, slot-1, prev, bh, off(offN))
		cons.enqueue(msg, o)
		prev = bh
		offN++
	}

	p := New(cons, repo, cursor, alerter, Config{
		Chain:        testChain,
		StartSlot:    100,
		PollInterval: 100 * time.Millisecond,
	}, nil)

	waitsWhileBehind := 0
	p.wait = func(ctx context.Context, d time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		cp, hasCP, _, _, _ := repo.snapshot()
		if hasCP && cp.Slot >= 103 {
			// Work done; park until cancel (empty-poll idle after catch-up is fine).
			<-ctx.Done()
			return ctx.Err()
		}
		waitsWhileBehind++
		// Mimic idle gap wait: only then does a non-draining loop see more msgs.
		cons.resetLongBudget()
		return nil
	}

	done := make(chan struct{})
	go func() {
		for {
			cp, hasCP, _, _, _ := repo.snapshot()
			if hasCP && cp.Slot >= 103 {
				close(done)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- p.Apply(ctx) }()

	select {
	case <-done:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timeout: expected drain to apply 101–103 without gap halt")
	}
	<-errCh

	cp, hasCP, batches, halts, _ := repo.snapshot()
	if !hasCP || cp.Slot != 103 || len(batches) != 3 {
		t.Fatalf("want slots 101–103 applied; cp=%+v batches=%d", cp, len(batches))
	}
	if len(halts) != 0 {
		t.Fatalf("halts = %+v, want none", halts)
	}
	if waitsWhileBehind != 0 {
		t.Fatalf("idle waits while behind = %d, want 0 (drain should apply queued slots first)", waitsWhileBehind)
	}
}
