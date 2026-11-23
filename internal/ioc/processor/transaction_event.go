package processor

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/event/transaction"
	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	"github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"gorm.io/gorm"
)

// InitTransactionProducer creates the Kafka producer and wraps the transaction publisher.
// Does not construct a provider or block producer.
func InitTransactionProducer(
	cfg config.KafkaConfig,
	db *gorm.DB,
	chain string,
	logger *slog.Logger,
) (*transaction.Producer, error) {
	kp, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  strings.Join(cfg.Brokers, ","),
		"message.timeout.ms": 5000,
	})
	if err != nil {
		return nil, fmt.Errorf("create transaction kafka producer")
	}
	pubRepo := repository.NewPublishRepository(postgre.NewPublishDAO(db))
	prod, err := transaction.NewProducer(kp, cfg.TransactionTopic, pubRepo, chain, logger)
	if err != nil {
		kp.Close()
		return nil, err
	}
	return prod, nil
}
