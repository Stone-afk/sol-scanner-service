package postgre

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const createBatchSize = 1000

type scanDAO struct {
	db *gorm.DB
}

// NewScanDAO returns a PostgreSQL-backed ScanDAO.
func NewScanDAO(db *gorm.DB) ScanDAO {
	return &scanDAO{db: db}
}

func (d *scanDAO) GetCheckpoint(ctx context.Context) (Checkpoint, bool, error) {
	var entity Checkpoint
	err := d.db.WithContext(ctx).Where("id = ?", 1).Take(&entity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Checkpoint{}, false, nil
	}
	if err != nil {
		return Checkpoint{}, false, fmt.Errorf("get checkpoint: %w", err)
	}
	return entity, true, nil
}

func (d *scanDAO) SaveBlockBatch(ctx context.Context, prev Checkpoint, batch ScanBatch) error {
	block := batch.Block
	block.CreatedAt = time.Now().Unix()

	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Select(
			"Slot", "Blockhash", "ParentSlot", "PreviousBlockhash",
			"BlockHeight", "BlockTime", "TransactionCount", "CreatedAt",
		).Create(&block).Error; err != nil {
			return fmt.Errorf("create block: %w", err)
		}

		if len(batch.Transactions) > 0 {
			if err := tx.Select(
				"Slot", "TxIndex", "Signature", "Status", "Version", "Fee", "FeePayer",
			).CreateInBatches(batch.Transactions, createBatchSize).Error; err != nil {
				return fmt.Errorf("create transactions: %w", err)
			}
		}
		if len(batch.Transfers) > 0 {
			if err := tx.Select(
				"Slot", "TxIndex", "IxIndex", "InnerIndex", "Program", "Kind",
				"Mint", "Decimals", "FromAccount", "FromOwner", "ToAccount", "ToOwner",
				"Amount", "Fee",
			).CreateInBatches(batch.Transfers, createBatchSize).Error; err != nil {
				return fmt.Errorf("create transfers: %w", err)
			}
		}
		if len(batch.BalanceChanges) > 0 {
			if err := tx.Select(
				"Slot", "TxIndex", "AccountIndex", "Program", "Account", "Owner",
				"Mint", "Decimals", "PreAmount", "PostAmount",
			).CreateInBatches(batch.BalanceChanges, createBatchSize).Error; err != nil {
				return fmt.Errorf("create balance changes: %w", err)
			}
		}

		now := time.Now().Unix()
		var result *gorm.DB
		if prev == (Checkpoint{}) {
			result = tx.Exec(
				`INSERT INTO sol_scan_checkpoint (id, last_block_slot, last_blockhash, updated_at) VALUES (1, ?, ?, ?) ON CONFLICT (id) DO NOTHING`,
				block.Slot, block.Blockhash, now,
			)
		} else {
			result = tx.Exec(
				`UPDATE sol_scan_checkpoint SET last_block_slot = ?, last_blockhash = ?, updated_at = ? WHERE id = 1 AND last_block_slot = ? AND last_blockhash = ?`,
				block.Slot, block.Blockhash, now, prev.LastBlockSlot, prev.LastBlockhash,
			)
		}
		if result.Error != nil {
			return fmt.Errorf("advance checkpoint: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrCheckpointConflict
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrCheckpointConflict) {
			return err
		}
		return fmt.Errorf("save block batch: %w", err)
	}
	return nil
}

func (d *scanDAO) CreateHalt(ctx context.Context, halt Halt) error {
	if err := d.db.WithContext(ctx).Select("Reason", "Slot", "Detail", "CreatedAt").Create(&halt).Error; err != nil {
		return fmt.Errorf("create halt: %w", err)
	}
	return nil
}

func (d *scanDAO) HasUnresolvedHalt(ctx context.Context) (bool, error) {
	var exists bool
	if err := d.db.WithContext(ctx).Raw(
		`SELECT EXISTS(SELECT 1 FROM sol_scan_halts WHERE resolved_at IS NULL)`,
	).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("has unresolved halt: %w", err)
	}
	return exists, nil
}
