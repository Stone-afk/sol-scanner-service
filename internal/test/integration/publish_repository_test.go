//go:build e2e

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	repopostgre "github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
	"gorm.io/gorm"
)

const publishTopic = "sol-scanner.transactions"

func newPublishRepository(db *gorm.DB) repository.PublishRepository {
	return repository.NewPublishRepository(repopostgre.NewPublishDAO(db))
}

func TestPublishRepository(t *testing.T) {
	// AC-061 / AC-062
	db := openTestDB(t)
	resetSchema(t, db)
	ctx := context.Background()
	scanRepo := newScanRepository(db)
	pubRepo := newPublishRepository(db)

	seedBlocks := []domain.BlockBatch{
		publishFixtureBlock(100, "Hash100", "Prev99", []domain.Transaction{
			{Slot: 100, TxIndex: 0, Signature: "Sig100A", Status: domain.TxStatusSuccess, Version: domain.TxVersion0, Fee: 5000, FeePayer: "Payer100A"},
			{Slot: 100, TxIndex: 1, Signature: "Sig100B", Status: domain.TxStatusFailed, Version: domain.TxVersionLegacy, Fee: 0, FeePayer: "Payer100B"},
		}, []domain.Transfer{
			{
				Slot: 100, TxIndex: 0, IxIndex: 1, InnerIndex: 0,
				Program: domain.ProgramSPLToken, Kind: domain.KindTransfer,
				FromAccount: "From100Inner", ToAccount: "To100Inner", Amount: 20,
			},
			{
				Slot: 100, TxIndex: 0, IxIndex: 0, InnerIndex: domain.TopLevelInnerIndex,
				Program: domain.ProgramSystem, Kind: domain.KindTransfer,
				FromAccount: "From100Top", ToAccount: "To100Top", Amount: 10,
			},
		}, []domain.TokenBalanceChange{
			{
				Slot: 100, TxIndex: 0, AccountIndex: 2,
				Program: domain.ProgramToken2022, Account: "Bal100B", Owner: "Own100B",
				Mint: "Mint100", Decimals: 6, PreAmount: 1, PostAmount: 2,
			},
			{
				Slot: 100, TxIndex: 0, AccountIndex: 1,
				Program: domain.ProgramToken2022, Account: "Bal100A", Owner: "Own100A",
				Mint: "Mint100", Decimals: 6, PreAmount: 5, PostAmount: 4,
			},
		}),
		publishFixtureBlock(101, "Hash101", "Hash100", nil, nil, nil),
		publishFixtureBlock(102, "Hash102", "Hash101", []domain.Transaction{
			{Slot: 102, TxIndex: 0, Signature: "Sig102", Status: domain.TxStatusSuccess, Version: domain.TxVersion1, Fee: 1, FeePayer: "Payer102"},
		}, nil, nil),
	}

	var prev domain.Checkpoint
	for _, batch := range seedBlocks {
		if err := scanRepo.SaveBlockBatch(ctx, prev, batch); err != nil {
			t.Fatalf("SaveBlockBatch slot %d: %v", batch.Block.Slot, err)
		}
		prev = domain.Checkpoint{Slot: batch.Block.Slot, Blockhash: batch.Block.Blockhash}
	}

	t.Run("no_cursor_lists_from_earliest", func(t *testing.T) {
		// AC-061: no cursor → start at earliest block
		slot, ok, err := pubRepo.GetPublishCursor(ctx, publishTopic)
		if err != nil {
			t.Fatalf("GetPublishCursor: %v", err)
		}
		if ok {
			t.Fatalf("GetPublishCursor ok=true slot=%d, want ok=false", slot)
		}

		batches, err := pubRepo.ListBlockBatches(ctx, 0, 10)
		if err != nil {
			t.Fatalf("ListBlockBatches: %v", err)
		}
		if len(batches) != 3 {
			t.Fatalf("ListBlockBatches len=%d want 3", len(batches))
		}
		if batches[0].Block.Slot != 100 || batches[1].Block.Slot != 101 || batches[2].Block.Slot != 102 {
			t.Fatalf("slots = [%d %d %d], want ascending 100,101,102",
				batches[0].Block.Slot, batches[1].Block.Slot, batches[2].Block.Slot)
		}
	})

	t.Run("list_stable_child_order_and_one_block_together", func(t *testing.T) {
		// AC-062: slot-ascending; one block's child rows fetched together with stable order
		batches, err := pubRepo.ListBlockBatches(ctx, 0, 1)
		if err != nil {
			t.Fatalf("ListBlockBatches limit=1: %v", err)
		}
		if len(batches) != 1 {
			t.Fatalf("limit=1 len=%d want 1 (must not split a slot)", len(batches))
		}
		b := batches[0]
		if b.Block.Slot != 100 {
			t.Fatalf("first block slot=%d want 100", b.Block.Slot)
		}
		if len(b.Transactions) != 2 {
			t.Fatalf("tx count=%d want 2", len(b.Transactions))
		}
		if b.Transactions[0].TxIndex != 0 || b.Transactions[1].TxIndex != 1 {
			t.Fatalf("tx order by tx_index failed: %+v", b.Transactions)
		}
		if len(b.Transfers) != 2 {
			t.Fatalf("transfer count=%d want 2", len(b.Transfers))
		}
		// Stable order: tx_index, ix_index, inner_index → top-level (0,-1) then inner (1,0)
		if b.Transfers[0].IxIndex != 0 || b.Transfers[0].InnerIndex != domain.TopLevelInnerIndex {
			t.Fatalf("transfers[0]=ix=%d inner=%d, want top-level first", b.Transfers[0].IxIndex, b.Transfers[0].InnerIndex)
		}
		if b.Transfers[1].IxIndex != 1 || b.Transfers[1].InnerIndex != 0 {
			t.Fatalf("transfers[1]=ix=%d inner=%d, want inner second", b.Transfers[1].IxIndex, b.Transfers[1].InnerIndex)
		}
		if len(b.BalanceChanges) != 2 {
			t.Fatalf("balance count=%d want 2", len(b.BalanceChanges))
		}
		if b.BalanceChanges[0].AccountIndex != 1 || b.BalanceChanges[1].AccountIndex != 2 {
			t.Fatalf("balance order by account_index failed: %+v", b.BalanceChanges)
		}
		// Sibling slots must not leak into this batch.
		for _, tx := range b.Transactions {
			if tx.Slot != 100 {
				t.Fatalf("tx slot=%d leaked into slot-100 batch", tx.Slot)
			}
		}
	})

	t.Run("advance_inserts_and_never_moves_backward", func(t *testing.T) {
		// AC-061: successful advance inserts; cursor never moves backward
		if err := pubRepo.AdvancePublishCursor(ctx, publishTopic, 100); err != nil {
			t.Fatalf("AdvancePublishCursor insert: %v", err)
		}
		slot, ok, err := pubRepo.GetPublishCursor(ctx, publishTopic)
		if err != nil || !ok {
			t.Fatalf("GetPublishCursor after insert: ok=%v err=%v", ok, err)
		}
		if slot != 100 {
			t.Fatalf("published_slot=%d want 100", slot)
		}

		if err := pubRepo.AdvancePublishCursor(ctx, publishTopic, 102); err != nil {
			t.Fatalf("AdvancePublishCursor forward: %v", err)
		}
		slot, ok, err = pubRepo.GetPublishCursor(ctx, publishTopic)
		if err != nil || !ok || slot != 102 {
			t.Fatalf("after forward: ok=%v slot=%d err=%v", ok, slot, err)
		}

		err = pubRepo.AdvancePublishCursor(ctx, publishTopic, 101)
		if err == nil {
			t.Fatal("AdvancePublishCursor backward succeeded, want error")
		}
		slot, ok, getErr := pubRepo.GetPublishCursor(ctx, publishTopic)
		if getErr != nil || !ok || slot != 102 {
			t.Fatalf("cursor changed after backward attempt: ok=%v slot=%d err=%v", ok, slot, getErr)
		}

		batches, err := pubRepo.ListBlockBatches(ctx, slot, 10)
		if err != nil {
			t.Fatalf("ListBlockBatches after cursor: %v", err)
		}
		if len(batches) != 0 {
			t.Fatalf("after published_slot=102 want no batches, got %d", len(batches))
		}
	})

	t.Run("topic_mismatch_errors_without_send_path", func(t *testing.T) {
		// AC-061: topic mismatch → error (producer must not send)
		_, _, err := pubRepo.GetPublishCursor(ctx, "other-topic")
		if !errors.Is(err, repository.ErrPublishTopicMismatch) {
			t.Fatalf("GetPublishCursor mismatched topic err=%v want ErrPublishTopicMismatch", err)
		}
	})
}

func publishFixtureBlock(
	slot uint64,
	hash, prevHash string,
	txs []domain.Transaction,
	transfers []domain.Transfer,
	balances []domain.TokenBalanceChange,
) domain.BlockBatch {
	return domain.BlockBatch{
		Block: domain.Block{
			Slot:              slot,
			Blockhash:         padHash(hash),
			ParentSlot:        slot - 1,
			PreviousBlockhash: padHash(prevHash),
			TransactionCount:  len(txs),
		},
		Transactions:   txs,
		Transfers:      transfers,
		BalanceChanges: balances,
	}
}

func padHash(s string) string {
	const want = 44
	if len(s) >= want {
		return s[:want]
	}
	b := make([]byte, want)
	copy(b, s)
	for i := len(s); i < want; i++ {
		b[i] = 'x'
	}
	return string(b)
}
