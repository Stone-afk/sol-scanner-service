package block

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/pkg/mqx"
)

type fakeMQX struct {
	batches [][]mqx.Keyed[BlockMessage]
	err     error
}

func (f *fakeMQX) ProduceBatch(_ context.Context, msgs []mqx.Keyed[BlockMessage]) error {
	if f.err != nil {
		return f.err
	}
	cp := make([]mqx.Keyed[BlockMessage], len(msgs))
	copy(cp, msgs)
	f.batches = append(f.batches, cp)
	return nil
}

func (f *fakeMQX) Close() {}

func TestBlockProducerKeysAreDecimalSlots(t *testing.T) {
	// AC-083
	t.Parallel()
	fake := &fakeMQX{}
	prod := NewProducerFromMQX(fake)

	blockJSON := []byte(`{"blockhash":"x"}`)
	msgs := []BlockMessage{
		{
			Version:    1,
			Chain:      testChain,
			Slot:       102,
			Commitment: "finalized",
			Kind:       "skipped",
		},
		{
			Version:    1,
			Chain:      testChain,
			Slot:       fixtureSlot,
			Commitment: "finalized",
			Kind:       "block",
			Block:      blockJSON,
		},
	}
	if err := prod.Produce(context.Background(), msgs); err != nil {
		t.Fatalf("Produce: %v", err)
	}
	if len(fake.batches) != 1 {
		t.Fatalf("batches = %d, want 1", len(fake.batches))
	}
	got := fake.batches[0]
	if len(got) != 2 {
		t.Fatalf("keyed len = %d, want 2", len(got))
	}
	if got[0].Key != "102" {
		t.Fatalf("key[0] = %q, want %q", got[0].Key, "102")
	}
	if got[1].Key != "471991643" {
		t.Fatalf("key[1] = %q, want %q", got[1].Key, "471991643")
	}
	if got[0].Value.Kind != "skipped" || len(got[0].Value.Block) != 0 {
		t.Fatalf("value[0] = %+v", got[0].Value)
	}
	if got[1].Value.Kind != "block" || !bytesEqual(got[1].Value.Block, blockJSON) {
		t.Fatalf("value[1].Block = %s", got[1].Value.Block)
	}
}

func TestBlockProducerPropagatesMQXError(t *testing.T) {
	// AC-083
	t.Parallel()
	wantErr := errors.New("mqx boom")
	fake := &fakeMQX{err: wantErr}
	prod := NewProducerFromMQX(fake)

	err := prod.Produce(context.Background(), []BlockMessage{{
		Version:    1,
		Chain:      testChain,
		Slot:       1,
		Commitment: "finalized",
		Kind:       "skipped",
	}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Produce err = %v, want %v", err, wantErr)
	}
	if len(fake.batches) != 0 {
		t.Fatalf("ProduceBatch must not record success on error; batches=%d", len(fake.batches))
	}
}

func TestBlockProducerEmptyBatchOK(t *testing.T) {
	t.Parallel()
	fake := &fakeMQX{}
	prod := NewProducerFromMQX(fake)
	if err := prod.Produce(context.Background(), nil); err != nil {
		t.Fatalf("Produce(nil): %v", err)
	}
	if err := prod.Produce(context.Background(), []BlockMessage{}); err != nil {
		t.Fatalf("Produce(empty): %v", err)
	}
}

func TestBlockProducerEncodedValueIsObjectBlock(t *testing.T) {
	// Ensure Keyed values marshal with object block when used via mqx JSON path.
	t.Parallel()
	fake := &fakeMQX{}
	prod := NewProducerFromMQX(fake)
	blockJSON := []byte(`{"parentSlot":1}`)
	if err := prod.Produce(context.Background(), []BlockMessage{{
		Version:    1,
		Chain:      testChain,
		Slot:       9,
		Commitment: "finalized",
		Kind:       "block",
		Block:      blockJSON,
	}}); err != nil {
		t.Fatalf("Produce: %v", err)
	}
	raw, err := json.Marshal(fake.batches[0][0].Value)
	if err != nil {
		t.Fatalf("json.Marshal BlockMessage: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	blockField := obj["block"]
	if len(blockField) == 0 || blockField[0] != '{' {
		t.Fatalf("marshaled block must be object, got %s", blockField)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
