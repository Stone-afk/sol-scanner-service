package transaction

import (
	"context"
	"log/slog"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/ctxwait"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/mqx"
	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

const (
	publishBatchLimit   = 1
	publishIdleWait     = 500 * time.Millisecond
	publishBackoffStart = time.Second
	publishBackoffMax   = 30 * time.Second
)

// Producer publishes committed blocks by publish cursor via mqx.
// Pointer receiver: owns the underlying Kafka producer lifecycle through mqx
// and an injectable wait clock for backoff / idle polling.
type Producer struct {
	inner  mqx.Producer[TransactionMessage]
	repo   repository.PublishRepository
	topic  string
	chain  string
	logger *slog.Logger
	wait   func(ctx context.Context, d time.Duration) error
}

// NewProducer wraps *kafka.Producer for topic, matching event/block inject style:
// domain event constructors take *kafka.Producer + topic and delegate to mqx.
func NewProducer(
	producer *kafka.Producer,
	topic string,
	repo repository.PublishRepository,
	chain string,
	logger *slog.Logger,
) (*Producer, error) {
	inner, err := mqx.NewGeneralProducer[TransactionMessage](producer, topic)
	if err != nil {
		return nil, err
	}
	return NewProducerFromMQX(inner, topic, repo, chain, logger), nil
}

// NewProducerFromMQX is for tests that inject a fake mqx.Producer.
func NewProducerFromMQX(
	inner mqx.Producer[TransactionMessage],
	topic string,
	repo repository.PublishRepository,
	chain string,
	logger *slog.Logger,
) *Producer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Producer{
		inner:  inner,
		repo:   repo,
		topic:  topic,
		chain:  chain,
		logger: logger,
		wait:   ctxwait.Wait,
	}
}

// Run publishes one committed block at a time until ctx is canceled.
// published_slot advances only after ProduceBatch fully acks (or for empty-tx blocks).
func (p *Producer) Run(ctx context.Context) error {
	backoff := publishBackoffStart
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		afterSlot, err := p.afterSlot(ctx)
		if err != nil {
			return err
		}

		batches, err := p.repo.ListBlockBatches(ctx, afterSlot, publishBatchLimit)
		if err != nil {
			return err
		}
		if len(batches) == 0 {
			if waitErr := p.wait(ctx, publishIdleWait); waitErr != nil {
				return waitErr
			}
			continue
		}

		batch := batches[0]
		if err := p.publishBlock(ctx, batch); err != nil {
			if waitErr := p.wait(ctx, backoff); waitErr != nil {
				return waitErr
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = publishBackoffStart
	}
}

func (p *Producer) afterSlot(ctx context.Context) (uint64, error) {
	slot, ok, err := p.repo.GetPublishCursor(ctx, p.topic)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	return slot, nil
}

func (p *Producer) publishBlock(ctx context.Context, batch domain.BlockBatch) error {
	msgs := FromBlockBatch(p.chain, batch)
	if len(msgs) > 0 {
		keyed := make([]mqx.Keyed[TransactionMessage], len(msgs))
		for i, msg := range msgs {
			keyed[i] = mqx.Keyed[TransactionMessage]{
				Key:   msg.Signature,
				Value: msg,
			}
		}
		if err := p.inner.ProduceBatch(ctx, keyed); err != nil {
			return err
		}
	}
	return p.repo.AdvancePublishCursor(ctx, p.topic, batch.Block.Slot)
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > publishBackoffMax || next < current {
		return publishBackoffMax
	}
	return next
}

// Close closes the underlying mqx producer.
func (p *Producer) Close() {
	p.inner.Close()
}
