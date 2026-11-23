package scanner

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

func TestHandoffS01_FortySegmentsAdvanceOnce(t *testing.T) {
	// S-01 / AC-084
	t.Parallel()

	reader := newHandoffReader(21000)
	// Empty lists → all skipped; still proves 40 segment boundaries and one advance.
	reader.slotsFn = func(from, to uint64) ([]uint64, error) {
		return []uint64{}, nil
	}

	cursor := newFakeCursor(1000, true)
	prod := &fakeProducer{}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		StartSlot:      1,
		PollInterval:   time.Second,
		StepSlots:      20000,
		SlotsPerWorker: 500,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.wait = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}

	err := s.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}

	assertSegmentCalls(t, reader.blockSlotsRanges(), Segments(1001, 21000, 500))
	if cursor.advanceCount() != 1 {
		t.Fatalf("AdvanceHandedSlot calls = %d, want 1", cursor.advanceCount())
	}
	prev, next, ok := cursor.lastAdvance()
	if !ok || prev != 1000 || next != 21000 {
		t.Fatalf("advance = (%d,%d ok=%v), want (1000,21000)", prev, next, ok)
	}
}

func TestHandoffS02_OutOfOrderProduceStillAdvancesOnce(t *testing.T) {
	// S-02 / AC-084 unordered receipts
	t.Parallel()

	reader := newHandoffReader(21000)
	reader.slotsFn = func(from, to uint64) ([]uint64, error) {
		return []uint64{}, nil
	}

	cursor := newFakeCursor(1000, true)
	prod := &fakeProducer{release: make(map[int]chan struct{})}
	// 40 segments → gate each Produce; release out of order.
	for i := 1; i <= 40; i++ {
		prod.release[i] = make(chan struct{})
	}

	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Second,
		StepSlots:      20000,
		SlotsPerWorker: 500,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.wait = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}

	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	order := make([]int, 0, 40)
	for i := 40; i >= 1; i-- {
		order = append(order, i)
	}
	for _, call := range order {
		select {
		case prod.release[call] <- struct{}{}:
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout releasing produce call %d", call)
		}
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context.Canceled", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run timed out")
	}

	if cursor.advanceCount() != 1 {
		t.Fatalf("AdvanceHandedSlot calls = %d, want 1", cursor.advanceCount())
	}
	_, next, ok := cursor.lastAdvance()
	if !ok || next != 21000 {
		t.Fatalf("advance next = %d ok=%v, want 21000", next, ok)
	}
}

func TestStepS03_SkippedGapInSegment(t *testing.T) {
	// S-03 / AC-086
	t.Parallel()

	reader := newHandoffReader(103)
	reader.slots[[2]uint64{101, 103}] = []uint64{101, 103}
	reader.setBlock(101, minimalBlock(101))
	reader.setBlock(103, minimalBlock(103))

	cursor := newFakeCursor(100, true)
	prod := &fakeProducer{}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Second,
		StepSlots:      3,
		SlotsPerWorker: 3,
	})

	if err := s.Step(context.Background()); err != nil {
		t.Fatalf("Step error = %v", err)
	}

	gotBlocks := reader.blockCalls()
	if len(gotBlocks) != 2 || gotBlocks[0] != 101 || gotBlocks[1] != 103 {
		t.Fatalf("Block calls = %v, want [101 103]", gotBlocks)
	}
	msgs := prod.messages()
	if len(msgs) != 3 {
		t.Fatalf("messages = %d, want 3", len(msgs))
	}
	bySlot := map[uint64]block.BlockMessage{}
	for _, m := range msgs {
		bySlot[m.Slot] = m
	}
	if bySlot[101].Kind != block.KindBlock || bySlot[103].Kind != block.KindBlock {
		t.Fatalf("101/103 kinds = %q/%q", bySlot[101].Kind, bySlot[103].Kind)
	}
	if bySlot[102].Kind != block.KindSkipped || len(bySlot[102].Block) != 0 {
		t.Fatalf("102 = %+v, want skipped", bySlot[102])
	}
	_, next, ok := cursor.lastAdvance()
	if !ok || next != 103 {
		t.Fatalf("advance next = %d ok=%v, want 103", next, ok)
	}
}

func TestStepS07_EmptyBlockSlotsAllSkipped(t *testing.T) {
	// S-07 / AC-086
	t.Parallel()

	reader := newHandoffReader(105)
	reader.slots[[2]uint64{101, 105}] = []uint64{}

	cursor := newFakeCursor(100, true)
	prod := &fakeProducer{}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Second,
		StepSlots:      5,
		SlotsPerWorker: 5,
	})

	if err := s.Step(context.Background()); err != nil {
		t.Fatalf("Step error = %v", err)
	}
	msgs := prod.messages()
	if len(msgs) != 5 {
		t.Fatalf("messages = %d, want 5 skipped", len(msgs))
	}
	for _, m := range msgs {
		if m.Kind != block.KindSkipped {
			t.Fatalf("msg slot %d kind = %q, want skipped", m.Slot, m.Kind)
		}
	}
	if reader.blockSlotsCallCount() != 1 {
		t.Fatalf("BlockSlots calls = %d, want 1", reader.blockSlotsCallCount())
	}
	if len(reader.blockCalls()) != 0 {
		t.Fatalf("Block calls = %v, want none", reader.blockCalls())
	}
	_, next, ok := cursor.lastAdvance()
	if !ok || next != 105 {
		t.Fatalf("advance next = %d ok=%v, want 105", next, ok)
	}
}

