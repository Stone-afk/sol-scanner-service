package repository

import (
	"strconv"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
)

func toEntities(batch domain.BlockBatch) postgre.ScanBatch {
	block := postgre.Block{
		Slot:              batch.Block.Slot,
		Blockhash:         batch.Block.Blockhash,
		ParentSlot:        batch.Block.ParentSlot,
		PreviousBlockhash: batch.Block.PreviousBlockhash,
		TransactionCount:  batch.Block.TransactionCount,
	}
	if batch.Block.HasBlockHeight {
		height := int64(batch.Block.BlockHeight)
		block.BlockHeight = &height
	}
	if batch.Block.HasBlockTime {
		t := batch.Block.BlockTime
		block.BlockTime = &t
	}

	txs := make([]postgre.Transaction, len(batch.Transactions))
	for i, tx := range batch.Transactions {
		txs[i] = postgre.Transaction{
			Slot:      tx.Slot,
			TxIndex:   tx.TxIndex,
			Signature: tx.Signature,
			Status:    tx.Status.ToInt16(),
			Version:   tx.Version.ToInt16(),
			Fee:       tx.Fee,
			FeePayer:  tx.FeePayer,
		}
	}

	transfers := make([]postgre.Transfer, len(batch.Transfers))
	for i, tr := range batch.Transfers {
		entity := postgre.Transfer{
			Slot:        tr.Slot,
			TxIndex:     tr.TxIndex,
			IxIndex:     int16(tr.IxIndex),
			InnerIndex:  int16(tr.InnerIndex),
			Program:     tr.Program.ToInt16(),
			Kind:        tr.Kind.ToInt16(),
			Mint:        tr.Mint,
			FromAccount: tr.FromAccount,
			FromOwner:   tr.FromOwner,
			ToAccount:   tr.ToAccount,
			ToOwner:     tr.ToOwner,
			Amount:      strconv.FormatUint(tr.Amount, 10),
		}
		if tr.HasDecimals {
			d := int16(tr.Decimals)
			entity.Decimals = &d
		}
		if tr.HasFee {
			fee := strconv.FormatUint(tr.Fee, 10)
			entity.Fee = &fee
		}
		transfers[i] = entity
	}

	balances := make([]postgre.TokenBalanceChange, len(batch.BalanceChanges))
	for i, bc := range batch.BalanceChanges {
		balances[i] = postgre.TokenBalanceChange{
			Slot:         bc.Slot,
			TxIndex:      bc.TxIndex,
			AccountIndex: int16(bc.AccountIndex),
			Program:      bc.Program.ToInt16(),
			Account:      bc.Account,
			Owner:        bc.Owner,
			Mint:         bc.Mint,
			Decimals:     int16(bc.Decimals),
			PreAmount:    strconv.FormatUint(bc.PreAmount, 10),
			PostAmount:   strconv.FormatUint(bc.PostAmount, 10),
		}
	}

	return postgre.ScanBatch{
		Block:          block,
		Transactions:   txs,
		Transfers:      transfers,
		BalanceChanges: balances,
	}
}

func fromEntities(batch postgre.ScanBatch) domain.BlockBatch {
	block := batch.Block
	out := domain.BlockBatch{
		Block: domain.Block{
			Slot:              block.Slot,
			Blockhash:         block.Blockhash,
			ParentSlot:        block.ParentSlot,
			PreviousBlockhash: block.PreviousBlockhash,
			TransactionCount:  block.TransactionCount,
		},
		Transactions:   make([]domain.Transaction, len(batch.Transactions)),
		Transfers:      make([]domain.Transfer, len(batch.Transfers)),
		BalanceChanges: make([]domain.TokenBalanceChange, len(batch.BalanceChanges)),
	}
	if block.BlockHeight != nil {
		out.Block.HasBlockHeight = true
		out.Block.BlockHeight = uint64(*block.BlockHeight)
	}
	if block.BlockTime != nil {
		out.Block.HasBlockTime = true
		out.Block.BlockTime = *block.BlockTime
	}

	for i, tx := range batch.Transactions {
		out.Transactions[i] = domain.Transaction{
			Slot:      tx.Slot,
			TxIndex:   tx.TxIndex,
			Signature: tx.Signature,
			Status:    domain.TxStatus(tx.Status),
			Version:   domain.TxVersion(tx.Version),
			Fee:       tx.Fee,
			FeePayer:  tx.FeePayer,
		}
	}

	for i, tr := range batch.Transfers {
		transfer := domain.Transfer{
			Slot:        tr.Slot,
			TxIndex:     tr.TxIndex,
			IxIndex:     int(tr.IxIndex),
			InnerIndex:  int(tr.InnerIndex),
			Program:     domain.Program(tr.Program),
			Kind:        domain.TransferKind(tr.Kind),
			Mint:        tr.Mint,
			FromAccount: tr.FromAccount,
			FromOwner:   tr.FromOwner,
			ToAccount:   tr.ToAccount,
			ToOwner:     tr.ToOwner,
			Amount:      mustParseUint64(tr.Amount),
		}
		if tr.Decimals != nil {
			transfer.HasDecimals = true
			transfer.Decimals = uint8(*tr.Decimals)
		}
		if tr.Fee != nil {
			transfer.HasFee = true
			transfer.Fee = mustParseUint64(*tr.Fee)
		}
		out.Transfers[i] = transfer
	}

	for i, bc := range batch.BalanceChanges {
		out.BalanceChanges[i] = domain.TokenBalanceChange{
			Slot:         bc.Slot,
			TxIndex:      bc.TxIndex,
			AccountIndex: int(bc.AccountIndex),
			Program:      domain.Program(bc.Program),
			Account:      bc.Account,
			Owner:        bc.Owner,
			Mint:         bc.Mint,
			Decimals:     uint8(bc.Decimals),
			PreAmount:    mustParseUint64(bc.PreAmount),
			PostAmount:   mustParseUint64(bc.PostAmount),
		}
	}

	// Normalize empty slices to nil so DeepEqual with domain fixtures that use nil matches.
	if len(out.Transactions) == 0 {
		out.Transactions = nil
	}
	if len(out.Transfers) == 0 {
		out.Transfers = nil
	}
	if len(out.BalanceChanges) == 0 {
		out.BalanceChanges = nil
	}

	return out
}

func mustParseUint64(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func checkpointEntity(c domain.Checkpoint) postgre.Checkpoint {
	if c == (domain.Checkpoint{}) {
		return postgre.Checkpoint{}
	}
	return postgre.Checkpoint{
		ID:            1,
		LastBlockSlot: c.Slot,
		LastBlockhash: c.Blockhash,
	}
}

func checkpointFromEntity(e postgre.Checkpoint) domain.Checkpoint {
	return domain.Checkpoint{Slot: e.LastBlockSlot, Blockhash: e.LastBlockhash}
}

func haltEntity(h domain.Halt) postgre.Halt {
	return postgre.Halt{
		Reason:    h.Reason.ToString(),
		Slot:      h.Slot,
		Detail:    h.Detail,
		CreatedAt: h.CreatedAt,
	}
}
