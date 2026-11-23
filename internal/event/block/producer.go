package block

import (
	"context"

	"github.com/Stone-afk/sol-scanner-service/internal/pkg/mqx"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// Producer publishes BlockMessage batches via mqx.
// Pointer receiver: owns the underlying Kafka producer lifecycle through mqx.
type Producer struct {
	inner mqx.Producer[BlockMessage]
}

// NewProducer wraps *kafka.Producer for topic, matching notification-platform/internal/event:
// domain event constructors take *kafka.Producer + topic and delegate to mqx.
func NewProducer(producer *kafka.Producer, topic string) (*Producer, error) {
	inner, err := mqx.NewGeneralProducer[BlockMessage](producer, topic)
	if err != nil {
		return nil, err
	}
	return &Producer{inner: inner}, nil
}

// NewProducerFromMQX is for tests that inject a fake mqx.Producer.
func NewProducerFromMQX(inner mqx.Producer[BlockMessage]) *Producer {
	return &Producer{inner: inner}
}

// Produce maps messages to keyed mqx batches (key = decimal slot) and ProduceBatch.
// Does not advance cursors or write databases.
func (p *Producer) Produce(ctx context.Context, msgs []BlockMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	keyed := make([]mqx.Keyed[BlockMessage], len(msgs))
	for i, msg := range msgs {
		keyed[i] = mqx.Keyed[BlockMessage]{
			Key:   Key(msg.Slot),
			Value: msg,
		}
	}
	return p.inner.ProduceBatch(ctx, keyed)
}

// Close closes the underlying mqx producer.
func (p *Producer) Close() {
	p.inner.Close()
}