func TestStepS07b_GetBlocksFailureNoSkippedNoAdvance(t *testing.T) {
	// S-07b / AC-087
	t.Parallel()

	reader := newHandoffReader(105)
	reader.slotsErr[[2]uint64{101, 105}] = provider.Error{
		Operation: "getBlocks",
		Kind:      provider.ErrorKindTransient,
	}

	cursor := newFakeCursor(100, true)
	prod := &fakeProducer{}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Second,
		StepSlots:      5,
		SlotsPerWorker: 5,
	})

	err := s.Step(context.Background())
	if err == nil {
		t.Fatal("Step error = nil, want getBlocks failure")
	}
	if len(prod.messages()) != 0 {
		t.Fatalf("produced %d msgs, want 0 (no skipped)", len(prod.messages()))
	}
	if cursor.advanceCount() != 0 {
		t.Fatalf("AdvanceHandedSlot calls = %d, want 0", cursor.advanceCount())
	}
}

func TestStepS08_GetBlockFailureNoAdvance(t *testing.T) {
	// S-08 / AC-087
	t.Parallel()

	reader := newHandoffReader(103)
	reader.slots[[2]uint64{101, 103}] = []uint64{101, 103}
	reader.setBlock(101, minimalBlock(101))
	reader.blockErrs[103] = provider.Error{Operation: "getBlock", Kind: provider.ErrorKindRPC}

	cursor := newFakeCursor(100, true)
	prod := &fakeProducer{}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Second,
		StepSlots:      3,
		SlotsPerWorker: 3,
	})

	err := s.Step(context.Background())
	if err == nil {
		t.Fatal("Step error = nil, want getBlock failure")
	}
	if cursor.advanceCount() != 0 {
		t.Fatalf("AdvanceHandedSlot calls = %d, want 0", cursor.advanceCount())
	}
}

func TestStepS15_FinalizedCapsRightEdge(t *testing.T) {
	// S-15 / AC-085
	t.Parallel()

	reader := newHandoffReader(1800)
	reader.slotsFn = func(from, to uint64) ([]uint64, error) {
		return []uint64{}, nil
	}

	cursor := newFakeCursor(1000, true)
	prod := &fakeProducer{}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Second,
		StepSlots:      20000,
		SlotsPerWorker: 500,
	})

	if err := s.Step(context.Background()); err != nil {
		t.Fatalf("Step error = %v", err)
	}

	assertSegmentCalls(t, reader.blockSlotsRanges(), Segments(1001, 1800, 500))
	for _, m := range prod.messages() {
		if m.Slot > 1800 {
			t.Fatalf("produced slot %d > 1800", m.Slot)
		}
	}
	_, next, ok := cursor.lastAdvance()
	if !ok || next != 1800 {
		t.Fatalf("advance next = %d ok=%v, want 1800", next, ok)
	}
}

func TestHandoffS16_CaughtUpNoGetBlocks(t *testing.T) {
	// S-16 / AC-085 caught-up half
	t.Parallel()

	reader := newHandoffReader(1000)
	cursor := newFakeCursor(1000, true)
	prod := &fakeProducer{}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   77 * time.Millisecond,
		StepSlots:      20000,
		SlotsPerWorker: 500,
	})

	var waited []time.Duration
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.wait = func(ctx context.Context, d time.Duration) error {
		waited = append(waited, d)
		cancel()
		return ctx.Err()
	}

	err := s.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if reader.blockSlotsCallCount() != 0 {
		t.Fatalf("BlockSlots calls = %d, want 0", reader.blockSlotsCallCount())
	}
	if len(waited) != 1 || waited[0] != 77*time.Millisecond {
		t.Fatalf("waits = %v, want one pollInterval", waited)
	}
	if cursor.advanceCount() != 0 {
		t.Fatalf("AdvanceHandedSlot calls = %d, want 0", cursor.advanceCount())
	}
}

func TestStepProduceFailureNoAdvance(t *testing.T) {
	// AC-095
	t.Parallel()

	reader := newHandoffReader(102)
	reader.slots[[2]uint64{101, 102}] = []uint64{101, 102}
	reader.setBlock(101, minimalBlock(101))
	reader.setBlock(102, minimalBlock(102))

	cursor := newFakeCursor(100, true)
	prod := &fakeProducer{err: errors.New("kafka down")}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Second,
		StepSlots:      2,
		SlotsPerWorker: 2,
	})

	err := s.Step(context.Background())
	if err == nil {
		t.Fatal("Step error = nil, want produce failure")
	}
	if cursor.advanceCount() != 0 {
		t.Fatalf("AdvanceHandedSlot calls = %d, want 0", cursor.advanceCount())
	}
	slot, ok := cursor.handedSlot()
	if !ok || slot != 100 {
		t.Fatalf("handed_slot = %d ok=%v, want 100", slot, ok)
	}
}

