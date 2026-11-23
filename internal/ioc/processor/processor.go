package processor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/alert"
	appprocessor "github.com/Stone-afk/sol-scanner-service/internal/processor"
	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	"github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"gorm.io/gorm"
)

const blockConsumerGroupID = "sol-processor-blocks"

// InitBlockConsumer creates the Kafka consumer and wraps the block-queue reader.
// Caller must Close the returned consumer.
func InitBlockConsumer(cfg config.KafkaConfig, logger *slog.Logger) (*block.Consumer, error) {
	kc, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  strings.Join(cfg.Brokers, ","),
		"group.id":           blockConsumerGroupID,
		"enable.auto.commit": false,
		"auto.offset.reset":  "earliest",
		// shortcut: mainnet getBlock JSON often 1.5–4MiB; 32MiB is e2e/prod client cap — raise + retest if larger blocks appear.
		"fetch.message.max.bytes":   32 * 1024 * 1024,
		"max.partition.fetch.bytes": 32 * 1024 * 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("create block kafka consumer")
	}
	cons, err := block.NewConsumer(kc, cfg.BlockTopic, logger)
	if err != nil {
		_ = kc.Close()
		return nil, err
	}
	return cons, nil
}

// InitProcessor wires the apply-loop Processor with scan/cursor repositories and LogAlerter.
// Does not construct a provider or call LoadForScanner.
func InitProcessor(
	consumer *block.Consumer,
	db *gorm.DB,
	cfg config.ProcessorConfig,
	logger *slog.Logger,
) *appprocessor.Processor {
	repo := repository.NewScanRepository(postgre.NewScanDAO(db))
	cursor := repository.NewScanCursorRepository(postgre.NewScanCursorDAO(db))
	alerter := alert.NewLogAlerter(logger)
	return appprocessor.New(consumer, repo, cursor, alerter, appprocessor.Config{
		Chain:        cfg.Chain.Name,
		StartSlot:    cfg.Chain.StartSlot,
		PollInterval: cfg.Scanner.PollInterval,
		ParseWorkers: cfg.ParseWorkers,
	}, logger)
}

// HasUnresolvedHalt reports whether an unresolved halt row exists.
func HasUnresolvedHalt(ctx context.Context, db *gorm.DB) (bool, error) {
	return repository.NewScanRepository(postgre.NewScanDAO(db)).HasUnresolvedHalt(ctx)
}
