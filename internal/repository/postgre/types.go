package postgre

import (
	"context"
	"errors"
)

// ErrCheckpointConflict is returned when conditional checkpoint advance does not update exactly one row.
var ErrCheckpointConflict = errors.New("postgre: checkpoint conflict")

// ScanBatch is the entity payload persisted by SaveBlockBatch.
type ScanBatch struct {
	Block          Block
	Transactions   []Transaction
	Transfers      []Transfer
	BalanceChanges []TokenBalanceChange
}

// ScanDAO persists scan checkpoint, blocks, and halt records using GORM entities only.
type ScanDAO interface {
	GetCheckpoint(ctx context.Context) (Checkpoint, bool, error)
	SaveBlockBatch(ctx context.Context, prev Checkpoint, batch ScanBatch) error
	CreateHalt(ctx context.Context, halt Halt) error
	HasUnresolvedHalt(ctx context.Context) (bool, error)
}
