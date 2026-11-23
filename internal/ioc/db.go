package ioc

import (
	"context"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/postgre"
	"gorm.io/gorm"
)

// requiredSchemaVersion is the schema gate identity for both processes (version 2 / scan_cursor).
const requiredSchemaVersion int64 = 2

// InitDB opens PostgreSQL, pings, and enforces the schema gate through version 2.
func InitDB(ctx context.Context, cfg config.DatabaseConfig) (*gorm.DB, error) {
	db, err := postgre.Open(cfg.DSN)
	if err != nil {
		return nil, err
	}
	if err := postgre.Ping(ctx, db); err != nil {
		_ = postgre.Close(db)
		return nil, err
	}
	if err := postgre.CheckSchemaVersion(ctx, db, requiredSchemaVersion); err != nil {
		_ = postgre.Close(db)
		return nil, err
	}
	return db, nil
}

// CloseDB closes the PostgreSQL connection pool opened by InitDB.
func CloseDB(db *gorm.DB) error {
	return postgre.Close(db)
}
