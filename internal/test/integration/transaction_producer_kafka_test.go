//go:build e2e

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/event/transaction"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/mqx"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func TestTransactionProducerKafka(t *testing.T) {
	// AC-058 / AC-059
	db := openTestDB(t)
	resetSchema(t, db)
	ctx := context.Background()
	scanRepo := newScanRepository(db)
	pubRepo := newPublishRepository(db)

	brokers := kafkaBrokers(t)
	topic := fmt.Sprintf("tx-publish-e2e-%d", time.Now().UnixNano())
	groupID := fmt.Sprintf("tx-publish-cg-%d", time.Now().UnixNano())

	admin, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers": brokers,
	})
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer admin.Close()

	createCtx, createCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer createCancel()
	results, err := admin.CreateTopics(createCtx, []kafka.TopicSpecification{{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	}})
	if err != nil {
		t.Fatalf("CreateTopics: %v", err)
	}
	for _, r := range results {
		if r.Error.Code() != kafka.ErrNoError && r.Error.Code() != kafka.ErrTopicAlreadyExists {
			t.Fatalf("CreateTopics %s: %v", r.Topic, r.Error)
		}
	}

	const chain = "solana-mainnet"
	batch := domain.BlockBatch{
		Block: domain.Block{
			Slot:              500,
			Blockhash:         padHash("Hash500"),
			ParentSlot:        499,
			PreviousBlockhash: padHash("Prev499"),
			HasBlockTime:      true,
			BlockTime:         1_700_000_500,
			TransactionCount:  3,
		},
		Transactions: []domain.Transaction{
			{Slot: 500, TxIndex: 0, Signature: "SigSuccess2Xfersxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Status: domain.TxStatusSuccess, Version: domain.TxVersion0, Fee: 5000, FeePayer: "PayerSuccess"},
			{Slot: 500, TxIndex: 1, Signature: "SigFailedEmptyxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Status: domain.TxStatusFailed, Version: domain.TxVersionLegacy, Fee: 0, FeePayer: "PayerFailed"},
			{Slot: 500, TxIndex: 2, Signature: "SigSuccessNoXferxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Status: domain.TxStatusSuccess, Version: domain.TxVersion1, Fee: 100, FeePayer: "PayerNoXfer"},
		},
		Transfers: []domain.Transfer{
			{
				Slot: 500, TxIndex: 0, IxIndex: 0, InnerIndex: domain.TopLevelInnerIndex,
				Program: domain.ProgramSystem, Kind: domain.KindTransfer,
				FromAccount: "FromA", ToAccount: "ToA", Amount: 11,
			},
			{
				Slot: 500, TxIndex: 0, IxIndex: 1, InnerIndex: domain.TopLevelInnerIndex,
				Program: domain.ProgramSPLToken, Kind: domain.KindTransfer,
				Mint: "MintX", FromAccount: "FromB", ToAccount: "ToB", Amount: 22, HasDecimals: true, Decimals: 6,
			},
		},
	}
	if err := scanRepo.SaveBlockBatch(ctx, domain.Checkpoint{}, batch); err != nil {
		t.Fatalf("SaveBlockBatch: %v", err)
	}

	t.Run("k01_three_messages_and_cursor", func(t *testing.T) {
		// AC-058 / K-01
		kp, err := kafka.NewProducer(&kafka.ConfigMap{
			"bootstrap.servers":  brokers,
			"message.timeout.ms": 5000,
		})
		if err != nil {
			t.Fatalf("NewProducer: %v", err)
		}
		prod, err := transaction.NewProducer(kp, topic, pubRepo, chain, slog.Default())
		if err != nil {
			kp.Close()
			t.Fatalf("transaction.NewProducer: %v", err)
		}
		defer prod.Close()

		runCtx, runCancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- prod.Run(runCtx) }()

		deadline := time.Now().Add(30 * time.Second)
		for {
			slot, ok, err := pubRepo.GetPublishCursor(context.Background(), topic)
			if err != nil {
				runCancel()
				t.Fatalf("GetPublishCursor: %v", err)
			}
			if ok && slot == 500 {
				break
			}
			if time.Now().After(deadline) {
				runCancel()
				t.Fatalf("timeout waiting for published_slot=500 (ok=%v slot=%d)", ok, slot)
			}
			time.Sleep(50 * time.Millisecond)
		}
		runCancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not exit after cancel")
		}

		consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
			"bootstrap.servers":  brokers,
			"group.id":           groupID,
			"auto.offset.reset":  "earliest",
			"enable.auto.commit": false,
		})
		if err != nil {
			t.Fatalf("NewConsumer: %v", err)
		}
		defer consumer.Close()
		if err := consumer.Subscribe(topic, nil); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		wantKeys := []string{
			batch.Transactions[0].Signature,
			batch.Transactions[1].Signature,
			batch.Transactions[2].Signature,
		}
		got := make(map[string]transaction.TransactionMessage, 3)
		consumeDeadline := time.Now().Add(30 * time.Second)
		for len(got) < 3 {
			if time.Now().After(consumeDeadline) {
				t.Fatalf("timed out waiting for 3 messages, got %d keys=%v", len(got), keysOf(got))
			}
			msg, err := consumer.ReadMessage(2 * time.Second)
			if err != nil {
				var kErr kafka.Error
				if errors.As(err, &kErr) && kErr.Code() == kafka.ErrTimedOut {
					continue
				}
				t.Fatalf("ReadMessage: %v", err)
			}
			key := string(msg.Key)
			var payload transaction.TransactionMessage
			if err := json.Unmarshal(msg.Value, &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got[key] = payload
		}

		for _, key := range wantKeys {
			if _, ok := got[key]; !ok {
				t.Fatalf("missing key %q; got=%v", key, keysOf(got))
			}
		}
		failed := got[batch.Transactions[1].Signature]
		if failed.Status != transaction.StatusFailed {
			t.Fatalf("failed status=%q want FAILED", failed.Status)
		}
		if failed.Transfers == nil || len(failed.Transfers) != 0 {
			t.Fatalf("failed transfers=%#v want empty slice", failed.Transfers)
		}
		if failed.TokenBalanceChanges == nil || len(failed.TokenBalanceChanges) != 0 {
			t.Fatalf("failed balances=%#v want empty slice", failed.TokenBalanceChanges)
		}
		success := got[batch.Transactions[0].Signature]
		if success.Status != transaction.StatusSuccess || len(success.Transfers) != 2 {
			t.Fatalf("success msg status=%s transfers=%d", success.Status, len(success.Transfers))
		}
		noXfer := got[batch.Transactions[2].Signature]
		if noXfer.Status != transaction.StatusSuccess || len(noXfer.Transfers) != 0 {
			t.Fatalf("no-xfer msg status=%s transfers=%d", noXfer.Status, len(noXfer.Transfers))
		}
	})

	t.Run("k02_mid_fail_no_advance_retry_identical", func(t *testing.T) {
		// AC-059 / K-02: mid-batch failure leaves cursor; retry bytes identical.
		resetSchema(t, db)
		batch2 := domain.BlockBatch{
			Block: domain.Block{
				Slot:              600,
				Blockhash:         padHash("Hash600"),
				ParentSlot:        599,
				PreviousBlockhash: padHash("Prev599"),
				HasBlockTime:      true,
				BlockTime:         1_700_000_600,
				TransactionCount:  2,
			},
			Transactions: []domain.Transaction{
				{Slot: 600, TxIndex: 0, Signature: "Sig600Axxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Status: domain.TxStatusSuccess, Version: domain.TxVersion0, Fee: 1, FeePayer: "PA"},
				{Slot: 600, TxIndex: 1, Signature: "Sig600Bxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Status: domain.TxStatusSuccess, Version: domain.TxVersion0, Fee: 2, FeePayer: "PB"},
			},
		}
		if err := scanRepo.SaveBlockBatch(ctx, domain.Checkpoint{}, batch2); err != nil {
			t.Fatalf("SaveBlockBatch: %v", err)
		}

		wantMsgs := transaction.FromBlockBatch(chain, batch2)
		wantRaw := make([][]byte, len(wantMsgs))
		for i, m := range wantMsgs {
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("marshal want: %v", err)
			}
			wantRaw[i] = b
		}

		failOnce := &failOnceMQX{err: errors.New("injected mid-batch failure")}
		prod := transaction.NewProducerFromMQX(failOnce, topic, pubRepo, chain, slog.Default())

		runCtx, runCancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- prod.Run(runCtx) }()

		deadline := time.Now().Add(30 * time.Second)
		for {
			slot, ok, err := pubRepo.GetPublishCursor(context.Background(), topic)
			if err != nil {
				runCancel()
				t.Fatalf("GetPublishCursor: %v", err)
			}
			if ok && slot == 600 {
				break
			}
			if time.Now().After(deadline) {
				runCancel()
				t.Fatalf("timeout waiting for advance after retry (ok=%v slot=%d calls=%d)", ok, slot, failOnce.calls())
			}
			time.Sleep(20 * time.Millisecond)
		}
		runCancel()
		<-done

		if failOnce.calls() < 2 {
			t.Fatalf("ProduceBatch calls=%d want >= 2", failOnce.calls())
		}
		first, second := failOnce.raw(0), failOnce.raw(1)
		if len(first) != len(wantRaw) || len(second) != len(wantRaw) {
			t.Fatalf("raw lens first=%d second=%d want=%d", len(first), len(second), len(wantRaw))
		}
		for i := range wantRaw {
			if !bytes.Equal(first[i], second[i]) {
				t.Fatalf("retry JSON[%d] not identical:\nfirst=%s\nsecond=%s", i, first[i], second[i])
			}
			if !bytes.Equal(first[i], wantRaw[i]) {
				t.Fatalf("sent JSON[%d] != FromBlockBatch:\ngot=%s\nwant=%s", i, first[i], wantRaw[i])
			}
		}
	})
}

type failOnceMQX struct {
	err     error
	batches [][]mqx.Keyed[transaction.TransactionMessage]
	raws    [][][]byte
	n       int
}

func (f *failOnceMQX) ProduceBatch(_ context.Context, msgs []mqx.Keyed[transaction.TransactionMessage]) error {
	cp := make([]mqx.Keyed[transaction.TransactionMessage], len(msgs))
	copy(cp, msgs)
	f.batches = append(f.batches, cp)
	raw := make([][]byte, len(msgs))
	for i, m := range msgs {
		b, err := json.Marshal(m.Value)
		if err != nil {
			return err
		}
		raw[i] = b
	}
	f.raws = append(f.raws, raw)
	f.n++
	if f.n == 1 {
		return f.err
	}
	return nil
}

func (f *failOnceMQX) Close() {}

func (f *failOnceMQX) calls() int { return f.n }

func (f *failOnceMQX) raw(i int) [][]byte {
	if i < 0 || i >= len(f.raws) {
		return nil
	}
	return f.raws[i]
}

func keysOf(m map[string]transaction.TransactionMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
