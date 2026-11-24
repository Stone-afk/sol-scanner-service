package parser

import (
	"errors"
	"fmt"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

// ErrDecode is the sentinel for any block/transaction decode failure.
var ErrDecode = errors.New("parser: decode error")

// DecodeError pinpoints which transaction/instruction failed to decode.
type DecodeError struct {
	TxIndex    int
	IxIndex    int
	InnerIndex int
	Reason     string
}

func (e DecodeError) Error() string {
	return fmt.Sprintf("parser: decode error tx=%d ix=%d inner=%d: %s", e.TxIndex, e.IxIndex, e.InnerIndex, e.Reason)
}

// Is reports whether target is ErrDecode.
func (e DecodeError) Is(target error) bool {
	return target == ErrDecode
}

// ParseBlock maps a finalized getBlock result into a domain BlockBatch.
func ParseBlock(slot uint64, blk *rpc.GetBlockResult) (domain.BlockBatch, error) {
	if blk == nil {
		return domain.BlockBatch{}, ErrDecode
	}

	batch := domain.BlockBatch{
		Block: domain.Block{
			Slot:              slot,
			Blockhash:         blk.Blockhash.String(),
			ParentSlot:        blk.ParentSlot,
			PreviousBlockhash: blk.PreviousBlockhash.String(),
			TransactionCount:  len(blk.Transactions),
		},
		Transactions:   make([]domain.Transaction, 0, len(blk.Transactions)),
		Transfers:      make([]domain.Transfer, 0),
		BalanceChanges: make([]domain.TokenBalanceChange, 0),
	}
	if blk.BlockHeight != nil {
		batch.Block.BlockHeight = *blk.BlockHeight
		batch.Block.HasBlockHeight = true
	}
	if blk.BlockTime != nil {
		batch.Block.BlockTime = int64(*blk.BlockTime)
		batch.Block.HasBlockTime = true
	}

	for i, twm := range blk.Transactions {
		if twm.Meta == nil {
			return domain.BlockBatch{}, DecodeError{TxIndex: i, IxIndex: -1, InnerIndex: -1, Reason: "meta"}
		}
		tx, err := twm.GetTransaction()
		if err != nil {
			return domain.BlockBatch{}, DecodeError{TxIndex: i, IxIndex: -1, InnerIndex: -1, Reason: "transaction"}
		}
		if len(tx.Signatures) == 0 {
			return domain.BlockBatch{}, DecodeError{TxIndex: i, IxIndex: -1, InnerIndex: -1, Reason: "signature"}
		}
		if len(tx.Message.AccountKeys) == 0 {
			return domain.BlockBatch{}, DecodeError{TxIndex: i, IxIndex: -1, InnerIndex: -1, Reason: "fee_payer"}
		}

		version, err := mapTxVersion(twm.Version)
		if err != nil {
			return domain.BlockBatch{}, DecodeError{TxIndex: i, IxIndex: -1, InnerIndex: -1, Reason: "version"}
		}

		status := domain.TxStatusSuccess
		if twm.Meta.Err != nil {
			status = domain.TxStatusFailed
		}

		batch.Transactions = append(batch.Transactions, domain.Transaction{
			Slot:      slot,
			TxIndex:   i,
			Signature: tx.Signatures[0].String(),
			Status:    status,
			Version:   version,
			Fee:       twm.Meta.Fee,
			FeePayer:  tx.Message.AccountKeys[0].String(),
		})

		if status == domain.TxStatusFailed {
			continue
		}
		keys := accountKeys(tx, twm.Meta)
		trs, err := parseTransfers(slot, i, keys, tx, twm.Meta)
		if err != nil {
			return domain.BlockBatch{}, err
		}
		batch.Transfers = append(batch.Transfers, trs...)
		bcs, err := parseBalanceChangesChecked(slot, i, keys, twm.Meta)
		if err != nil {
			return domain.BlockBatch{}, err
		}
		batch.BalanceChanges = append(batch.BalanceChanges, bcs...)
	}
	return batch, nil
}

// 交易消息版本。SDK 只导出 legacy。
const (
	txVersionV0 rpc.TransactionVersion = 0
	txVersionV1 rpc.TransactionVersion = 1
)

func mapTxVersion(v rpc.TransactionVersion) (domain.TxVersion, error) {
	switch v {
	case rpc.LegacyTransactionVersion:
		return domain.TxVersionLegacy, nil
	case txVersionV0:
		return domain.TxVersion0, nil
	case txVersionV1:
		return domain.TxVersion1, nil
	default:
		return 0, fmt.Errorf("unsupported transaction version %d", v)
	}
}

// accountKeys returns static message keys plus ALT loaded addresses (writable then readonly).
func accountKeys(tx *solana.Transaction, meta *rpc.TransactionMeta) []solana.PublicKey {
	static := tx.Message.AccountKeys
	n := len(static) + len(meta.LoadedAddresses.Writable) + len(meta.LoadedAddresses.ReadOnly)
	out := make([]solana.PublicKey, 0, n)
	out = append(out, static...)
	out = append(out, meta.LoadedAddresses.Writable...)
	out = append(out, meta.LoadedAddresses.ReadOnly...)
	return out
}
