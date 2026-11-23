//go:build e2e

package integration

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	repopostgre "github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
	"gorm.io/gorm"
)

func newScanRepository(db *gorm.DB) repository.ScanRepository {
	return repository.NewScanRepository(repopostgre.NewScanDAO(db))
}

func TestScanRepositoryR01AtomicRollback(t *testing.T) {
	db := openTestDB(t)
	resetSchema(t, db)
	ctx := context.Background()
	repo := newScanRepository(db)

	seed := domain.BlockBatch{
		Block: domain.Block{
			Slot:              10,
			Blockhash:         "SeedHash10xxxxxxxxxxxxxxxxxxxxxxxxxxx",
			ParentSlot:        9,
			PreviousBlockhash: "SeedPrev9xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			TransactionCount:  0,
		},
	}
	if err := repo.SaveBlockBatch(ctx, domain.Checkpoint{}, seed); err != nil {
		t.Fatalf("seed SaveBlockBatch: %v", err)
	}
	prev, found, err := repo.GetCheckpoint(ctx)
	if err != nil || !found {
		t.Fatalf("GetCheckpoint after seed: found=%v err=%v", found, err)
	}
	if prev.Slot != 10 || prev.Blockhash != seed.Block.Blockhash {
		t.Fatalf("seed checkpoint = %+v", prev)
	}

	dup := domain.Transfer{
		Slot:        11,
		TxIndex:     0,
		IxIndex:     0,
		InnerIndex:  domain.TopLevelInnerIndex,
		Program:     domain.ProgramSystem,
		Kind:        domain.KindTransfer,
		FromAccount: "FromDupxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		ToAccount:   "ToDupxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		Amount:      1,
	}
	bad := domain.BlockBatch{
		Block: domain.Block{
			Slot:              11,
			Blockhash:         "BadHash11xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			ParentSlot:        10,
			PreviousBlockhash: seed.Block.Blockhash,
			TransactionCount:  1,
		},
		Transactions: []domain.Transaction{
			{
				Slot:      11,
				TxIndex:   0,
				Signature: "BadSigxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				Status:    domain.TxStatusSuccess,
				Version:   domain.TxVersion0,
				Fee:       5000,
				FeePayer:  "FeePayerDupxxxxxxxxxxxxxxxxxxxxxxxxxx",
			},
		},
		Transfers: []domain.Transfer{dup, dup},
		BalanceChanges: []domain.TokenBalanceChange{
			{
				Slot:         11,
				TxIndex:      0,
				AccountIndex: 0,
				Program:      domain.ProgramToken2022,
				Account:      "BalAccxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				Mint:         "BalMintxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				Decimals:     9,
				PreAmount:    1,
				PostAmount:   2,
			},
		},
	}
	err = repo.SaveBlockBatch(ctx, prev, bad)
	if err == nil {
		t.Fatal("SaveBlockBatch with duplicate transfer keys succeeded, want unique violation")
	}
	if errors.Is(err, repository.ErrCheckpointConflict) {
		t.Fatalf("unexpected ErrCheckpointConflict: %v", err)
	}

	assertSlotAbsent(t, db, 11)
	cp, found, err := repo.GetCheckpoint(ctx)
	if err != nil || !found {
		t.Fatalf("GetCheckpoint after rollback: found=%v err=%v", found, err)
	}
	if cp != prev {
		t.Fatalf("checkpoint changed: got %+v want %+v", cp, prev)
	}
}

