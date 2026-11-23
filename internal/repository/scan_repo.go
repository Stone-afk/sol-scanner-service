package repository

import (
	"context"
	"errors"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
)

type scanRepository struct {
	dao postgre.ScanDAO
}

// NewScanRepository returns a domain-facing ScanRepository over a ScanDAO.
func NewScanRepository(dao postgre.ScanDAO) ScanRepository {
	return scanRepository{dao: dao}
}

func (r scanRepository) GetCheckpoint(ctx context.Context) (domain.Checkpoint, bool, error) {
	entity, found, err := r.dao.GetCheckpoint(ctx)
	if err != nil || !found {
		return domain.Checkpoint{}, found, err
	}
	return checkpointFromEntity(entity), true, nil
}

func (r scanRepository) SaveBlockBatch(ctx context.Context, prev domain.Checkpoint, batch domain.BlockBatch) error {
	err := r.dao.SaveBlockBatch(ctx, checkpointEntity(prev), toEntities(batch))
	if errors.Is(err, postgre.ErrCheckpointConflict) {
		return ErrCheckpointConflict
	}
	return err
}

func (r scanRepository) CreateHalt(ctx context.Context, halt domain.Halt) error {
	return r.dao.CreateHalt(ctx, haltEntity(halt))
}

func (r scanRepository) HasUnresolvedHalt(ctx context.Context) (bool, error) {
	return r.dao.HasUnresolvedHalt(ctx)
}
