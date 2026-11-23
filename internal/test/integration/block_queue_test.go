//go:build e2e

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func TestBlockQueueRoundTrip(t *testing.T) {
	// AC-083: one block + one skipped round-trip through real Kafka
	brokers := kafkaBrokers(t)
	topic := fmt.Sprintf("block-queue-e2e-%d", time.Now().UnixNano())
	groupID := fmt.Sprintf("block-queue-cg-%d", time.Now().UnixNano())

	admin, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
	})
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer admin.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	results, err := admin.CreateTopics(ctx, []kafka.TopicSpecification{{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	}})
	if err != nil {
		t.Fatalf("CreateTopics: %v", err)
	}
	for _, r := range results {
		if r.Error.Code() != kafka.ErrNoError && r.Error.Code() != kafka.ErrTopicAlreadyExists {
			t.Fatalf("CreateTopics %s: %v", r.Topic, r.Error)
		}
	}

	// IoC-style: create Kafka clients here, inject into event constructors.
	kp, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"message.timeout.ms": 5000,
	})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	prod, err := block.NewProducer(kp, topic)
	if err != nil {
		t.Fatalf("block.NewProducer: %v", err)
	}
	defer prod.Close()

	blockJSON := []byte(`{"blockhash":"e2e-hash","parentSlot":100}`)
	want := []block.BlockMessage{
		{
			Version:    1,
			Chain:      "solana-mainnet",
			Slot:       102,
			Commitment: "finalized",
			Kind:       "skipped",
		},
		{
			Version:    1,
			Chain:      "solana-mainnet",
			Slot:       103,
			Commitment: "finalized",
			Kind:       "block",
			Block:      blockJSON,
		},
	}
	if err := prod.Produce(context.Background(), want); err != nil {
		t.Fatalf("Produce: %v", err)
	}

	kc, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"group.id":           groupID,
		"enable.auto.commit": false,
		"auto.offset.reset":  "earliest",
	})
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	cons, err := block.NewConsumer(kc, topic, slog.Default())
	if err != nil {
		_ = kc.Close()
		t.Fatalf("block.NewConsumer: %v", err)
	}
	defer func() { _ = cons.Close() }()

	got := make(map[uint64]block.BlockMessage, len(want))
	deadline := time.Now().Add(30 * time.Second)
	for len(got) < len(want) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d messages, got %d", len(want), len(got))
		}
		pollCtx, pollCancel := context.WithTimeout(context.Background(), 2*time.Second)
		msg, off, err := cons.Poll(pollCtx)
		pollCancel()
		if err != nil {
			continue
		}
		got[msg.Slot] = msg
		if err := cons.Commit(context.Background(), off); err != nil {
			t.Fatalf("Commit slot %d: %v", msg.Slot, err)
		}
	}

	skipped, ok := got[102]
	if !ok {
		t.Fatalf("missing skipped slot 102; got=%v", slotsOf(got))
	}
	if skipped.Kind != "skipped" {
		t.Fatalf("slot 102 kind = %q, want skipped", skipped.Kind)
	}
	if len(skipped.Block) != 0 {
		t.Fatalf("slot 102 must have empty Block, got %s", skipped.Block)
	}
	rawSkipped, err := block.Encode(skipped)
	if err != nil {
		t.Fatalf("Encode skipped: %v", err)
	}
	var skippedObj map[string]json.RawMessage
	if err := json.Unmarshal(rawSkipped, &skippedObj); err != nil {
		t.Fatalf("unmarshal skipped: %v", err)
	}
	if _, hasBlock := skippedObj["block"]; hasBlock {
		t.Fatal("skipped re-encode must omit block")
	}

	blk, ok := got[103]
	if !ok {
		t.Fatalf("missing block slot 103; got=%v", slotsOf(got))
	}
	if blk.Kind != "block" {
		t.Fatalf("slot 103 kind = %q, want block", blk.Kind)
	}
	if string(blk.Block) != string(blockJSON) && !jsonRawEqual(blk.Block, blockJSON) {
		t.Fatalf("slot 103 block = %s, want %s", blk.Block, blockJSON)
	}
}

func slotsOf(m map[uint64]block.BlockMessage) []uint64 {
	out := make([]uint64, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	return out
}

func jsonRawEqual(a, b []byte) bool {
	var xa, xb any
	if err := json.Unmarshal(a, &xa); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &xb); err != nil {
		return false
	}
	ra, _ := json.Marshal(xa)
	rb, _ := json.Marshal(xb)
	return string(ra) == string(rb)
}
