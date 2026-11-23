//go:build e2e

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/pkg/mqx"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type mqxE2EPayload struct {
	ID string `json:"id"`
}

func TestMqxProduceBatchKeyedDelivery(t *testing.T) {
	// AC-052
	brokers := kafkaBrokers(t)
	topic := fmt.Sprintf("mqx-e2e-%d", time.Now().UnixNano())

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

	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"group.id":           fmt.Sprintf("mqx-e2e-cg-%d", time.Now().UnixNano()),
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
	})
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	defer consumer.Close()
	if err := consumer.Subscribe(topic, nil); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	kp, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"message.timeout.ms": 1000,
	})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	prod, err := mqx.NewGeneralProducer[mqxE2EPayload](kp, topic)
	if err != nil {
		t.Fatalf("NewGeneralProducer: %v", err)
	}
	defer prod.Close()

	want := []mqx.Keyed[mqxE2EPayload]{
		{Key: "sig-a", Value: mqxE2EPayload{ID: "a"}},
		{Key: "sig-b", Value: mqxE2EPayload{ID: "b"}},
		{Key: "sig-c", Value: mqxE2EPayload{ID: "c"}},
	}
	if err := prod.ProduceBatch(context.Background(), want); err != nil {
		t.Fatalf("ProduceBatch: %v", err)
	}

	got := make(map[string]mqxE2EPayload, len(want))
	deadline := time.Now().Add(30 * time.Second)
	for len(got) < len(want) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d messages, got %d", len(want), len(got))
		}
		msg, err := consumer.ReadMessage(2 * time.Second)
		if err != nil {
			var kErr kafka.Error
			if errors.As(err, &kErr) && kErr.Code() == kafka.ErrTimedOut {
				continue
			}
			t.Fatalf("ReadMessage: %v", err)
		}
		key := string(msg.Key)
		var payload mqxE2EPayload
		if err := json.Unmarshal(msg.Value, &payload); err != nil {
			t.Fatalf("unmarshal value for key %q: %v", key, err)
		}
		got[key] = payload
	}

	for _, m := range want {
		payload, ok := got[m.Key]
		if !ok {
			t.Fatalf("missing consumed key %q; got=%v", m.Key, got)
		}
		if payload != m.Value {
			t.Fatalf("key %q payload = %#v, want %#v", m.Key, payload, m.Value)
		}
	}
}
