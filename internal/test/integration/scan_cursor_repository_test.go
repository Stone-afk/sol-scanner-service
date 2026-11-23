//go:build e2e

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	repopostgre "github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
	"gorm.io/gorm"
)

func newScanCursorRepository(db *gorm.DB) repository.ScanCursorRepository {
	return repository.NewScanCursorRepository(repopostgre.NewScanCursorDAO(db))
}

func TestScanCursorRepository(t *testing.T) {
	db := openTestDB(t)
	resetSchema(t, db)
	ctx := context.Background()
	repo := newScanCursorRepository(db)

	const startSlot uint64 = 500

	t.Run("no_row", func(t *testing.T) {
		resetSchema(t, db)
		slot, ok, err := repo.GetHandedSlot(ctx)
		if err != nil {
			t.Fatalf("GetHandedSlot: %v", err)
		}
		if ok {
			t.Fatalf("GetHandedSlot ok=true slot=%d, want ok=false", slot)
		}
	})

	t.Run("insert_when_prev_is_start_slot_minus_one", func(t *testing.T) {
		resetSchema(t, db)
		prev := startSlot - 1
		next := uint64(510)
		if err := repo.AdvanceHandedSlot(ctx, prev, next); err != nil {
			t.Fatalf("AdvanceHandedSlot first insert: %v", err)
		}
		slot, ok, err := repo.GetHandedSlot(ctx)
		if err != nil || !ok {
			t.Fatalf("GetHandedSlot after insert: ok=%v err=%v", ok, err)
		}
		if slot != next {
			t.Fatalf("handed_slot=%d want %d", slot, next)
		}
	})

	t.Run("conditional_advance", func(t *testing.T) {
		resetSchema(t, db)
		if err := repo.AdvanceHandedSlot(ctx, startSlot-1, 600); err != nil {
			t.Fatalf("seed AdvanceHandedSlot: %v", err)
		}
		if err := repo.AdvanceHandedSlot(ctx, 600, 700); err != nil {
			t.Fatalf("AdvanceHandedSlot: %v", err)
		}
		slot, ok, err := repo.GetHandedSlot(ctx)
		if err != nil || !ok {
			t.Fatalf("GetHandedSlot: ok=%v err=%v", ok, err)
		}
		if slot != 700 {
			t.Fatalf("handed_slot=%d want 700", slot)
		}
	})

	t.Run("wrong_prev_leaves_row_unchanged", func(t *testing.T) {
		resetSchema(t, db)
		if err := repo.AdvanceHandedSlot(ctx, startSlot-1, 800); err != nil {
			t.Fatalf("seed AdvanceHandedSlot: %v", err)
		}
		err := repo.AdvanceHandedSlot(ctx, 799, 900)
		if !errors.Is(err, repository.ErrHandedSlotConflict) {
			t.Fatalf("err=%v want ErrHandedSlotConflict", err)
		}
		slot, ok, getErr := repo.GetHandedSlot(ctx)
		if getErr != nil || !ok {
			t.Fatalf("GetHandedSlot after conflict: ok=%v err=%v", ok, getErr)
		}
		if slot != 800 {
			t.Fatalf("handed_slot=%d want 800 (unchanged)", slot)
		}
	})

	t.Run("rewind_21000_to_101", func(t *testing.T) {
		resetSchema(t, db)
		if err := repo.AdvanceHandedSlot(ctx, startSlot-1, 21000); err != nil {
			t.Fatalf("seed AdvanceHandedSlot: %v", err)
		}
		if err := repo.RewindHandedSlot(ctx, 101); err != nil {
			t.Fatalf("RewindHandedSlot: %v", err)
		}
		slot, ok, err := repo.GetHandedSlot(ctx)
		if err != nil || !ok {
			t.Fatalf("GetHandedSlot after rewind: ok=%v err=%v", ok, err)
		}
		if slot != 101 {
			t.Fatalf("handed_slot=%d want 101", slot)
		}
	})

	t.Run("rewind_to_499", func(t *testing.T) {
		resetSchema(t, db)
		// S-19: no checkpoint path uses startSlot-1=499; also covers upsert when empty.
		if err := repo.RewindHandedSlot(ctx, 499); err != nil {
			t.Fatalf("RewindHandedSlot empty table: %v", err)
		}
		slot, ok, err := repo.GetHandedSlot(ctx)
		if err != nil || !ok {
			t.Fatalf("GetHandedSlot after rewind insert: ok=%v err=%v", ok, err)
		}
		if slot != 499 {
			t.Fatalf("handed_slot=%d want 499", slot)
		}

		if err := repo.AdvanceHandedSlot(ctx, 499, 21000); err != nil {
			t.Fatalf("AdvanceHandedSlot after rewind: %v", err)
		}
		if err := repo.RewindHandedSlot(ctx, 499); err != nil {
			t.Fatalf("RewindHandedSlot lower: %v", err)
		}
		slot, ok, err = repo.GetHandedSlot(ctx)
		if err != nil || !ok {
			t.Fatalf("GetHandedSlot after second rewind: ok=%v err=%v", ok, err)
		}
		if slot != 499 {
			t.Fatalf("handed_slot=%d want 499", slot)
		}
	})
}
