package transaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/mqx"
)

const testPublishTopic = "sol-scanner.transactions"

type fakeMQX struct {
	mu      sync.Mutex
	batches [][]mqx.Keyed[TransactionMessage]
	rawJSON [][][]byte
	errs    []error // per ProduceBatch call; exhausted → nil
	calls   int
}

func (f *fakeMQX) ProduceBatch(_ context.Context, msgs []mqx.Keyed[TransactionMessage]) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.calls
	f.calls++
	var err error
	if idx < len(f.errs) {
		err = f.errs[idx]
	}
	cp := make([]mqx.Keyed[TransactionMessage], len(msgs))
	copy(cp, msgs)
	f.batches = append(f.batches, cp)
	raw := make([][]byte, len(msgs))
	for i, m := range msgs {
		b, marshalErr := json.Marshal(m.Value)
		if marshalErr != nil {
			return marshalErr
		}
		raw[i] = b
	}
	f.rawJSON = append(f.rawJSON, raw)
	return err
}

func (f *fakeMQX) Close() {}

func (f *fakeMQX) snapshot() (batches [][]mqx.Keyed[TransactionMessage], raw [][][]byte, calls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]mqx.Keyed[TransactionMessage](nil), f.batches...),
		append([][][]byte(nil), f.rawJSON...),
		f.calls
}

type fakePublishRepo struct {
	mu     sync.Mutex
	cursor uint64
	ok     bool
	topic  string
	getErr error

	batches []domain.BlockBatch
	listN   int

	advances []uint64
}

func (r *fakePublishRepo) GetPublishCursor(_ context.Context, topic string) (uint64, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return 0, false, r.getErr
	}
	if r.ok && r.topic != "" && r.topic != topic {
		return 0, false, errors.New("topic mismatch")
	}
	return r.cursor, r.ok, nil
}

