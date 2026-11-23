package repository

import (
	"context"
	"errors"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
)

// ErrPublishTopicMismatch is returned when the stored cursor topic differs from the configured topic.
var ErrPublishTopicMismatch = errors.New("repository: publish cursor topic mismatch")

// ErrPublishCursorRegression is returned when AdvancePublishCursor would move published_slot backward.
var ErrPublishCursorRegression = errors.New("repository: publish cursor cannot move backward")

// PublishRepository reads committed blocks by publish cursor and advances the cursor after full ack.
type PublishRepository interface {
	GetPublishCursor(ctx context.Context, topic string) (uint64, bool, error)
	ListBlockBatches(ctx context.Context, afterSlot uint64, limit int) ([]domain.BlockBatch, error)
	AdvancePublishCursor(ctx context.Context, topic string, slot uint64) error
}

type publishRepository struct {
	dao postgre.PublishDAO
}

// NewPublishRepository returns a domain-facing PublishRepository over a PublishDAO.
func NewPublishRepository(dao postgre.PublishDAO) PublishRepository {
	return publishRepository{dao: dao}
}

func (r publishRepository) GetPublishCursor(ctx context.Context, topic string) (uint64, bool, error) {
	slot, ok, err := r.dao.GetPublishCursor(ctx, topic)
	if errors.Is(err, postgre.ErrPublishTopicMismatch) {
		return 0, false, ErrPublishTopicMismatch
	}
	return slot, ok, err
}

func (r publishRepository) ListBlockBatches(ctx context.Context, afterSlot uint64, limit int) ([]domain.BlockBatch, error) {
	entities, err := r.dao.ListBlockBatches(ctx, afterSlot, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.BlockBatch, len(entities))
	for i, batch := range entities {
		out[i] = fromEntities(batch)
	}
	return out, nil
}

func (r publishRepository) AdvancePublishCursor(ctx context.Context, topic string, slot uint64) error {
	err := r.dao.AdvancePublishCursor(ctx, topic, slot)
	if errors.Is(err, postgre.ErrPublishTopicMismatch) {
		return ErrPublishTopicMismatch
	}
	if errors.Is(err, postgre.ErrPublishCursorRegression) {
		return ErrPublishCursorRegression
	}
	return err
}
