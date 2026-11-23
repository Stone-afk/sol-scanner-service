package scanner

import (
	"fmt"
	"strings"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// InitBlockProducer creates the Kafka producer and wraps the block-queue publisher.
// Does not construct a transaction producer.
func InitBlockProducer(cfg config.KafkaConfig) (*block.Producer, error) {
	kp, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": strings.Join(cfg.Brokers, ","),
		// shortcut: mainnet getBlock JSON often 1.5–4MiB; 32MiB is e2e/prod client cap — raise + retest if larger blocks appear.
		"message.max.bytes": 32 * 1024 * 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("create block kafka producer")
	}
	prod, err := block.NewProducer(kp, cfg.BlockTopic)
	if err != nil {
		kp.Close()
		return nil, err
	}
	return prod, nil
}
