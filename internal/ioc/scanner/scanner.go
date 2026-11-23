package scanner

import (
	"log/slog"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	"github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
	appscanner "github.com/Stone-afk/sol-scanner-service/internal/scanner"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
	"gorm.io/gorm"
)

// InitScanner wires the handoff scanner with cursor and halt-query repositories.
// Does not wire parser, SaveBlockBatch, or the transaction producer.
func InitScanner(
	reader provider.Reader,
	producer appscanner.BlockProducer,
	db *gorm.DB,
	cfg config.Config,
	logger *slog.Logger,
) *appscanner.Scanner {
	cursor := repository.NewScanCursorRepository(postgre.NewScanCursorDAO(db))
	halts := repository.NewScanRepository(postgre.NewScanDAO(db))
	return appscanner.New(reader, producer, cursor, halts, appscanner.Config{
		Chain:          cfg.Chain.Name,
		GenesisHash:    cfg.Chain.GenesisHash,
		StartSlot:      cfg.Chain.StartSlot,
		PollInterval:   cfg.Scanner.PollInterval,
		StepSlots:      cfg.Scanner.StepSlots,
		SlotsPerWorker: cfg.Scanner.SlotsPerWorker,
	}, logger)
}