func TestScanRepositoryR02CheckpointConflict(t *testing.T) {
	db := openTestDB(t)
	resetSchema(t, db)
	ctx := context.Background()
	repo := newScanRepository(db)

	t.Run("first_insert_conflict", func(t *testing.T) {
		resetSchema(t, db)
		seed := domain.BlockBatch{
			Block: domain.Block{
				Slot:              20,
				Blockhash:         "Hash20xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				ParentSlot:        19,
				PreviousBlockhash: "Prev19xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				TransactionCount:  0,
			},
		}
		if err := repo.SaveBlockBatch(ctx, domain.Checkpoint{}, seed); err != nil {
			t.Fatalf("seed: %v", err)
		}

		conflictBatch := domain.BlockBatch{
			Block: domain.Block{
				Slot:              21,
				Blockhash:         "Hash21xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				ParentSlot:        20,
				PreviousBlockhash: seed.Block.Blockhash,
				TransactionCount:  0,
			},
		}
		err := repo.SaveBlockBatch(ctx, domain.Checkpoint{}, conflictBatch)
		if !errors.Is(err, repository.ErrCheckpointConflict) {
			t.Fatalf("err=%v want ErrCheckpointConflict", err)
		}
		assertSlotAbsent(t, db, 21)
		cp, found, getErr := repo.GetCheckpoint(ctx)
		if getErr != nil || !found || cp.Slot != 20 {
			t.Fatalf("checkpoint after first-insert conflict: found=%v cp=%+v err=%v", found, cp, getErr)
		}
	})

	t.Run("stale_prev_update", func(t *testing.T) {
		resetSchema(t, db)
		seed := domain.BlockBatch{
			Block: domain.Block{
				Slot:              30,
				Blockhash:         "Hash30xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				ParentSlot:        29,
				PreviousBlockhash: "Prev29xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				TransactionCount:  0,
			},
		}
		if err := repo.SaveBlockBatch(ctx, domain.Checkpoint{}, seed); err != nil {
			t.Fatalf("seed: %v", err)
		}

		stale := domain.Checkpoint{Slot: 29, Blockhash: "StaleHashxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}
		next := domain.BlockBatch{
			Block: domain.Block{
				Slot:              31,
				Blockhash:         "Hash31xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				ParentSlot:        30,
				PreviousBlockhash: seed.Block.Blockhash,
				TransactionCount:  0,
			},
		}
		err := repo.SaveBlockBatch(ctx, stale, next)
		if !errors.Is(err, repository.ErrCheckpointConflict) {
			t.Fatalf("err=%v want ErrCheckpointConflict", err)
		}
		assertSlotAbsent(t, db, 31)
		cp, found, getErr := repo.GetCheckpoint(ctx)
		if getErr != nil || !found || cp.Slot != 30 || cp.Blockhash != seed.Block.Blockhash {
			t.Fatalf("checkpoint after stale update: found=%v cp=%+v err=%v", found, cp, getErr)
		}
	})
}

func TestScanRepositoryRoundTrip(t *testing.T) {
	db := openTestDB(t)
	resetSchema(t, db)
	ctx := context.Background()
	repo := newScanRepository(db)

	const maxUint64 = uint64(math.MaxUint64)
	batch := domain.BlockBatch{
		Block: domain.Block{
			Slot:              50,
			Blockhash:         "RoundHash50xxxxxxxxxxxxxxxxxxxxxxxxxx",
			ParentSlot:        49,
			PreviousBlockhash: "RoundPrev49xxxxxxxxxxxxxxxxxxxxxxxxxx",
			HasBlockHeight:    false,
			HasBlockTime:      false,
			TransactionCount:  2,
		},
		Transactions: []domain.Transaction{
			{
				Slot:      50,
				TxIndex:   0,
				Signature: "RoundSig0xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				Status:    domain.TxStatusSuccess,
				Version:   domain.TxVersionLegacy,
				Fee:       5000,
				FeePayer:  "RoundPayer0xxxxxxxxxxxxxxxxxxxxxxxxxx",
			},
			{
				Slot:      50,
				TxIndex:   1,
				Signature: "RoundSig1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				Status:    domain.TxStatusFailed,
				Version:   domain.TxVersion1,
				Fee:       0,
				FeePayer:  "RoundPayer1xxxxxxxxxxxxxxxxxxxxxxxxxx",
			},
		},
		Transfers: []domain.Transfer{
			{
				Slot:        50,
				TxIndex:     0,
				IxIndex:     0,
				InnerIndex:  domain.TopLevelInnerIndex,
				Program:     domain.ProgramSystem,
				Kind:        domain.KindTransfer,
				HasDecimals: false,
				FromAccount: "RoundFrom0xxxxxxxxxxxxxxxxxxxxxxxxxxx",
				FromOwner:   "",
				ToAccount:   "RoundTo0xxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				ToOwner:     "",
				Amount:      maxUint64,
				HasFee:      false,
			},
			{
				Slot:        50,
				TxIndex:     0,
				IxIndex:     1,
				InnerIndex:  0,
				Program:     domain.ProgramToken2022,
				Kind:        domain.KindTransferCheckedWithFee,
				Mint:        "RoundMintxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				Decimals:    0,
				HasDecimals: true,
				FromAccount: "RoundFrom1xxxxxxxxxxxxxxxxxxxxxxxxxxx",
				FromOwner:   "RoundFromOwner1xxxxxxxxxxxxxxxxxxxxxx",
				ToAccount:   "RoundTo1xxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				ToOwner:     "RoundToOwner1xxxxxxxxxxxxxxxxxxxxxxxx",
				Amount:      0,
				Fee:         5,
				HasFee:      true,
			},
		},
		BalanceChanges: []domain.TokenBalanceChange{
			{
				Slot:         50,
				TxIndex:      0,
				AccountIndex: 2,
				Program:      domain.ProgramToken2022,
				Account:      "RoundBalAccxxxxxxxxxxxxxxxxxxxxxxxxxx",
				Owner:        "RoundBalOwnerxxxxxxxxxxxxxxxxxxxxxxxx",
				Mint:         "RoundBalMintxxxxxxxxxxxxxxxxxxxxxxxxx",
				Decimals:     0,
				PreAmount:    maxUint64,
				PostAmount:   0,
			},
		},
	}

	if err := repo.SaveBlockBatch(ctx, domain.Checkpoint{}, batch); err != nil {
		t.Fatalf("SaveBlockBatch: %v", err)
	}

	cp, found, err := repo.GetCheckpoint(ctx)
	if err != nil || !found {
		t.Fatalf("GetCheckpoint: found=%v err=%v", found, err)
	}
	if cp.Slot != 50 || cp.Blockhash != batch.Block.Blockhash {
		t.Fatalf("checkpoint=%+v", cp)
	}

	var blockRow struct {
		Slot              int64
		Blockhash         string
		ParentSlot        int64
		PreviousBlockhash string
		BlockHeight       *int64
		BlockTime         *int64
		TransactionCount  int
	}
	if err := db.Raw(`SELECT slot, blockhash, parent_slot, previous_blockhash, block_height, block_time, transaction_count
		FROM sol_blocks WHERE slot = ?`, 50).Scan(&blockRow).Error; err != nil {
		t.Fatalf("read block: %v", err)
	}
	if blockRow.Slot != 50 || blockRow.Blockhash != batch.Block.Blockhash {
		t.Fatalf("block row = %+v", blockRow)
	}
	if blockRow.BlockHeight != nil || blockRow.BlockTime != nil {
		t.Fatalf("want NULL height/time, got height=%#v time=%#v", blockRow.BlockHeight, blockRow.BlockTime)
	}
	if blockRow.TransactionCount != 2 {
		t.Fatalf("transaction_count=%d", blockRow.TransactionCount)
	}

	var txFees []int64
	if err := db.Raw(`SELECT fee FROM sol_transactions WHERE slot = ? ORDER BY tx_index`, 50).Scan(&txFees).Error; err != nil {
		t.Fatalf("read txs: %v", err)
	}
	if len(txFees) != 2 || txFees[0] != 5000 || txFees[1] != 0 {
		t.Fatalf("tx fees = %#v", txFees)
	}

	type transferRow struct {
		IxIndex    int16
		InnerIndex int16
		Amount     string
		Fee        *string
		Decimals   *int16
	}
	var transferRows []transferRow
	if err := db.Raw(`SELECT ix_index, inner_index, amount::text AS amount, fee::text AS fee, decimals
		FROM sol_transfers WHERE slot = ? ORDER BY tx_index, ix_index, inner_index`, 50).Scan(&transferRows).Error; err != nil {
		t.Fatalf("read transfers: %v", err)
	}
	if len(transferRows) != 2 {
		t.Fatalf("transfer rows=%d", len(transferRows))
	}
	if transferRows[0].InnerIndex != -1 || transferRows[0].Amount != "18446744073709551615" || transferRows[0].Fee != nil || transferRows[0].Decimals != nil {
		t.Fatalf("transfer[0]=%+v", transferRows[0])
	}
	if transferRows[1].InnerIndex != 0 || transferRows[1].Amount != "0" {
		t.Fatalf("transfer[1]=%+v", transferRows[1])
	}
	if transferRows[1].Fee == nil || *transferRows[1].Fee != "5" {
		t.Fatalf("transfer[1].Fee=%#v want \"5\"", transferRows[1].Fee)
	}
	if transferRows[1].Decimals == nil || *transferRows[1].Decimals != 0 {
		t.Fatalf("transfer[1].Decimals=%#v want 0", transferRows[1].Decimals)
	}

	type balRow struct {
		Decimals   int16
		PreAmount  string
		PostAmount string
	}
	var balRows []balRow
	if err := db.Raw(`SELECT decimals, pre_amount::text AS pre_amount, post_amount::text AS post_amount
		FROM sol_token_balance_changes WHERE slot = ?`, 50).Scan(&balRows).Error; err != nil {
		t.Fatalf("read balances: %v", err)
	}
	if len(balRows) != 1 || balRows[0].Decimals != 0 || balRows[0].PreAmount != "18446744073709551615" || balRows[0].PostAmount != "0" {
		t.Fatalf("balance rows=%#v", balRows)
	}

	// Second block with height/time present (including zero) to distinguish NULL vs 0.
	prev := cp
	second := domain.BlockBatch{
		Block: domain.Block{
			Slot:              51,
			Blockhash:         "RoundHash51xxxxxxxxxxxxxxxxxxxxxxxxxx",
			ParentSlot:        50,
			PreviousBlockhash: batch.Block.Blockhash,
			BlockHeight:       0,
			HasBlockHeight:    true,
			BlockTime:         0,
			HasBlockTime:      true,
			TransactionCount:  0,
		},
	}
	if err := repo.SaveBlockBatch(ctx, prev, second); err != nil {
		t.Fatalf("SaveBlockBatch second: %v", err)
	}
	var heightTime struct {
		BlockHeight *int64
		BlockTime   *int64
	}
	if err := db.Raw(`SELECT block_height, block_time FROM sol_blocks WHERE slot = ?`, 51).Scan(&heightTime).Error; err != nil {
		t.Fatalf("read height/time: %v", err)
	}
	if heightTime.BlockHeight == nil || *heightTime.BlockHeight != 0 {
		t.Fatalf("block_height=%#v want 0", heightTime.BlockHeight)
	}
	if heightTime.BlockTime == nil || *heightTime.BlockTime != 0 {
		t.Fatalf("block_time=%#v want 0", heightTime.BlockTime)
	}
}

func TestScanRepositoryHalts(t *testing.T) {
	db := openTestDB(t)
	resetSchema(t, db)
	ctx := context.Background()
	repo := newScanRepository(db)

	unresolved, err := repo.HasUnresolvedHalt(ctx)
	if err != nil {
		t.Fatalf("HasUnresolvedHalt empty: %v", err)
	}
	if unresolved {
		t.Fatal("HasUnresolvedHalt on empty DB = true, want false")
	}

	halt := domain.Halt{
		Reason:    domain.HaltReasonHashMismatch,
		Slot:      99,
		Detail:    "expected=aaa actual=bbb",
		CreatedAt: 1_700_000_001,
	}
	if err := repo.CreateHalt(ctx, halt); err != nil {
		t.Fatalf("CreateHalt: %v", err)
	}
	unresolved, err = repo.HasUnresolvedHalt(ctx)
	if err != nil {
		t.Fatalf("HasUnresolvedHalt after create: %v", err)
	}
	if !unresolved {
		t.Fatal("HasUnresolvedHalt after create = false, want true")
	}

	if err := db.Exec(`UPDATE sol_scan_halts SET resolved_at = ? WHERE slot = ? AND resolved_at IS NULL`, int64(1_700_000_002), halt.Slot).Error; err != nil {
		t.Fatalf("resolve halt: %v", err)
	}
	unresolved, err = repo.HasUnresolvedHalt(ctx)
	if err != nil {
		t.Fatalf("HasUnresolvedHalt after resolve: %v", err)
	}
	if unresolved {
		t.Fatal("HasUnresolvedHalt after resolve = true, want false")
	}
}

func assertSlotAbsent(t *testing.T, db *gorm.DB, slot uint64) {
	t.Helper()
	tables := []string{
		"sol_blocks",
		"sol_transactions",
		"sol_transfers",
		"sol_token_balance_changes",
	}
	for _, table := range tables {
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM "+table+" WHERE slot = ?", slot).Scan(&count).Error; err != nil {
			t.Fatalf("count %s slot %d: %v", table, slot, err)
		}
		if count != 0 {
			t.Fatalf("%s still has %d row(s) for slot %d", table, count, slot)
		}
	}
}
