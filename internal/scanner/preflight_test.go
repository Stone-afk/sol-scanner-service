package scanner

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

func TestPreflightRejectsUnsetStartSlot(t *testing.T) {
	// S-10 / AC-064
	t.Parallel()

	reader := newHandoffReader(1000)
	cursor := newFakeCursor(0, false)
	halts := &fakeHaltChecker{}
	s := New(reader, &fakeProducer{}, cursor, halts, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		StartSlot:      0,
		PollInterval:   time.Second,
		StepSlots:      20000,
		SlotsPerWorker: 500,
	}, slog.Default())

	err := s.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight error = nil, want reject unset startSlot")
	}
}

func TestPreflightRejectsGenesisMismatch(t *testing.T) {
	// S-11 / AC-065
	t.Parallel()

	reader := newHandoffReader(1000)
	reader.genesis = "genesis-node"
	cursor := newFakeCursor(0, false)
	s := New(reader, &fakeProducer{}, cursor, &fakeHaltChecker{}, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-config",
		StartSlot:      500,
		PollInterval:   time.Second,
		StepSlots:      20000,
		SlotsPerWorker: 500,
	}, slog.Default())

	err := s.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight error = nil, want genesis mismatch")
	}
}

func TestPreflightRejectsStartBeforeEarliest(t *testing.T) {
	// S-12 / AC-066
	t.Parallel()

	reader := newHandoffReader(1000)
	reader.genesis = "genesis-a"
	reader.earliest = 600
	cursor := newFakeCursor(0, false)
	s := New(reader, &fakeProducer{}, cursor, &fakeHaltChecker{}, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		StartSlot:      500,
		PollInterval:   time.Second,
		StepSlots:      20000,
		SlotsPerWorker: 500,
	}, slog.Default())

	err := s.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight error = nil, want startSlot before earliest")
	}
}

func TestPreflightOKWithCursorSkipsStartSlotEarliest(t *testing.T) {
	t.Parallel()

	reader := newHandoffReader(1000)
	reader.genesis = "genesis-a"
	reader.earliest = 600
	cursor := newFakeCursor(1000, true)
	s := New(reader, &fakeProducer{}, cursor, &fakeHaltChecker{}, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		StartSlot:      0, // unused when cursor exists
		PollInterval:   time.Second,
		StepSlots:      20000,
		SlotsPerWorker: 500,
	}, slog.Default())

	if err := s.Preflight(context.Background()); err != nil {
		t.Fatalf("Preflight error = %v, want nil", err)
	}
}

func TestPreflightUnresolvedHaltWaitsNoGetBlocks(t *testing.T) {
	// S-13 / AC-067 scanner half — wait in Run, not a process-exiting halt error
	t.Parallel()

	reader := newHandoffReader(21000)
	reader.genesis = "genesis-a"
	cursor := newFakeCursor(1000, true)
	halts := &fakeHaltChecker{unresolved: true}
	prod := &fakeProducer{}
	s := New(reader, prod, cursor, halts, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		StartSlot:      500,
		PollInterval:   50 * time.Millisecond,
		StepSlots:      20000,
		SlotsPerWorker: 500,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits int
	s.wait = func(ctx context.Context, d time.Duration) error {
		waits++
		if waits >= 2 {
			cancel()
			return ctx.Err()
		}
		return nil
	}

	err := s.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled (not process-exiting halt)", err)
	}
	if reader.blockSlotsCallCount() != 0 {
		t.Fatalf("BlockSlots calls = %d, want 0 while unresolved halt", reader.blockSlotsCallCount())
	}
	if cursor.advanceCount() != 0 {
		t.Fatalf("AdvanceHandedSlot calls = %d, want 0", cursor.advanceCount())
	}
	if len(prod.messages()) != 0 {
		t.Fatalf("produced %d messages, want 0", len(prod.messages()))
	}
}

// --- shared fakes for handoff / preflight ---

type handoffReader struct {
	mu            sync.Mutex
	finalized     uint64
	genesis       string
	earliest      uint64
	blocks        map[uint64]*rpc.GetBlockResult
	blockErrs     map[uint64]error
	slots         map[[2]uint64][]uint64
	slotsErr      map[[2]uint64]error
	slotsFn       func(from, to uint64) ([]uint64, error)
	release       map[uint64]chan struct{}
	blockSlotsLog [][2]uint64
	blockLog      []uint64
	finalizedFn   func() (uint64, error)
}

func newHandoffReader(finalized uint64) *handoffReader {
	return &handoffReader{
		finalized: finalized,
		genesis:   "genesis-a",
		earliest:  1,
		blocks:    make(map[uint64]*rpc.GetBlockResult),
		blockErrs: make(map[uint64]error),
		slots:     make(map[[2]uint64][]uint64),
		slotsErr:  make(map[[2]uint64]error),
		release:   make(map[uint64]chan struct{}),
	}
}