func (r *fakePublishRepo) ListBlockBatches(_ context.Context, afterSlot uint64, limit int) ([]domain.BlockBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listN++
	var out []domain.BlockBatch
	for _, b := range r.batches {
		if b.Block.Slot <= afterSlot {
			continue
		}
		out = append(out, b)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *fakePublishRepo) AdvancePublishCursor(_ context.Context, _ string, slot uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ok && slot <= r.cursor {
		return errors.New("cursor cannot move backward")
	}
	r.cursor = slot
	r.ok = true
	r.topic = testPublishTopic
	r.advances = append(r.advances, slot)
	return nil
}

func (r *fakePublishRepo) snapshot() (cursor uint64, ok bool, advances []uint64, listN int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cursor, r.ok, append([]uint64(nil), r.advances...), r.listN
}

func TestPublishMidBatchFailLeavesCursorAndRetryIdentical(t *testing.T) {
	// AC-059
	t.Parallel()
	batch := domain.BlockBatch{
		Block: domain.Block{
			Slot: 200, Blockhash: "Hash200", HasBlockTime: true, BlockTime: 1700000000,
		},
		Transactions: []domain.Transaction{
			{Slot: 200, TxIndex: 0, Signature: "SigOk1", Status: domain.TxStatusSuccess, Version: domain.TxVersion0, Fee: 1, FeePayer: "P1"},
			{Slot: 200, TxIndex: 1, Signature: "SigFail", Status: domain.TxStatusFailed, Version: domain.TxVersion0, Fee: 2, FeePayer: "P2"},
			{Slot: 200, TxIndex: 2, Signature: "SigOk2", Status: domain.TxStatusSuccess, Version: domain.TxVersion0, Fee: 3, FeePayer: "P3"},
		},
		Transfers: []domain.Transfer{
			{Slot: 200, TxIndex: 0, IxIndex: 0, InnerIndex: domain.TopLevelInnerIndex, Program: domain.ProgramSystem, Kind: domain.KindTransfer, FromAccount: "A", ToAccount: "B", Amount: 9},
			{Slot: 200, TxIndex: 0, IxIndex: 1, InnerIndex: domain.TopLevelInnerIndex, Program: domain.ProgramSystem, Kind: domain.KindTransfer, FromAccount: "C", ToAccount: "D", Amount: 8},
		},
	}
	repo := &fakePublishRepo{batches: []domain.BlockBatch{batch}}
	boom := errors.New("kafka mid-batch fail")
	fake := &fakeMQX{errs: []error{boom, nil}}
	prod := NewProducerFromMQX(fake, testPublishTopic, repo, testChain, slog.Default())

	var waits []time.Duration
	prod.wait = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- prod.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, _, calls := fake.snapshot()
		_, _, advances, _ := repo.snapshot()
		if calls >= 2 && len(advances) >= 1 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("timeout: calls=%d advances=%v", calls, advances)
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	_, raw, calls := fake.snapshot()
	cursor, ok, advances, _ := repo.snapshot()
	if calls < 2 {
		t.Fatalf("ProduceBatch calls=%d want >= 2", calls)
	}
	if len(raw) < 2 || len(raw[0]) != 3 || len(raw[1]) != 3 {
		t.Fatalf("raw batches malformed: %#v", raw)
	}
	for i := range raw[0] {
		if !bytes.Equal(raw[0][i], raw[1][i]) {
			t.Fatalf("retry JSON[%d] not byte-identical:\nfirst=%s\nretry=%s", i, raw[0][i], raw[1][i])
		}
	}
	if !ok || cursor != 200 {
		t.Fatalf("cursor after success: ok=%v slot=%d", ok, cursor)
	}
	if len(advances) != 1 || advances[0] != 200 {
		t.Fatalf("advances=%v want single 200 (no advance on first failure)", advances)
	}
	if len(waits) < 1 || waits[0] != time.Second {
		t.Fatalf("first backoff waits=%v want leading 1s", waits)
	}
}

func TestPublishEmptyTxBlockAdvancesWithoutSend(t *testing.T) {
	// AC-060
	t.Parallel()
	empty := domain.BlockBatch{
		Block: domain.Block{Slot: 300, Blockhash: "Hash300", TransactionCount: 0},
	}
	repo := &fakePublishRepo{batches: []domain.BlockBatch{empty}}
	fake := &fakeMQX{}
	prod := NewProducerFromMQX(fake, testPublishTopic, repo, testChain, slog.Default())
	prod.wait = func(ctx context.Context, _ time.Duration) error {
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- prod.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, ok, advances, _ := repo.snapshot()
		if ok && len(advances) >= 1 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("timeout waiting for empty-block advance")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	batches, _, calls := fake.snapshot()
	cursor, ok, advances, _ := repo.snapshot()
	if calls != 0 || len(batches) != 0 {
		t.Fatalf("empty block must not ProduceBatch; calls=%d batches=%d", calls, len(batches))
	}
	if !ok || cursor != 300 || len(advances) != 1 || advances[0] != 300 {
		t.Fatalf("cursor=%d ok=%v advances=%v", cursor, ok, advances)
	}
}

func TestPublishBackoffSequenceAndCancel(t *testing.T) {
	// AC-063
	t.Parallel()
	batch := domain.BlockBatch{
		Block: domain.Block{Slot: 400, Blockhash: "Hash400"},
		Transactions: []domain.Transaction{
			{Slot: 400, TxIndex: 0, Signature: "Sig400", Status: domain.TxStatusSuccess, Version: domain.TxVersion0, Fee: 1, FeePayer: "P"},
		},
	}
	repo := &fakePublishRepo{batches: []domain.BlockBatch{batch}}
	forever := errors.New("always fail")
	fake := &fakeMQX{errs: []error{forever, forever, forever, forever, forever, forever}}
	prod := NewProducerFromMQX(fake, testPublishTopic, repo, testChain, slog.Default())

	var (
		mu    sync.Mutex
		waits []time.Duration
	)
	enteredThird := make(chan struct{})
	prod.wait = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		waits = append(waits, d)
		n := len(waits)
		mu.Unlock()
		if n == 3 {
			close(enteredThird)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- prod.Run(ctx) }()

	select {
	case <-enteredThird:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timeout waiting for third backoff")
	}
	start := time.Now()
	cancel()
	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel during backoff did not return immediately")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("cancel during backoff took %s, want immediate return", time.Since(start))
	}
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run err=%v want context.Canceled", runErr)
	}

	mu.Lock()
	got := append([]time.Duration(nil), waits...)
	mu.Unlock()
	wantPrefix := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if len(got) < 3 {
		t.Fatalf("waits=%v want at least %v", got, wantPrefix)
	}
	for i, w := range wantPrefix {
		if got[i] != w {
			t.Fatalf("waits=%v want prefix %v", got, wantPrefix)
		}
	}

	// Cap at 30s: keep failing enough times via a separate producer.
	repo2 := &fakePublishRepo{batches: []domain.BlockBatch{batch}}
	errs := make([]error, 20)
	for i := range errs {
		errs[i] = forever
	}
	fake2 := &fakeMQX{errs: errs}
	prod2 := NewProducerFromMQX(fake2, testPublishTopic, repo2, testChain, slog.Default())
	var waits2 []time.Duration
	prod2.wait = func(ctx context.Context, d time.Duration) error {
		waits2 = append(waits2, d)
		if len(waits2) >= 10 {
			return context.Canceled
		}
		return nil
	}
	_ = prod2.Run(context.Background())
	if len(waits2) < 10 {
		t.Fatalf("waits2 len=%d", len(waits2))
	}
	// 1,2,4,8,16,30,30,30,30,30
	wantCap := []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
		30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second,
	}
	for i, w := range wantCap {
		if waits2[i] != w {
			t.Fatalf("capped waits=%v want %v", waits2[:len(wantCap)], wantCap)
		}
	}
	_, ok, advances, _ := repo2.snapshot()
	if ok || len(advances) != 0 {
		t.Fatalf("cursor must not advance while failing: ok=%v advances=%v", ok, advances)
	}
}

