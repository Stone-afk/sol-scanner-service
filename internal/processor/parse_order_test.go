package processor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
)

// parseFinishedMu guards appends from concurrent parse workers in gated tests.
var parseFinishedMu sync.Mutex

// requireOnDemandBlockTaskPool fails until PLAN-002 wires ekit/pool into the apply path.
// RED reason: no parallel ready / out-of-order parse protection.
func requireOnDemandBlockTaskPool(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(src), "OnDemandBlockTaskPool") &&
			strings.Contains(string(src), "ekit/pool") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no parallel ready/out-of-order protection: OnDemandBlockTaskPool (ekit/pool) missing from processor production sources")
	}
}

func TestApplyParseOrder(t *testing.T) {
	// AC-T01 / T005: parse completion may be out of order; SaveBlockBatch order follows applySlot.
	// Not parallel: shares package-level parseSlotHook.
	requireOnDemandBlockTaskPool(t)

	tests := []struct {
		name          string
		startSlot     uint64
		checkpoint    domain.Checkpoint
		hasCP         bool
		deliver       func(t *testing.T, cons *fakeConsumer)
		releaseParse  []uint64 // hold each slot until released in this order (out-of-order completion)
		wantSaveSlots []uint64
	}{
		{
			name:      "three_blocks_reverse_parse_release_saves_in_slot_order",
			startSlot: 100,
			checkpoint: domain.Checkpoint{
				Slot:      100,
				Blockhash: hashForSlot(100, 1).String(),
			},
			hasCP: true,
			deliver: func(t *testing.T, cons *fakeConsumer) {
				cpHash := hashForSlot(100, 1)
				h101 := hashForSlot(101, 2)
				h102 := hashForSlot(102, 3)
				h103 := hashForSlot(103, 4)
				m101, o1 := makeBlockMsg(t, 101, 100, cpHash, h101, off(1))
				m102, o2 := makeBlockMsg(t, 102, 101, h101, h102, off(2))
				m103, o3 := makeBlockMsg(t, 103, 102, h102, h103, off(3))
				cons.enqueue(m101, o1)
				cons.enqueue(m102, o2)
				cons.enqueue(m103, o3)
			},
			releaseParse:  []uint64{103, 102, 101},
			wantSaveSlots: []uint64{101, 102, 103},
		},
		{
			name:      "skipped_prefix_then_blocks_save_order",
			startSlot: 500,
			deliver: func(t *testing.T, cons *fakeConsumer) {
				sk500, o0 := makeSkippedMsg(500, off(0))
				sk501, o1 := makeSkippedMsg(501, off(1))
				bh := hashForSlot(502, 2)
				prev := hashForSlot(501, 1)
				blk502, o2 := makeBlockMsg(t, 502, 501, prev, bh, off(2))
				cons.enqueue(sk500, o0)
				cons.enqueue(sk501, o1)
				cons.enqueue(blk502, o2)
			},
			releaseParse:  []uint64{502},
			wantSaveSlots: []uint64{502},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			requireOnDemandBlockTaskPool(t)

			cons := &fakeConsumer{}
			repo := &fakeRepo{checkpoint: tc.checkpoint, hasCP: tc.hasCP}
			cursor := &fakeCursor{}
			if tc.hasCP {
				cursor.handedSlot = tc.checkpoint.Slot
				cursor.hasHanded = true
			}
			alerter := &fakeAlerter{}
			tc.deliver(t, cons)

			p := newTestProcessor(t, cons, repo, cursor, alerter, tc.startSlot)
			p.cfg.ParseWorkers = 4
			wantN := len(tc.wantSaveSlots)
			p.wait = func(ctx context.Context, _ time.Duration) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				_, _, batches, _, _ := repo.snapshot()
				if len(batches) >= wantN {
					<-ctx.Done()
					return ctx.Err()
				}
				return nil
			}

			var parseFinished []uint64
			gates := installParseReleaseGates(t, tc.releaseParse, &parseFinished)
			defer clearParseSlotHook()

			done := make(chan struct{})
			go func() {
				for {
					_, _, batches, _, _ := repo.snapshot()
					if len(batches) >= wantN {
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

			// Release one-by-one so parse finish order matches releaseParse (out-of-order vs apply).
			for i, slot := range tc.releaseParse {
				close(gates[slot])
				deadline := time.Now().Add(2 * time.Second)
				for {
					finished := snapshotParseFinished(&parseFinished)
					if len(finished) >= i+1 && finished[i] == slot {
						break
					}
					if time.Now().After(deadline) {
						cancel()
						<-errCh
						t.Fatalf("timeout waiting for parse finish of slot %d; got %v", slot, finished)
					}
					time.Sleep(time.Millisecond)
				}
			}

			select {
			case <-done:
				cancel()
			case <-time.After(3 * time.Second):
				cancel()
				t.Fatal("timeout waiting for ordered saves")
			}
			<-errCh

			_, _, batches, halts, _ := repo.snapshot()
			if len(halts) != 0 {
				t.Fatalf("halts = %+v, want none", halts)
			}
			if len(batches) != len(tc.wantSaveSlots) {
				t.Fatalf("batches len=%d, want %d", len(batches), len(tc.wantSaveSlots))
			}
			for i, want := range tc.wantSaveSlots {
				if batches[i].Block.Slot != want {
					t.Fatalf("save order[%d]=%d, want %d (full=%v)", i, batches[i].Block.Slot, want, batchSlots(batches))
				}
			}

			if len(tc.releaseParse) > 1 {
				finished := snapshotParseFinished(&parseFinished)
				if len(finished) < len(tc.releaseParse) {
					t.Fatalf("parseFinished=%v, want at least %v", finished, tc.releaseParse)
				}
				for i, want := range tc.releaseParse {
					if finished[i] != want {
						t.Fatalf("parse finish order[%d]=%d, want %d (got %v); saves stayed %v",
							i, finished[i], want, finished, tc.wantSaveSlots)
					}
				}
			}
		})
	}
}

func batchSlots(batches []domain.BlockBatch) []uint64 {
	out := make([]uint64, len(batches))
	for i, b := range batches {
		out[i] = b.Block.Slot
	}
	return out
}

// installParseReleaseGates arms parseSlotHook so each listed slot blocks until its gate is closed.
func installParseReleaseGates(t *testing.T, slots []uint64, parseFinished *[]uint64) map[uint64]chan struct{} {
	t.Helper()
	gates := make(map[uint64]chan struct{}, len(slots))
	for _, s := range slots {
		gates[s] = make(chan struct{})
	}
	parseFinishedMu.Lock()
	*parseFinished = (*parseFinished)[:0]
	parseFinishedMu.Unlock()
	parseSlotHook = func(slot uint64) {
		if g, ok := gates[slot]; ok {
			<-g
		}
		parseFinishedMu.Lock()
		*parseFinished = append(*parseFinished, slot)
		parseFinishedMu.Unlock()
	}
	return gates
}

func snapshotParseFinished(parseFinished *[]uint64) []uint64 {
	parseFinishedMu.Lock()
	defer parseFinishedMu.Unlock()
	return append([]uint64(nil), (*parseFinished)...)
}

func clearParseSlotHook() {
	parseSlotHook = nil
}