func (r *handoffReader) setBlock(slot uint64, blk *rpc.GetBlockResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blocks[slot] = blk
}

func (r *handoffReader) GenesisHash(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.genesis, nil
}

func (r *handoffReader) FirstAvailableBlock(context.Context) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.earliest, nil
}

func (r *handoffReader) FinalizedSlot(context.Context) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalizedFn != nil {
		return r.finalizedFn()
	}
	return r.finalized, nil
}

func (r *handoffReader) BlockSlots(_ context.Context, from, to uint64) ([]uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockSlotsLog = append(r.blockSlotsLog, [2]uint64{from, to})
	if r.slotsFn != nil {
		return r.slotsFn(from, to)
	}
	key := [2]uint64{from, to}
	if err, ok := r.slotsErr[key]; ok {
		return nil, err
	}
	if slots, ok := r.slots[key]; ok {
		return append([]uint64(nil), slots...), nil
	}
	var out []uint64
	for slot := range r.blocks {
		if slot >= from && slot <= to {
			out = append(out, slot)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (r *handoffReader) Block(ctx context.Context, slot uint64) (*rpc.GetBlockResult, error) {
	r.mu.Lock()
	r.blockLog = append(r.blockLog, slot)
	rel := r.release[slot]
	if err, ok := r.blockErrs[slot]; ok {
		r.mu.Unlock()
		return nil, err
	}
	blk := r.blocks[slot]
	r.mu.Unlock()

	if rel != nil {
		select {
		case <-rel:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if blk == nil {
		return nil, provider.Error{Operation: "getBlock", Kind: provider.ErrorKindRPC}
	}
	return blk, nil
}

func (r *handoffReader) blockSlotsCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.blockSlotsLog)
}

func (r *handoffReader) blockSlotsRanges() [][2]uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][2]uint64(nil), r.blockSlotsLog...)
}

func (r *handoffReader) blockCalls() []uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]uint64(nil), r.blockLog...)
}

type fakeCursor struct {
	mu       sync.Mutex
	slot     uint64
	ok       bool
	advances [][2]uint64
	advErr   error
	getErr   error
}

func newFakeCursor(slot uint64, ok bool) *fakeCursor {
	return &fakeCursor{slot: slot, ok: ok}
}

func (c *fakeCursor) GetHandedSlot(context.Context) (uint64, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return 0, false, c.getErr
	}
	return c.slot, c.ok, nil
}

func (c *fakeCursor) AdvanceHandedSlot(_ context.Context, prev, next uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.advErr != nil {
		return c.advErr
	}
	c.advances = append(c.advances, [2]uint64{prev, next})
	c.slot = next
	c.ok = true
	return nil
}

func (c *fakeCursor) RewindHandedSlot(_ context.Context, slot uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slot = slot
	c.ok = true
	return nil
}

func (c *fakeCursor) advanceCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.advances)
}

func (c *fakeCursor) lastAdvance() (prev, next uint64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.advances) == 0 {
		return 0, 0, false
	}
	a := c.advances[len(c.advances)-1]
	return a[0], a[1], true
}

func (c *fakeCursor) handedSlot() (uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.slot, c.ok
}

type fakeHaltChecker struct {
	mu         sync.Mutex
	unresolved bool
	err        error
	calls      int
}

func (h *fakeHaltChecker) HasUnresolvedHalt(context.Context) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	return h.unresolved, h.err
}

type fakeProducer struct {
	mu       sync.Mutex
	msgs     []block.BlockMessage
	batches  [][]block.BlockMessage
	err      error
	errAfter int // fail on Nth Produce call (1-based); 0 = always use err if set once
	calls    int
	release  map[int]chan struct{} // gate Produce by call index
}

func (p *fakeProducer) Produce(ctx context.Context, msgs []block.BlockMessage) error {
	p.mu.Lock()
	p.calls++
	call := p.calls
	rel := p.release[call]
	fail := p.err
	if p.errAfter > 0 {
		if call == p.errAfter {
			fail = p.err
		} else {
			fail = nil
		}
	}
	p.mu.Unlock()

	if rel != nil {
		select {
		case <-rel:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if fail != nil {
		return fail
	}
	cp := append([]block.BlockMessage(nil), msgs...)
	p.batches = append(p.batches, cp)
	p.msgs = append(p.msgs, cp...)
	return nil
}

func (p *fakeProducer) messages() []block.BlockMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]block.BlockMessage(nil), p.msgs...)
}

func (p *fakeProducer) produceCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}
