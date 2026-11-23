package processor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
)

func TestApplyNoCheckpointSkippedPrefixCommitsFirstBlock(t *testing.T) {
	// S-09 / AC-089
	t.Parallel()

	cons := &fakeConsumer{}
	repo := &fakeRepo{}
	cursor := &fakeCursor{}
	alerter := &fakeAlerter{}

	sk500, o0 := makeSkippedMsg(500, off(0))
	sk501, o1 := makeSkippedMsg(501, off(1))
	bh := hashForSlot(502, 2)
	prev := hashForSlot(501, 1)
	blk502, o2 := makeBlockMsg(t, 502, 501, prev, bh, off(2))
	cons.enqueue(sk500, o0)
	cons.enqueue(sk501, o1)
	cons.enqueue(blk502, o2)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 500)
	blockUntilCancelWait(p)

	done := make(chan struct{})
	go func() {
		for {
			_, hasCP, batches, _, _ := repo.snapshot()
			if hasCP && len(batches) == 1 && batches[0].Block.Slot == 502 {
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
		t.Fatal("timeout waiting for slot 502 commit")
	}
	<-errCh

	cp, hasCP, batches, halts, _ := repo.snapshot()
	if !hasCP || cp.Slot != 502 || cp.Blockhash != bh.String() {
		t.Fatalf("checkpoint = %+v has=%v, want slot 502", cp, hasCP)
	}
	if len(batches) != 1 || batches[0].Block.Slot != 502 {
		t.Fatalf("batches = %+v, want one block at 502", batches)
	}
	// No parent-hash compare against DB: first block allowed after skipped prefix.
	if batches[0].Block.PreviousBlockhash != prev.String() {
		t.Fatalf("previousBlockhash = %q", batches[0].Block.PreviousBlockhash)
	}
	if len(halts) != 0 {
		t.Fatalf("halts = %+v, want none", halts)
	}
	commits, _, _ := cons.snapshot()
	if len(commits) < 3 {
		t.Fatalf("commits = %+v, want at least skipped 500/501 and block 502", commits)
	}
}

func TestApplyNoCheckpointMissingPrefixDoesNotCommit(t *testing.T) {
	// S-09b / AC-089
	t.Parallel()

	cons := &fakeConsumer{}
	repo := &fakeRepo{}
	cursor := &fakeCursor{}
	alerter := &fakeAlerter{}

	bh := hashForSlot(502, 2)
	prev := hashForSlot(501, 1)
	blk502, o2 := makeBlockMsg(t, 502, 501, prev, bh, off(2))
	cons.enqueue(blk502, o2)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 500)

	// After 10 empty rounds past the buffered 502, expect GAP_UNRESOLVED without saving 502.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Apply(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		// Apply may return nil after halt.
		if !errors.Is(err, context.DeadlineExceeded) {
			// halt path returns nil; unexpected errors fail
			t.Fatalf("Apply: %v", err)
		}
	}

	// Give halt path a moment if still running under timeout cancel.
	_, hasCP, batches, halts, _ := repo.snapshot()
	if hasCP || len(batches) != 0 {
		t.Fatalf("must not commit 502 without prefix; hasCP=%v batches=%d", hasCP, len(batches))
	}
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonGapUnresolved {
		t.Fatalf("halts = %+v, want GAP_UNRESOLVED", halts)
	}
	if halts[0].Slot != 500 {
		t.Fatalf("halt slot = %d, want applySlot 500", halts[0].Slot)
	}
}

