package processor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
)

func TestApplyParseErrorHaltsOnApply(t *testing.T) {
	// AC-T02 / T006: decode/parse failure halts when that slot is applied,
	// not merely when its parse task finishes.
	// Not parallel: shares package-level parseSlotHook.
	requireOnDemandBlockTaskPool(t)

	t.Run("later_slot_parse_fail_does_not_halt_before_apply_slot", func(t *testing.T) {
		requireOnDemandBlockTaskPool(t)

		cpHash := hashForSlot(100, 1)
		cons := &fakeConsumer{}
		repo := &fakeRepo{
			checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
			hasCP:      true,
		}
		cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
		alerter := &fakeAlerter{repo: repo}

		// Only a bad block at 102 is available; applySlot is 101.
		// Parallel parse may finish (fail) for 102 first — must not DECODE_ERROR yet.
		bad := block.BlockMessage{
			Version:    block.MessageVersion,
			Chain:      testChain,
			Slot:       102,
			Commitment: block.CommitmentFinalized,
			Kind:       block.KindBlock,
			Block:      []byte(`{`),
		}
		cons.enqueue(bad, off(2))

		p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
		p.cfg.ParseWorkers = 4

		var (
			mu           sync.Mutex
			parseDone102 bool
		)
		parseSlotHook = func(slot uint64) {
			if slot == 102 {
				mu.Lock()
				parseDone102 = true
				mu.Unlock()
			}
		}
		defer clearParseSlotHook()

		// Park gap waits so we can observe post-parse state before GAP_UNRESOLVED.
		p.wait = func(ctx context.Context, _ time.Duration) error {
			<-ctx.Done()
			return ctx.Err()
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errCh := make(chan error, 1)
		go func() { errCh <- p.Apply(ctx) }()

		deadline := time.Now().Add(2 * time.Second)
		for {
			mu.Lock()
			done := parseDone102
			mu.Unlock()
			if done {
				break
			}
			if time.Now().After(deadline) {
				cancel()
				<-errCh
				t.Fatal("timeout waiting for slot 102 parse attempt")
			}
			time.Sleep(time.Millisecond)
		}

		// Parse of 102 finished; applySlot is still 101 — must not DECODE_ERROR.
		time.Sleep(20 * time.Millisecond)
		_, _, batches, halts, _ := repo.snapshot()
		for _, h := range halts {
			if h.Reason == domain.HaltReasonDecodeError {
				cancel()
				<-errCh
				t.Fatalf("DECODE_ERROR at slot %d before apply reached it; must not halt on parse finish alone", h.Slot)
			}
		}
		if len(batches) != 0 {
			cancel()
			<-errCh
			t.Fatalf("batches=%d, want none before applySlot 101", len(batches))
		}

		cancel()
		<-errCh
	})

	t.Run("apply_bad_slot_after_prior_save_halts_decode", func(t *testing.T) {
		requireOnDemandBlockTaskPool(t)

		cpHash := hashForSlot(100, 1)
		cons := &fakeConsumer{}
		repo := &fakeRepo{
			checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
			hasCP:      true,
		}
		cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
		alerter := &fakeAlerter{repo: repo}

		h101 := hashForSlot(101, 2)
		m101, o1 := makeBlockMsg(t, 101, 100, cpHash, h101, off(1))
		bad := block.BlockMessage{
			Version:    block.MessageVersion,
			Chain:      testChain,
			Slot:       102,
			Commitment: block.CommitmentFinalized,
			Kind:       block.KindBlock,
			Block:      []byte(`{`),
		}
		cons.enqueue(m101, o1)
		cons.enqueue(bad, off(2))

		p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
		p.cfg.ParseWorkers = 4

		var parseFinished []uint64
		gates := installParseReleaseGates(t, []uint64{101, 102}, &parseFinished)
		defer clearParseSlotHook()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		errCh := make(chan error, 1)
		go func() { errCh <- p.Apply(ctx) }()

		close(gates[102])
		deadline := time.Now().Add(2 * time.Second)
		for {
			finished := snapshotParseFinished(&parseFinished)
			if len(finished) >= 1 && finished[0] == 102 {
				break
			}
			if time.Now().After(deadline) {
				cancel()
				<-errCh
				t.Fatalf("expected 102 to finish parse first; got %v", finished)
			}
			time.Sleep(time.Millisecond)
		}
		cp, hasCP, batches, halts, _ := repo.snapshot()
		if len(halts) != 0 {
			cancel()
			<-errCh
			t.Fatalf("halt before applying 101; halts=%+v", halts)
		}
		if hasCP && cp.Slot != 100 {
			cancel()
			<-errCh
			t.Fatalf("checkpoint advanced to %d on parse finish alone", cp.Slot)
		}
		if len(batches) != 0 {
			cancel()
			<-errCh
			t.Fatalf("saved before releasing 101 parse; batches=%d", len(batches))
		}

		close(gates[101])
		<-errCh

		cp, hasCP, batches, halts, _ = repo.snapshot()
		if !hasCP || cp.Slot != 101 {
			t.Fatalf("checkpoint = %+v has=%v, want 101 after prior save", cp, hasCP)
		}
		if len(batches) != 1 || batches[0].Block.Slot != 101 {
			t.Fatalf("batches = %+v, want only 101", batches)
		}
		if len(halts) != 1 || halts[0].Reason != domain.HaltReasonDecodeError {
			t.Fatalf("halts = %+v, want DECODE_ERROR on apply of 102", halts)
		}
		if halts[0].Slot != 102 {
			t.Fatalf("halt slot = %d, want 102", halts[0].Slot)
		}
	})

	t.Run("parse_panic_halts_decode", func(t *testing.T) {
		cpHash := hashForSlot(100, 1)
		cons := &fakeConsumer{}
		repo := &fakeRepo{
			checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
			hasCP:      true,
		}
		cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
		alerter := &fakeAlerter{repo: repo}

		m101, o1 := makeBlockMsg(t, 101, 100, cpHash, hashForSlot(101, 2), off(1))
		cons.enqueue(m101, o1)

		p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
		parseSlotHook = func(uint64) { panic("boom") }
		defer clearParseSlotHook()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := p.Apply(ctx); err != nil {
			t.Fatalf("Apply() = %v, want nil after halt", err)
		}
		_, _, batches, halts, _ := repo.snapshot()
		if len(batches) != 0 {
			t.Fatalf("batches = %+v, want none", batches)
		}
		if len(halts) != 1 || halts[0].Reason != domain.HaltReasonDecodeError || halts[0].Slot != 101 {
			t.Fatalf("halts = %+v, want one DECODE_ERROR at 101", halts)
		}
	})
}
