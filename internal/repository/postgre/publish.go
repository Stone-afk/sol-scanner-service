package postgre

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrPublishTopicMismatch is returned when the stored cursor topic differs from the requested topic.
var ErrPublishTopicMismatch = errors.New("postgre: publish cursor topic mismatch")

// ErrPublishCursorRegression is returned when AdvancePublishCursor would move published_slot backward.
var ErrPublishCursorRegression = errors.New("postgre: publish cursor cannot move backward")

// PublishCursor maps sol_publish_cursor.
type PublishCursor struct {
	ID            int16  `gorm:"column:id;primaryKey"`
	Topic         string `gorm:"column:topic"`
	PublishedSlot uint64 `gorm:"column:published_slot"`
	UpdatedAt     int64  `gorm:"column:updated_at"`
}

func (PublishCursor) TableName() string { return "sol_publish_cursor" }

// PublishDAO reads committed blocks by publish cursor and advances the cursor.
type PublishDAO interface {
	GetPublishCursor(ctx context.Context, topic string) (uint64, bool, error)
	ListBlockBatches(ctx context.Context, afterSlot uint64, limit int) ([]ScanBatch, error)
	AdvancePublishCursor(ctx context.Context, topic string, slot uint64) error
}

type publishDAO struct {
	db *gorm.DB
}

// NewPublishDAO returns a PostgreSQL-backed PublishDAO.
func NewPublishDAO(db *gorm.DB) PublishDAO {
	return &publishDAO{db: db}
}

func (d *publishDAO) GetPublishCursor(ctx context.Context, topic string) (uint64, bool, error) {
	var entity PublishCursor
	err := d.db.WithContext(ctx).Where("id = ?", 1).Take(&entity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get publish cursor: %w", err)
	}
	if entity.Topic != topic {
		return 0, false, ErrPublishTopicMismatch
	}
	return entity.PublishedSlot, true, nil
}

func (d *publishDAO) ListBlockBatches(ctx context.Context, afterSlot uint64, limit int) ([]ScanBatch, error) {
	if limit <= 0 {
		return nil, nil
	}

	var blocks []Block
	if err := d.db.WithContext(ctx).
		Where("slot > ?", afterSlot).
		Order("slot ASC").
		Limit(limit).
		Find(&blocks).Error; err != nil {
		return nil, fmt.Errorf("list publish blocks: %w", err)
	}
	if len(blocks) == 0 {
		return nil, nil
	}

	slots := make([]uint64, len(blocks))
	for i, b := range blocks {
		slots[i] = b.Slot
	}

	var txs []Transaction
	if err := d.db.WithContext(ctx).
		Where("slot IN ?", slots).
		Order("slot ASC, tx_index ASC").
		Find(&txs).Error; err != nil {
		return nil, fmt.Errorf("list publish transactions: %w", err)
	}

	var transfers []Transfer
	if err := d.db.WithContext(ctx).
		Where("slot IN ?", slots).
		Order("slot ASC, tx_index ASC, ix_index ASC, inner_index ASC").
		Find(&transfers).Error; err != nil {
		return nil, fmt.Errorf("list publish transfers: %w", err)
	}

	var balances []TokenBalanceChange
	if err := d.db.WithContext(ctx).
		Where("slot IN ?", slots).
		Order("slot ASC, tx_index ASC, account_index ASC").
		Find(&balances).Error; err != nil {
		return nil, fmt.Errorf("list publish balance changes: %w", err)
	}

	txsBySlot := groupBySlot(txs, func(t Transaction) uint64 { return t.Slot })
	transfersBySlot := groupBySlot(transfers, func(t Transfer) uint64 { return t.Slot })
	balancesBySlot := groupBySlot(balances, func(b TokenBalanceChange) uint64 { return b.Slot })

	out := make([]ScanBatch, len(blocks))
	for i, block := range blocks {
		out[i] = ScanBatch{
			Block:          block,
			Transactions:   txsBySlot[block.Slot],
			Transfers:      transfersBySlot[block.Slot],
			BalanceChanges: balancesBySlot[block.Slot],
		}
	}
	return out, nil
}

func groupBySlot[T any](rows []T, slotOf func(T) uint64) map[uint64][]T {
	out := make(map[uint64][]T)
	for _, row := range rows {
		slot := slotOf(row)
		out[slot] = append(out[slot], row)
	}
	return out
}

func (d *publishDAO) AdvancePublishCursor(ctx context.Context, topic string, slot uint64) error {
	now := time.Now().Unix()
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entity PublishCursor
		err := tx.Where("id = ?", 1).Take(&entity).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result := tx.Exec(
				`INSERT INTO sol_publish_cursor (id, topic, published_slot, updated_at) VALUES (1, ?, ?, ?)`,
				topic, slot, now,
			)
			if result.Error != nil {
				return fmt.Errorf("insert publish cursor: %w", result.Error)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("load publish cursor: %w", err)
		}
		if entity.Topic != topic {
			return ErrPublishTopicMismatch
		}
		if slot <= entity.PublishedSlot {
			return ErrPublishCursorRegression
		}
		result := tx.Exec(
			`UPDATE sol_publish_cursor SET published_slot = ?, updated_at = ? WHERE id = 1 AND topic = ? AND published_slot = ?`,
			slot, now, topic, entity.PublishedSlot,
		)
		if result.Error != nil {
			return fmt.Errorf("advance publish cursor: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrPublishCursorRegression
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrPublishTopicMismatch) || errors.Is(err, ErrPublishCursorRegression) {
			return err
		}
		return fmt.Errorf("advance publish cursor: %w", err)
	}
	return nil
}
