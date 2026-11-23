package mqx

import (
	"context"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// Unreachable broker for offline tests (no Compose required).
const unreachableBroker = "127.0.0.1:1"

type testPayload struct {
	N int `json:"n"`
}

func newUnreachableProducer(t *testing.T) *kafka.Producer {
	t.Helper()
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  unreachableBroker,
		"message.timeout.ms": 1000,
		"socket.timeout.ms":  1000,
	})
	if err != nil {
		t.Fatalf("kafka.NewProducer: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestProduceBatchUnreachableBroker(t *testing.T) {
	// AC-053
	kp := newUnreachableProducer(t)
	prod, err := NewGeneralProducer[testPayload](kp, "mqx-offline-unreachable")
	if err != nil {
		t.Fatalf("NewGeneralProducer: %v", err)
	}

	err = prod.ProduceBatch(context.Background(), []Keyed[testPayload]{
		{Key: "k1", Value: testPayload{N: 1}},
	})
	if err == nil {
		t.Fatal("ProduceBatch: want non-nil error when broker unreachable")
	}
}

func TestProduceBatchCancelWithin2s(t *testing.T) {
	// AC-054
	kp := newUnreachableProducer(t)
	prod, err := NewGeneralProducer[testPayload](kp, "mqx-offline-cancel")
	if err != nil {
		t.Fatalf("NewGeneralProducer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err = prod.ProduceBatch(ctx, []Keyed[testPayload]{
		{Key: "k1", Value: testPayload{N: 1}},
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ProduceBatch: want error after ctx cancel")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("ProduceBatch returned after %v, want <= 2s on cancel", elapsed)
	}
}

func TestProduceBatchEmptyReturnsNil(t *testing.T) {
	// AC-055
	kp := newUnreachableProducer(t)
	prod, err := NewGeneralProducer[testPayload](kp, "mqx-offline-empty")
	if err != nil {
		t.Fatalf("NewGeneralProducer: %v", err)
	}

	if err := prod.ProduceBatch(context.Background(), nil); err != nil {
		t.Fatalf("ProduceBatch(nil) = %v, want nil", err)
	}
	if err := prod.ProduceBatch(context.Background(), []Keyed[testPayload]{}); err != nil {
		t.Fatalf("ProduceBatch(empty) = %v, want nil", err)
	}
}

func TestCloseBoundedOnUnreachableBroker(t *testing.T) {
	// AC-055
	kp, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  unreachableBroker,
		"message.timeout.ms": 1000,
		"socket.timeout.ms":  1000,
	})
	if err != nil {
		t.Fatalf("kafka.NewProducer: %v", err)
	}
	prod, err := NewGeneralProducer[testPayload](kp, "mqx-offline-close")
	if err != nil {
		t.Fatalf("NewGeneralProducer: %v", err)
	}

	start := time.Now()
	prod.Close()
	elapsed := time.Since(start)
	if elapsed > 6*time.Second {
		t.Fatalf("Close returned after %v, want <= 6s on unreachable broker", elapsed)
	}
}

func TestFlushTimeoutMsIs1000(t *testing.T) {
	// AC-055: Flush 参数必须是 1000（毫秒）
	if FlushTimeoutMs != 1000 {
		t.Fatalf("FlushTimeoutMs = %d, want 1000", FlushTimeoutMs)
	}
}
