package scanner

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
)

func TestRunFatalHandedSlotConflict(t *testing.T) {
	// AC-069 / FR-09 — ErrHandedSlotConflict must make Run return, not sleep-retry.
	t.Parallel()

	reader := newHandoffReader(102)
	reader.slots[[2]uint64{101, 102}] = []uint64{}
	cursor := newFakeCursor(100, true)
	cursor.advErr = repository.ErrHandedSlotConflict
	halts := &countingHaltProbe{}
	s := New(reader, &fakeProducer{}, cursor, halts, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Hour,
		StepSlots:      2,
		SlotsPerWorker: 2,
	}, slog.Default())
	s.wait = func(context.Context, time.Duration) error {
		t.Fatal("wait must not run on fatal handed-slot conflict")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	if !errors.Is(err, repository.ErrHandedSlotConflict) {
		t.Fatalf("Run error = %v, want ErrHandedSlotConflict", err)
	}
	slot, ok := cursor.handedSlot()
	if !ok || slot != 100 {
		t.Fatalf("handed_slot = %d ok=%v, want unchanged 100", slot, ok)
	}
	if cursor.advanceCount() != 0 {
		t.Fatalf("successful AdvanceHandedSlot count = %d, want 0", cursor.advanceCount())
	}
	if halts.createHaltCalls != 0 {
		t.Fatalf("CreateHalt calls = %d, want 0", halts.createHaltCalls)
	}
}

func TestRunFatalGetHandedSlotDBError(t *testing.T) {
	// AC-069 — GetHandedSlot database error is fatal for Run.
	t.Parallel()

	dbErr := errors.New("get handed_slot: connection reset")
	reader := newHandoffReader(102)
	cursor := newFakeCursor(100, true)
	cursor.getErr = dbErr
	halts := &countingHaltProbe{}
	s := New(reader, &fakeProducer{}, cursor, halts, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Hour,
		StepSlots:      2,
		SlotsPerWorker: 2,
	}, slog.Default())
	s.wait = func(context.Context, time.Duration) error {
		t.Fatal("wait must not run on fatal GetHandedSlot DB error")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	if !errors.Is(err, dbErr) {
		t.Fatalf("Run error = %v, want %v", err, dbErr)
	}
	slot, ok := cursor.handedSlot()
	if !ok || slot != 100 {
		t.Fatalf("handed_slot = %d ok=%v, want unchanged 100", slot, ok)
	}
	if reader.blockSlotsCallCount() != 0 {
		t.Fatalf("BlockSlots calls = %d, want 0 before cursor read succeeds", reader.blockSlotsCallCount())
	}
	if halts.createHaltCalls != 0 {
		t.Fatalf("CreateHalt calls = %d, want 0", halts.createHaltCalls)
	}
}

func TestRunFatalAdvanceHandedSlotDBError(t *testing.T) {
	// AC-069 — AdvanceHandedSlot database error is fatal for Run.
	t.Parallel()

	dbErr := errors.New("advance handed_slot: disk full")
	reader := newHandoffReader(102)
	reader.slots[[2]uint64{101, 102}] = []uint64{}
	cursor := newFakeCursor(100, true)
	cursor.advErr = dbErr
	halts := &countingHaltProbe{}
	s := New(reader, &fakeProducer{}, cursor, halts, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Hour,
		StepSlots:      2,
		SlotsPerWorker: 2,
	}, slog.Default())
	s.wait = func(context.Context, time.Duration) error {
		t.Fatal("wait must not run on fatal AdvanceHandedSlot DB error")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	if !errors.Is(err, dbErr) {
		t.Fatalf("Run error = %v, want %v", err, dbErr)
	}
	slot, ok := cursor.handedSlot()
	if !ok || slot != 100 {
		t.Fatalf("handed_slot = %d ok=%v, want unchanged 100", slot, ok)
	}
	if halts.createHaltCalls != 0 {
		t.Fatalf("CreateHalt calls = %d, want 0", halts.createHaltCalls)
	}
}

func TestRunFatalHasUnresolvedHaltDBError(t *testing.T) {
	// AC-069 — HasUnresolvedHalt database error is fatal for Run.
	t.Parallel()

	dbErr := errors.New("halt query: connection reset")
	reader := newHandoffReader(102)
	cursor := newFakeCursor(100, true)
	halts := &countingHaltProbe{err: dbErr}
	s := New(reader, &fakeProducer{}, cursor, halts, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Hour,
		StepSlots:      2,
		SlotsPerWorker: 2,
	}, slog.Default())
	s.wait = func(context.Context, time.Duration) error {
		t.Fatal("wait must not run on fatal HasUnresolvedHalt DB error")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	if !errors.Is(err, dbErr) {
		t.Fatalf("Run error = %v, want %v", err, dbErr)
	}
	if reader.blockSlotsCallCount() != 0 {
		t.Fatalf("BlockSlots calls = %d, want 0", reader.blockSlotsCallCount())
	}
	if halts.createHaltCalls != 0 {
		t.Fatalf("CreateHalt calls = %d, want 0", halts.createHaltCalls)
	}
}