func TestStepUnsupportedVersionFatalNoAdvance(t *testing.T) {
	// AC-069 / T105 UNSUPPORTED_VERSION
	t.Parallel()

	reader := newHandoffReader(102)
	reader.slots[[2]uint64{101, 102}] = []uint64{101}
	reader.blockErrs[101] = provider.Error{
		Operation: "getBlock",
		Kind:      provider.ErrorKindUnsupportedVersion,
	}

	cursor := newFakeCursor(100, true)
	prod := &fakeProducer{}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Second,
		StepSlots:      2,
		SlotsPerWorker: 2,
	})

	err := s.Step(context.Background())
	if err == nil {
		t.Fatal("Step error = nil, want UNSUPPORTED_VERSION")
	}
	if provider.KindOf(err) != provider.ErrorKindUnsupportedVersion {
		t.Fatalf("KindOf = %v, want UNSUPPORTED_VERSION", provider.KindOf(err))
	}
	if cursor.advanceCount() != 0 {
		t.Fatalf("AdvanceHandedSlot calls = %d, want 0", cursor.advanceCount())
	}
}

func TestHandoffProductionCodeImports(t *testing.T) {
	// AC-094 scanner half
	t.Parallel()

	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
	forbidden := []string{
		"github.com/Stone-afk/sol-scanner-service/internal/processor/parser",
		"github.com/Stone-afk/sol-scanner-service/internal/parser",
		"github.com/Stone-afk/sol-scanner-service/internal/event/transaction",
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", path, err)
		}
		if strings.Contains(string(src), "SaveBlockBatch") {
			t.Fatalf("%s references SaveBlockBatch", path)
		}
		file, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("ParseFile %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			pathLit := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbidden {
				if pathLit == bad {
					t.Fatalf("%s imports %s (AC-094)", path, bad)
				}
			}
		}
	}
}

func newTestHandoff(
	t *testing.T,
	reader provider.Reader,
	prod BlockProducer,
	cursor *fakeCursor,
	cfg Config,
) *Scanner {
	t.Helper()
	s := New(reader, prod, cursor, &fakeHaltChecker{}, cfg, slog.Default())
	s.wait = func(context.Context, time.Duration) error { return nil }
	return s
}

func assertSegmentCalls(t *testing.T, got, want [][2]uint64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("BlockSlots calls = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	sortedGot := append([][2]uint64(nil), got...)
	sortedWant := append([][2]uint64(nil), want...)
	sortSegments(sortedGot)
	sortSegments(sortedWant)
	for i := range sortedWant {
		if sortedGot[i] != sortedWant[i] {
			t.Fatalf("segments = %v, want %v", sortedGot, sortedWant)
		}
	}
}

func sortSegments(segs [][2]uint64) {
	for i := 0; i < len(segs); i++ {
		for j := i + 1; j < len(segs); j++ {
			if segs[j][0] < segs[i][0] {
				segs[i], segs[j] = segs[j], segs[i]
			}
		}
	}
}

func minimalBlock(slot uint64) *rpc.GetBlockResult {
	var h solana.Hash
	binary.BigEndian.PutUint64(h[24:], slot)
	h[0] = 1
	var prev solana.Hash
	binary.BigEndian.PutUint64(prev[24:], slot-1)
	prev[0] = 1
	return &rpc.GetBlockResult{
		Blockhash:         h,
		PreviousBlockhash: prev,
		ParentSlot:        slot - 1,
	}
}

func TestStepBlockMessageRoundTripJSON(t *testing.T) {
	// sanity: produced block payload is valid GetBlockResult JSON
	t.Parallel()

	reader := newHandoffReader(101)
	reader.slots[[2]uint64{101, 101}] = []uint64{101}
	raw := minimalBlock(101)
	reader.setBlock(101, raw)

	cursor := newFakeCursor(100, true)
	prod := &fakeProducer{}
	s := newTestHandoff(t, reader, prod, cursor, Config{
		Chain:          "solana-mainnet",
		GenesisHash:    "genesis-a",
		PollInterval:   time.Second,
		StepSlots:      1,
		SlotsPerWorker: 1,
	})
	if err := s.Step(context.Background()); err != nil {
		t.Fatalf("Step: %v", err)
	}
	msgs := prod.messages()
	if len(msgs) != 1 || msgs[0].Kind != block.KindBlock {
		t.Fatalf("msgs = %+v", msgs)
	}
	var decoded rpc.GetBlockResult
	if err := json.Unmarshal(msgs[0].Block, &decoded); err != nil {
		t.Fatalf("unmarshal block JSON: %v", err)
	}
}