func TestApplyQueueNotReadyDoesNotCountGapMiss(t *testing.T) {
	// Unknown-topic / auto-create race must not burn the 10 empty-poll budget.
	t.Parallel()

	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 101, Blockhash: hashForSlot(101, 1).String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 101, hasHanded: true}
	alerter := &fakeAlerter{}
	for i := 0; i < 15; i++ {
		cons.enqueueErr(block.ErrQueueNotReady)
	}
	bh := hashForSlot(102, 2)
	prev := hashForSlot(101, 1)
	msg, o := makeBlockMsg(t, 102, 101, prev, bh, off(0))
	cons.enqueue(msg, o)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	waits := 0
	p.wait = func(ctx context.Context, _ time.Duration) error {
		waits++
		cp, hasCP, _, _, _ := repo.snapshot()
		if hasCP && cp.Slot >= 102 {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}

	done := make(chan struct{})
	go func() {
		for {
			cp, hasCP, _, _, _ := repo.snapshot()
			if hasCP && cp.Slot == 102 {
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
		t.Fatal("timeout: queue-not-ready polls should not GAP before slot 102 applies")
	}
	<-errCh

	cp, hasCP, _, halts, _ := repo.snapshot()
	if !hasCP || cp.Slot != 102 {
		t.Fatalf("checkpoint = %+v has=%v, want slot 102", cp, hasCP)
	}
	if len(halts) != 0 {
		t.Fatalf("halts = %+v, want none after queue-not-ready waits", halts)
	}
	if waits < 15 {
		t.Fatalf("waits = %d, want >= 15 queue-not-ready waits", waits)
	}
}

func TestApplyTenRoundGapUnresolved(t *testing.T) {
	// S-04 / AC-090
	t.Parallel()

	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 101, Blockhash: hashForSlot(101, 1).String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 101, hasHanded: true}
	alerter := &fakeAlerter{}
	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)

	waits := 0
	p.wait = func(context.Context, time.Duration) error {
		waits++
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Apply(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("Apply: %v", err)
	}

	cp, hasCP, batches, halts, _ := repo.snapshot()
	if !hasCP || cp.Slot != 101 || len(batches) != 0 {
		t.Fatalf("checkpoint must stay at 101 unchanged; cp=%+v batches=%d", cp, len(batches))
	}
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonGapUnresolved {
		t.Fatalf("halts = %+v, want GAP_UNRESOLVED", halts)
	}
	if halts[0].Slot != 102 {
		t.Fatalf("halt slot = %d, want 102", halts[0].Slot)
	}
	if waits < 9 {
		// 10 miss rounds → at least 9 waits between them (or 10 depending on loop)
		t.Fatalf("injected waits = %d, want >= 9 (no real sleep of 10 intervals)", waits)
	}
	commits, _, _ := cons.snapshot()
	if len(commits) != 0 {
		t.Fatalf("commits = %+v, want none on gap halt", commits)
	}
}

func TestApplyHashMismatch(t *testing.T) {
	// S-05 / AC-090
	t.Parallel()

	cpHash := hashForSlot(100, 9)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
	alerter := &fakeAlerter{}

	wrongPrev := hashForSlot(100, 1) // different from checkpoint
	bh := hashForSlot(101, 2)
	msg, o := makeBlockMsg(t, 101, 100, wrongPrev, bh, off(0))
	cons.enqueue(msg, o)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Apply(ctx)

	_, hasCP, batches, halts, _ := repo.snapshot()
	if !hasCP || len(batches) != 0 {
		t.Fatalf("must not commit on HASH_MISMATCH; batches=%d", len(batches))
	}
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonHashMismatch {
		t.Fatalf("halts = %+v, want HASH_MISMATCH", halts)
	}
	commits, _, _ := cons.snapshot()
	if len(commits) != 0 {
		t.Fatalf("must not commit offset on HASH_MISMATCH; commits=%+v", commits)
	}
}

func TestApplyParentBehind(t *testing.T) {
	// S-06 / AC-090
	t.Parallel()

	cpHash := hashForSlot(100, 9)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
	alerter := &fakeAlerter{}

	bh := hashForSlot(101, 2)
	msg, o := makeBlockMsg(t, 101, 99, cpHash, bh, off(0)) // parentSlot 99 < 100
	cons.enqueue(msg, o)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Apply(ctx)

	_, _, batches, halts, _ := repo.snapshot()
	if len(batches) != 0 {
		t.Fatalf("must not commit on PARENT_BEHIND; batches=%d", len(batches))
	}
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonParentBehind {
		t.Fatalf("halts = %+v, want PARENT_BEHIND", halts)
	}
}

func TestApplyImmediateGapAfterSkippedParent(t *testing.T) {
	// S-14 / AC-090
	t.Parallel()

	cpHash := hashForSlot(101, 1)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 101, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 101, hasHanded: true}
	alerter := &fakeAlerter{}

	sk102, o0 := makeSkippedMsg(102, off(0))
	bh := hashForSlot(103, 3)
	// parentSlot=102 > checkpoint 101 → immediate GAP_UNRESOLVED
	blk103, o1 := makeBlockMsg(t, 103, 102, hashForSlot(102, 2), bh, off(1))
	cons.enqueue(sk102, o0)
	cons.enqueue(blk103, o1)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Apply(ctx)

	cp, hasCP, batches, halts, _ := repo.snapshot()
	if !hasCP || cp.Slot != 101 || len(batches) != 0 {
		t.Fatalf("checkpoint must stay 101; cp=%+v batches=%d", cp, len(batches))
	}
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonGapUnresolved {
		t.Fatalf("halts = %+v, want immediate GAP_UNRESOLVED", halts)
	}
	if halts[0].Slot != 103 {
		t.Fatalf("halt slot = %d, want 103", halts[0].Slot)
	}
}

