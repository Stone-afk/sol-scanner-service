package block

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// Offset is a Kafka topic-partition offset. Value semantics.
type Offset struct {
	Topic     string
	Partition int32
	Offset    int64
}

// OffsetFromTopicPartition converts a confluent TopicPartition into Offset.
func OffsetFromTopicPartition(tp kafka.TopicPartition) Offset {
	topic := ""
	if tp.Topic != nil {
		topic = *tp.Topic
	}
	return Offset{
		Topic:     topic,
		Partition: tp.Partition,
		Offset:    int64(tp.Offset),
	}
}

// TopicPartition converts Offset back to confluent TopicPartition.
func (o Offset) TopicPartition() kafka.TopicPartition {
	topic := o.Topic
	return kafka.TopicPartition{
		Topic:     &topic,
		Partition: o.Partition,
		Offset:    kafka.Offset(o.Offset),
	}
}

// Consumer reads BlockMessage values with auto-commit disabled.
// Pointer receiver: owns the injected Kafka consumer connection.
// Construction matches notification-platform/internal/event: IoC creates
// *kafka.Consumer; this package only Subscribe + domain Poll/Commit/Seek.
type Consumer struct {
	consumer *kafka.Consumer
	topic    string
	logger   *slog.Logger
}

// NewConsumer wraps an already-configured *kafka.Consumer and subscribes to topic.
// Caller must set enable.auto.commit=false before passing the consumer in.
func NewConsumer(consumer *kafka.Consumer, topic string, logger *slog.Logger) (*Consumer, error) {
	if consumer == nil {
		return nil, errors.New("kafka consumer is nil")
	}
	if topic == "" {
		return nil, errors.New("topic is empty")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := consumer.SubscribeTopics([]string{topic}, nil); err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", topic, err)
	}
	logger.Info("block consumer subscribed", "topic", topic)
	return &Consumer{consumer: consumer, topic: topic, logger: logger}, nil
}

// ErrQueueNotReady means the block topic is not visible yet (auto-create race).
// Apply must wait without counting a gap miss or exiting the process.
var ErrQueueNotReady = errors.New("block queue topic not ready")

// isRetriablePollError is true for broker/local states that clear once the topic exists.
func isRetriablePollError(e kafka.Error) bool {
	switch e.Code() {
	case kafka.ErrUnknownTopic, kafka.ErrUnknownTopicOrPart:
		return true
	default:
		return false
	}
}

// Poll reads the next message, returning the decoded BlockMessage and its offset.
// Does not commit. Caller must Commit or Seek explicitly.
func (c *Consumer) Poll(ctx context.Context) (BlockMessage, Offset, error) {
	for {
		if err := ctx.Err(); err != nil {
			return BlockMessage{}, Offset{}, err
		}
		timeout := 100 * time.Millisecond
		if deadline, ok := ctx.Deadline(); ok {
			remain := time.Until(deadline)
			if remain <= 0 {
				return BlockMessage{}, Offset{}, context.DeadlineExceeded
			}
			if remain < timeout {
				timeout = remain
			}
		}
		ev := c.consumer.Poll(int(timeout / time.Millisecond))
		if ev == nil {
			continue
		}
		switch e := ev.(type) {
		case *kafka.Message:
			msg, err := Decode(e.Value)
			if err != nil {
				c.logger.Error("decode block message failed",
					"err", err,
					"topic", c.topic,
					"partition", e.TopicPartition.Partition,
					"offset", int64(e.TopicPartition.Offset),
				)
				return BlockMessage{}, Offset{}, err
			}
			off := OffsetFromTopicPartition(e.TopicPartition)
			c.logger.Debug("polled block message",
				"topic", c.topic,
				"slot", msg.Slot,
				"kind", msg.Kind,
				"partition", off.Partition,
				"offset", off.Offset,
			)
			return msg, off, nil
		case kafka.Error:
			if e.IsTimeout() {
				continue
			}
			if isRetriablePollError(e) {
				return BlockMessage{}, Offset{}, fmt.Errorf("%w: %s", ErrQueueNotReady, e)
			}
			c.logger.Error("kafka poll failed", "err", e, "topic", c.topic)
			return BlockMessage{}, Offset{}, fmt.Errorf("kafka poll: %w", e)
		default:
			// ignore other events (assigned partitions, etc.)
		}
	}
}

// Commit stores the given message offset as the next consume position
// (commits offset+1 per Kafka convention via CommitOffsets).
func (c *Consumer) Commit(_ context.Context, off Offset) error {
	tp := off.TopicPartition()
	// CommitOffsets expects the next offset to read.
	tp.Offset = kafka.Offset(off.Offset + 1)
	_, err := c.consumer.CommitOffsets([]kafka.TopicPartition{tp})
	if err != nil {
		c.logger.Error("commit block offset failed",
			"err", err,
			"topic", off.Topic,
			"partition", off.Partition,
			"offset", off.Offset,
		)
		return fmt.Errorf("commit offset: %w", err)
	}
	c.logger.Debug("committed block offset",
		"topic", off.Topic,
		"partition", off.Partition,
		"offset", off.Offset,
	)
	return nil
}

// Seek moves the consumer to the given offset without committing.
func (c *Consumer) Seek(_ context.Context, off Offset) error {
	tp := off.TopicPartition()
	if err := c.consumer.Seek(tp, 1000); err != nil {
		c.logger.Error("seek block offset failed",
			"err", err,
			"topic", off.Topic,
			"partition", off.Partition,
			"offset", off.Offset,
		)
		return fmt.Errorf("seek: %w", err)
	}
	c.logger.Info("seeked block consumer",
		"topic", off.Topic,
		"partition", off.Partition,
		"offset", off.Offset,
	)
	return nil
}

// Close closes the underlying Kafka consumer.
func (c *Consumer) Close() error {
	c.logger.Info("closing block consumer", "topic", c.topic)
	return c.consumer.Close()
}
