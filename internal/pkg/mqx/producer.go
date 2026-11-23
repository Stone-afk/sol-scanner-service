package mqx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

var _ Producer[any] = (*GeneralProducer[any])(nil)

// GeneralProducer is a keyed batch Kafka producer. Pointer receiver is allowed
// because it owns a connection and lifecycle.
type GeneralProducer[T any] struct {
	producer *kafka.Producer
	topic    string
}

// NewGeneralProducer wraps an existing kafka.Producer for topic.
func NewGeneralProducer[T any](producer *kafka.Producer, topic string) (*GeneralProducer[T], error) {
	if producer == nil {
		return nil, errors.New("kafka producer is nil")
	}
	if topic == "" {
		return nil, errors.New("topic is empty")
	}
	return &GeneralProducer[T]{
		producer: producer,
		topic:    topic,
	}, nil
}

// ProduceBatch JSON-encodes each value, sets Key on every message, enqueues the
// whole batch, then waits until every delivery report succeeds (or ctx cancels).
func (p *GeneralProducer[T]) ProduceBatch(ctx context.Context, msgs []Keyed[T]) error {
	if len(msgs) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	deliveryChan := make(chan kafka.Event, len(msgs))
	for _, msg := range msgs {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := json.Marshal(msg.Value)
		if err != nil {
			return fmt.Errorf("marshal message: %w", err)
		}
		if err := p.enqueue(ctx, msg.Key, data, deliveryChan); err != nil {
			return err
		}
	}

	remaining := len(msgs)
	for remaining > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-deliveryChan:
			if err := deliveryError(ev); err != nil {
				return err
			}
			remaining--
		case <-time.After(10 * time.Millisecond):
			flushDone := make(chan struct{})
			go func() {
				p.producer.Flush(FlushTimeoutMs)
				close(flushDone)
			}()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-flushDone:
			case ev := <-deliveryChan:
				if err := deliveryError(ev); err != nil {
					return err
				}
				remaining--
			}
		}
	}
	return nil
}

func (p *GeneralProducer[T]) enqueue(ctx context.Context, key string, data []byte, deliveryChan chan kafka.Event) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := p.producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &p.topic, Partition: kafka.PartitionAny},
			Key:            []byte(key),
			Value:          data,
		}, deliveryChan)
		if err == nil {
			return nil
		}
		var kafkaErr kafka.Error
		if errors.As(err, &kafkaErr) && kafkaErr.Code() == kafka.ErrQueueFull {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
				continue
			}
		}
		return fmt.Errorf("produce to topic %s: %w", p.topic, err)
	}
}

func deliveryError(ev kafka.Event) error {
	switch e := ev.(type) {
	case *kafka.Message:
		if e.TopicPartition.Error != nil {
			return fmt.Errorf("delivery failed: %w", e.TopicPartition.Error)
		}
		return nil
	case kafka.Error:
		return fmt.Errorf("kafka error: %w", e)
	default:
		return fmt.Errorf("unknown delivery event: %s", ev.String())
	}
}

// Close flushes outstanding messages with a bounded Flush timeout, then closes.
func (p *GeneralProducer[T]) Close() {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p.producer.Flush(FlushTimeoutMs) == 0 {
			break
		}
	}
	p.producer.Close()
}