func TestApplySkippedAdvancesApplySlotOnly(t *testing.T) {
	// S-17 / AC-091
	t.Parallel()

	cpHash := hashForSlot(100, 1)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
	alerter := &fakeAlerter{}

	sk101, o0 := makeSkippedMsg(101, off(0))
	cons.enqueue(sk101, o0)

	// After skipped, leave queue empty so Apply eventually gaps on 102 — we stop once skipped committed.
	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	blockUntilCancelWait(p)

	committedSkipped := make(chan struct{})
	go func() {
		for {
			commits, _, _ := cons.snapshot()
			for _, c := range commits {
				if c.Offset == 0 {
					close(committedSkipped)
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- p.Apply(ctx) }()

	select {
	case <-committedSkipped:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timeout waiting for skipped commit")
	}
	<-errCh

	cp, hasCP, batches, halts, _ := repo.snapshot()
	if !hasCP || cp.Slot != 100 || cp.Blockhash != cpHash.String() {
		t.Fatalf("checkpoint must be unchanged; cp=%+v", cp)
	}
	if len(batches) != 0 {
		t.Fatalf("no business rows for skipped slot; batches=%d", len(batches))
	}
	if len(halts) != 0 {
		t.Fatalf("halts = %+v, want none before cancel", halts)
	}
	commits, _, _ := cons.snapshot()
	found := false
	for _, c := range commits {
		if c.Offset == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected commit of skipped offset 0; commits=%+v", commits)
	}
}

func TestApplyUnresolvedHaltWaitsWithoutExiting(t *testing.T) {
	// AC-067 processor half
	t.Parallel()

	cons := &fakeConsumer{}
	repo := &fakeRepo{unresolved: true}
	cursor := &fakeCursor{}
	alerter := &fakeAlerter{}
	p := newTestProcessor(t, cons, repo, cursor, alerter, 500)

	waits := 0
	p.wait = func(ctx context.Context, d time.Duration) error {
		waits++
		if waits >= 3 {
			return context.Canceled
		}
		return nil
	}

	err := p.Apply(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Apply err = %v, want context.Canceled after wait", err)
	}
	_, _, batches, halts, _ := repo.snapshot()
	if len(batches) != 0 || len(halts) != 0 {
		t.Fatalf("unresolved halt must not start apply; batches=%d halts=%d", len(batches), len(halts))
	}
	commits, _, pollN := cons.snapshot()
	if len(commits) != 0 || pollN != 0 {
		t.Fatalf("must not poll/commit while unresolved; commits=%d pollN=%d", len(commits), pollN)
	}
	if waits < 3 {
		t.Fatalf("waits = %d, want >= 3", waits)
	}
}

func TestApplyOutOfOrderBuffersUntilPrefix(t *testing.T) {
	// AC-089 companion: 502 then 500/501 still commits once prefix present
	t.Parallel()

	cons := &fakeConsumer{}
	repo := &fakeRepo{}
	cursor := &fakeCursor{}
	alerter := &fakeAlerter{}

	bh := hashForSlot(502, 2)
	prev := hashForSlot(501, 1)
	blk502, o2 := makeBlockMsg(t, 502, 501, prev, bh, off(2))
	sk500, o0 := makeSkippedMsg(500, off(0))
	sk501, o1 := makeSkippedMsg(501, off(1))
	cons.enqueue(blk502, o2)
	cons.enqueue(sk500, o0)
	cons.enqueue(sk501, o1)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 500)
	waitWhileNoCheckpoint(p, repo)

	done := make(chan struct{})
	go func() {
		for {
			_, hasCP, _, _, _ := repo.snapshot()
			if hasCP {
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
		t.Fatal("timeout waiting for buffered 502 commit after prefix")
	}
	<-errCh

	cp, hasCP, batches, _, _ := repo.snapshot()
	if !hasCP || cp.Slot != 502 || len(batches) != 1 {
		t.Fatalf("want 502 committed; cp=%+v batches=%d", cp, len(batches))
	}
}

func TestApplyManyFutureSlotsBeforeTargetNoFalseGap(t *testing.T) {
	// Concurrent handoff can deliver ≥10 later slots before applySlot.
	// Those must not count as gap misses.
	t.Parallel()

	cons := &fakeConsumer{}
	repo := &fakeRepo{}
	cursor := &fakeCursor{}
	alerter := &fakeAlerter{}

	var offN int64
	for slot := uint64(510); slot <= 520; slot++ {
		sk, o := makeSkippedMsg(slot, off(offN))
		cons.enqueue(sk, o)
		offN++
	}
	sk500, o500 := makeSkippedMsg(500, off(offN))
	offN++
	sk501, o501 := makeSkippedMsg(501, off(offN))
	offN++
	bh := hashForSlot(502, 2)
	prev := hashForSlot(501, 1)
	blk502, o502 := makeBlockMsg(t, 502, 501, prev, bh, off(offN))
	cons.enqueue(sk500, o500)
	cons.enqueue(sk501, o501)
	cons.enqueue(blk502, o502)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 500)
	waitWhileNoCheckpoint(p, repo)

	done := make(chan struct{})
	go func() {
		for {
			_, hasCP, _, _, _ := repo.snapshot()
			if hasCP {
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
		t.Fatal("timeout: future slots likely counted as gap misses")
	}
	<-errCh

	_, _, _, halts, _ := repo.snapshot()
	if len(halts) != 0 {
		t.Fatalf("halts = %+v, want none (no false GAP)", halts)
	}
	cp, hasCP, batches, _, _ := repo.snapshot()
	if !hasCP || cp.Slot != 502 || len(batches) != 1 {
		t.Fatalf("want 502 committed; cp=%+v batches=%d", cp, len(batches))
	}
}
