package repository

import (
	"context"
	"errors"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
)

// ErrCheckpointConflict is returned when conditional checkpoint advance does not update exactly one row.
var ErrCheckpointConflict = errors.New("repository: checkpoint conflict")

// ScanRepository persists finalized block batches and scan halt records.
type ScanRepository interface {
	GetCheckpoint(ctx context.Context) (domain.Checkpoint, bool, error)
	SaveBlockBatch(ctx context.Context, prev domain.Checkpoint, batch domain.BlockBatch) error
	CreateHalt(ctx context.Context, halt domain.Halt) error
	HasUnresolvedHalt(ctx context.Context) (bool, error)
}
