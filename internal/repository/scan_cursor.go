package repository

import (
	"context"
	"errors"

	"github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
)

// ErrHandedSlotConflict is returned when conditional handed_slot advance does not update exactly one row.
var ErrHandedSlotConflict = errors.New("repository: handed slot conflict")

// ScanCursorRepository persists the scanner handoff cursor (handed_slot).
type ScanCursorRepository interface {
	GetHandedSlot(ctx context.Context) (uint64, bool, error)
	AdvanceHandedSlot(ctx context.Context, prev, next uint64) error
	RewindHandedSlot(ctx context.Context, slot uint64) error
}

type scanCursorRepository struct {
	dao postgre.ScanCursorDAO
}

// NewScanCursorRepository returns a domain-facing ScanCursorRepository over a ScanCursorDAO.
func NewScanCursorRepository(dao postgre.ScanCursorDAO) ScanCursorRepository {
	return &scanCursorRepository{dao: dao}
}

func (r *scanCursorRepository) GetHandedSlot(ctx context.Context) (uint64, bool, error) {
	return r.dao.GetHandedSlot(ctx)
}

func (r *scanCursorRepository) AdvanceHandedSlot(ctx context.Context, prev, next uint64) error {
	err := r.dao.AdvanceHandedSlot(ctx, prev, next)
	if errors.Is(err, postgre.ErrHandedSlotConflict) {
		return ErrHandedSlotConflict
	}
	return err
}

func (r *scanCursorRepository) RewindHandedSlot(ctx context.Context, slot uint64) error {
	return r.dao.RewindHandedSlot(ctx, slot)
}
