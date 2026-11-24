package postgre

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrHandedSlotConflict is returned when conditional handed_slot advance does not update exactly one row.
var ErrHandedSlotConflict = errors.New("postgre: handed slot conflict")

// ScanCursorDAO persists the handoff cursor using GORM entities / SQL only.
type ScanCursorDAO interface {
	GetHandedSlot(ctx context.Context) (uint64, bool, error)
	AdvanceHandedSlot(ctx context.Context, prev, next uint64) error
	RewindHandedSlot(ctx context.Context, slot uint64) error
}

type scanCursorDAO struct {
	db *gorm.DB
}

// NewScanCursorDAO returns a PostgreSQL-backed ScanCursorDAO.
func NewScanCursorDAO(db *gorm.DB) ScanCursorDAO {
	return &scanCursorDAO{db: db}
}

func (d *scanCursorDAO) GetHandedSlot(ctx context.Context) (uint64, bool, error) {
	var entity ScanCursor
	err := d.db.WithContext(ctx).Where("id = ?", 1).Take(&entity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get handed slot: %w", err)
	}
	return entity.HandedSlot, true, nil
}

func (d *scanCursorDAO) AdvanceHandedSlot(ctx context.Context, prev, next uint64) error {
	now := time.Now().Unix()
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(
			`UPDATE sol_scan_cursor SET handed_slot = ?, updated_at = ? WHERE id = 1 AND handed_slot = ?`,
			next, now, prev,
		)
		if result.Error != nil {
			return fmt.Errorf("advance handed slot: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			return nil
		}

		result = tx.Exec(
			`INSERT INTO sol_scan_cursor (id, handed_slot, updated_at) VALUES (1, ?, ?) ON CONFLICT (id) DO NOTHING`,
			next, now,
		)
		if result.Error != nil {
			return fmt.Errorf("insert handed slot: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrHandedSlotConflict
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrHandedSlotConflict) {
			return err
		}
		return fmt.Errorf("advance handed slot: %w", err)
	}
	return nil
}

func (d *scanCursorDAO) RewindHandedSlot(ctx context.Context, slot uint64) error {
	now := time.Now().Unix()
	result := d.db.WithContext(ctx).Exec(
		`INSERT INTO sol_scan_cursor (id, handed_slot, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET handed_slot = EXCLUDED.handed_slot, updated_at = EXCLUDED.updated_at`,
		slot, now,
	)
	if result.Error != nil {
		return fmt.Errorf("rewind handed slot: %w", result.Error)
	}
	return nil
}