func TestPublishBackoffSequenceAndReset(t *testing.T) {
	// AC-S05 — capped exponential backoff, reset to 1s after a success.
	t.Parallel()
	tx := func(slot uint64, sig string) domain.Transaction {
		return domain.Transaction{Slot: slot, TxIndex: 0, Signature: sig, Status: domain.TxStatusSuccess, Version: domain.TxVersion0, Fee: 1, FeePayer: "P"}
	}
	repo := &fakePublishRepo{batches: []domain.BlockBatch{
		{Block: domain.Block{Slot: 500, Blockhash: "Hash500"}, Transactions: []domain.Transaction{tx(500, "Sig500")}},
		{Block: domain.Block{Slot: 501, Blockhash: "Hash501"}, Transactions: []domain.Transaction{tx(501, "Sig501")}},
	}}
	fail := errors.New("kafka down")
	// slot 500: 7 failures then success; slot 501: 1 failure then success.
	fake := &fakeMQX{errs: []error{fail, fail, fail, fail, fail, fail, fail, nil, fail, nil}}
	prod := NewProducerFromMQX(fake, testPublishTopic, repo, testChain, slog.Default())

	var backoffs []time.Duration
	prod.wait = func(_ context.Context, d time.Duration) error {
		if d == publishIdleWait {
			return context.Canceled // drained: stop at the first idle poll
		}
		backoffs = append(backoffs, d)
		return nil
	}

	if err := prod.Run(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run err=%v want context.Canceled from idle wait", err)
	}
	want := []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
		30 * time.Second, 30 * time.Second,
		time.Second,
	}
	if len(backoffs) != len(want) {
		t.Fatalf("backoff waits=%v want %v", backoffs, want)
	}
	for i := range want {
		if backoffs[i] != want[i] {
			t.Fatalf("backoff waits=%v want %v", backoffs, want)
		}
	}
	_, _, advances, _ := repo.snapshot()
	if len(advances) != 2 || advances[0] != 500 || advances[1] != 501 {
		t.Fatalf("advances=%v want [500 501]", advances)
	}
}
