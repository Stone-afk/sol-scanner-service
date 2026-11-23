//go:build e2e

package integration

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	iocprocessor "github.com/Stone-afk/sol-scanner-service/internal/ioc/processor"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

func TestProcessorRuntimeKafkaDown(t *testing.T) {
	// AC-072 / K-03: downstream topic unavailable → checkpoint may advance, publish cursor does not.
	db := openTestDB(t)
	resetSchema(t, db)
	ctx := context.Background()
	scanRepo := newScanRepository(db)
	pubRepo := newPublishRepository(db)

	brokers := kafkaBrokers(t)
	blockTopic := fmt.Sprintf("proc-rt-block-%d", time.Now().UnixNano())
	txTopic := fmt.Sprintf("proc-rt-tx-%d", time.Now().UnixNano())
	const (
		chain     = "solana-mainnet"
		seedSlot  = uint64(100)
		nextSlot  = uint64(101)
		startSlot = uint64(100)
	)

	admin, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
	})
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer admin.Close()

	createCtx, createCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer createCancel()
	results, err := admin.CreateTopics(createCtx, []kafka.TopicSpecification{
		{
			Topic:             blockTopic,
			NumPartitions:     1,
			ReplicationFactor: 1,
		},
		{
			Topic:             txTopic,
			NumPartitions:     1,
			ReplicationFactor: 1,
			Config: map[string]string{
				// Reject any real message so the transaction producer cannot ack.
				"max.message.bytes": "1",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateTopics: %v", err)
	}
	for _, r := range results {
		if r.Error.Code() != kafka.ErrNoError && r.Error.Code() != kafka.ErrTopicAlreadyExists {
			t.Fatalf("CreateTopics %s: %v", r.Topic, r.Error)
		}
	}

	seedHashObj := hashForSlot(seedSlot, 1)
	seedHash := seedHashObj.String()
	seed := domain.BlockBatch{
		Block: domain.Block{
			Slot:              seedSlot,
			Blockhash:         seedHash,
			ParentSlot:        seedSlot - 1,
			PreviousBlockhash: hashForSlot(seedSlot-1, 0).String(),
			HasBlockTime:      true,
			BlockTime:         1_700_000_100,
			TransactionCount:  1,
		},
		Transactions: []domain.Transaction{
			{
				Slot:      seedSlot,
				TxIndex:   0,
				Signature: "SigSeed100xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				Status:    domain.TxStatusSuccess,
				Version:   domain.TxVersion0,
				Fee:       5000,
				FeePayer:  "PayerSeed100",
			},
		},
	}
	if err := scanRepo.SaveBlockBatch(ctx, domain.Checkpoint{}, seed); err != nil {
		t.Fatalf("seed SaveBlockBatch: %v", err)
	}

	kp, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  brokers,
		"message.timeout.ms": 5000,
	})
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	blockProd, err := block.NewProducer(kp, blockTopic)
	if err != nil {
		kp.Close()
		t.Fatalf("block.NewProducer: %v", err)
	}
	defer blockProd.Close()

	nextHash := hashForSlot(nextSlot, 2)
	blockJSON, err := json.Marshal(&rpc.GetBlockResult{
		Blockhash:         nextHash,
		PreviousBlockhash: seedHashObj,
		ParentSlot:        seedSlot,
	})
	if err != nil {
		t.Fatalf("marshal next block: %v", err)
	}

	cfg := config.ProcessorConfig{
		Chain: config.ChainConfig{
			Name:        chain,
			GenesisHash: "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d",
			StartSlot:   startSlot,
		},
		Scanner: config.ScannerConfig{
			// Generous vs Kafka rebalance so 10 empty polls do not halt before assignment.
			PollInterval:   time.Second,
			StepSlots:      2,
			SlotsPerWorker: 2,
		},
		Database: config.DatabaseConfig{DSN: os.Getenv("SOL_SCANNER_E2E_DSN")},
		Kafka: config.KafkaConfig{
			Brokers:          []string{brokers},
			BlockTopic:       blockTopic,
			TransactionTopic: txTopic,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).
		With("test", "TestProcessorRuntimeKafkaDown")

	consumer, err := iocprocessor.InitBlockConsumer(cfg.Kafka, logger)
	if err != nil {
		t.Fatalf("InitBlockConsumer: %v", err)
	}
	proc := iocprocessor.InitProcessor(consumer, db, cfg, logger)
	publisher, err := iocprocessor.InitTransactionProducer(cfg.Kafka, db, cfg.Chain.Name, logger)
	if err != nil {
		_ = consumer.Close()
		t.Fatalf("InitTransactionProducer: %v", err)
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	errCh := make(chan error, 2)
	go func() { errCh <- proc.Apply(runCtx) }()
	go func() { errCh <- publisher.Run(runCtx) }()

	// Allow partition assignment, then enqueue the next block for apply.
	time.Sleep(2 * time.Second)
	if err := blockProd.Produce(ctx, []block.BlockMessage{{
		Version:    block.MessageVersion,
		Chain:      chain,
		Slot:       nextSlot,
		Commitment: block.CommitmentFinalized,
		Kind:       block.KindBlock,
		Block:      blockJSON,
	}}); err != nil {
		runCancel()
		t.Fatalf("Produce next block: %v", err)
	}

	deadline := time.Now().Add(45 * time.Second)
	for {
		cp, ok, err := scanRepo.GetCheckpoint(context.Background())
		if err != nil {
			runCancel()
			t.Fatalf("GetCheckpoint: %v", err)
		}
		if ok && cp.Slot >= nextSlot {
			break
		}
		if time.Now().After(deadline) {
			runCancel()
			t.Fatalf("timeout waiting for checkpoint >= %d (ok=%v slot=%d)", nextSlot, ok, cp.Slot)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Give the publish loop several failed attempts; cursor must stay unset.
	time.Sleep(2 * time.Second)
	slot, pubOK, err := pubRepo.GetPublishCursor(context.Background(), txTopic)
	if err != nil {
		runCancel()
		t.Fatalf("GetPublishCursor: %v", err)
	}
	if pubOK {
		runCancel()
		t.Fatalf("publish cursor advanced to %d while transaction topic unavailable", slot)
	}

	runCancel()
	publisher.Close()
	_ = consumer.Close()

	waitDeadline := time.Now().Add(5 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-errCh:
		case <-time.After(time.Until(waitDeadline)):
			t.Fatal("apply/publish did not exit after cancel")
		}
	}
}

func hashForSlot(slot uint64, tag byte) solana.Hash {
	var h solana.Hash
	binary.BigEndian.PutUint64(h[24:], slot)
	h[0] = tag
	return h
}