func TestRunFatalUnsupportedVersion(t *testing.T) {
	// AC-069 — UNSUPPORTED_VERSION remains fatal through Run.
	t.Parallel()

	reader := newHandoffReader(102)
	reader.slots[[2]uint64{101, 102}] = []uint64{101}
	reader.blockErrs[101] = provider.Error{
		Operation: "getBlock",
		Kind:      provider.ErrorKindUnsupportedVersion,
	}
	cursor := newFakeCursor(100, true)
	halts := &countingHaltProbe{}
	s := New(reader, &fakeProducer{}, cursor, halts, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Hour,
		StepSlots:      2,
		SlotsPerWorker: 2,
	}, slog.Default())
	s.wait = func(context.Context, time.Duration) error {
		t.Fatal("wait must not run on UNSUPPORTED_VERSION")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := s.Run(ctx)
	if provider.KindOf(err) != provider.ErrorKindUnsupportedVersion {
		t.Fatalf("Run KindOf = %v, want UNSUPPORTED_VERSION (err=%v)", provider.KindOf(err), err)
	}
	slot, ok := cursor.handedSlot()
	if !ok || slot != 100 {
		t.Fatalf("handed_slot = %d ok=%v, want unchanged 100", slot, ok)
	}
	if halts.createHaltCalls != 0 {
		t.Fatalf("CreateHalt calls = %d, want 0", halts.createHaltCalls)
	}
}

func TestRunFatalControlProduceRetries(t *testing.T) {
	// AC-095 / FR-09 — Produce failure must not make Run return from a single Step.
	t.Parallel()

	reader := newHandoffReader(102)
	reader.slots[[2]uint64{101, 102}] = []uint64{101}
	reader.setBlock(101, minimalBlock(101))
	cursor := newFakeCursor(100, true)
	prod := &fakeProducer{err: errors.New("kafka down")}
	s := New(reader, prod, cursor, &fakeHaltChecker{}, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   10 * time.Millisecond,
		StepSlots:      2,
		SlotsPerWorker: 2,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits int
	s.wait = func(ctx context.Context, d time.Duration) error {
		waits++
		cancel()
		return ctx.Err()
	}

	err := s.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled after Produce retry wait", err)
	}
	if waits < 1 {
		t.Fatalf("wait calls = %d, want >= 1 (retriable Produce)", waits)
	}
	slot, ok := cursor.handedSlot()
	if !ok || slot != 100 {
		t.Fatalf("handed_slot = %d ok=%v, want unchanged 100", slot, ok)
	}
}

func TestRunFatalControlProviderRetries(t *testing.T) {
	// FR-09 — non-UNSUPPORTED provider failure must sleep-retry, not exit Run.
	t.Parallel()

	reader := newHandoffReader(102)
	reader.slotsErr[[2]uint64{101, 102}] = provider.Error{
		Operation: "getBlocks",
		Kind:      provider.ErrorKindTransient,
	}
	cursor := newFakeCursor(100, true)
	s := New(reader, &fakeProducer{}, cursor, &fakeHaltChecker{}, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   10 * time.Millisecond,
		StepSlots:      2,
		SlotsPerWorker: 2,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits int
	s.wait = func(ctx context.Context, d time.Duration) error {
		waits++
		cancel()
		return ctx.Err()
	}

	err := s.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled after provider retry wait", err)
	}
	if waits < 1 {
		t.Fatalf("wait calls = %d, want >= 1 (retriable provider)", waits)
	}
	slot, ok := cursor.handedSlot()
	if !ok || slot != 100 {
		t.Fatalf("handed_slot = %d ok=%v, want unchanged 100", slot, ok)
	}
}

// countingHaltProbe satisfies HaltChecker and records accidental CreateHalt use.
type countingHaltProbe struct {
	unresolved      bool
	err             error
	createHaltCalls int
}

func (h *countingHaltProbe) HasUnresolvedHalt(context.Context) (bool, error) {
	return h.unresolved, h.err
}

func (h *countingHaltProbe) CreateHalt(context.Context, any) {
	h.createHaltCalls++
}
