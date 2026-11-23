package parser

import (
	"sort"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

type tokenBalanceInfo struct {
	owner    string
	mint     string
	decimals uint8
	hasDec   bool
}

func parseTransfers(slot uint64, txIndex int, keys []solana.PublicKey, tx *solana.Transaction, meta *rpc.TransactionMeta) ([]domain.Transfer, error) {
	balanceByIndex := buildBalanceByIndex(meta)
	out := make([]domain.Transfer, 0)

	emit := func(ixIndex, innerIndex int, ix solana.CompiledInstruction) error {
		if int(ix.ProgramIDIndex) >= len(keys) {
			return DecodeError{TxIndex: txIndex, IxIndex: ixIndex, InnerIndex: innerIndex, Reason: "programIdIndex"}
		}
		programID := keys[ix.ProgramIDIndex]
		accounts := make([]solana.PublicKey, len(ix.Accounts))
		for i, idx := range ix.Accounts {
			if int(idx) >= len(keys) {
				return DecodeError{TxIndex: txIndex, IxIndex: ixIndex, InnerIndex: innerIndex, Reason: "account"}
			}
			accounts[i] = keys[idx]
		}
		dec, ok, err := decodeInstruction(programID, accounts, []byte(ix.Data))
		if err != nil {
			if de, is := err.(DecodeError); is {
				de.TxIndex = txIndex
				de.IxIndex = ixIndex
				de.InnerIndex = innerIndex
				return de
			}
			return DecodeError{TxIndex: txIndex, IxIndex: ixIndex, InnerIndex: innerIndex, Reason: err.Error()}
		}
		if !ok {
			return nil
		}
		tr := domain.Transfer{
			Slot:        slot,
			TxIndex:     txIndex,
			IxIndex:     ixIndex,
			InnerIndex:  innerIndex,
			Program:     dec.program,
			Kind:        dec.kind,
			Mint:        dec.mint,
			Decimals:    dec.decimals,
			HasDecimals: dec.hasDecimals,
			FromAccount: dec.from,
			ToAccount:   dec.to,
			Amount:      dec.amount,
			Fee:         dec.fee,
			HasFee:      dec.hasFee,
		}
		out = append(out, backfillTransfer(tr, keys, balanceByIndex))
		return nil
	}

	innersByTop := map[uint16][]solana.CompiledInstruction{}
	if meta != nil {
		for _, inn := range meta.InnerInstructions {
			converted := make([]solana.CompiledInstruction, len(inn.Instructions))
			for i, iix := range inn.Instructions {
				converted[i] = solana.CompiledInstruction{
					ProgramIDIndex: iix.ProgramIDIndex,
					Accounts:       iix.Accounts,
					Data:           solana.Base58(iix.Data),
				}
			}
			innersByTop[inn.Index] = append(innersByTop[inn.Index], converted...)
		}
	}

	for i, ix := range tx.Message.Instructions {
		if err := emit(i, domain.TopLevelInnerIndex, ix); err != nil {
			return nil, err
		}
		for j, inner := range innersByTop[uint16(i)] {
			if err := emit(i, j, inner); err != nil {
				return nil, err
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IxIndex != out[j].IxIndex {
			return out[i].IxIndex < out[j].IxIndex
		}
		return out[i].InnerIndex < out[j].InnerIndex
	})
	return out, nil
}

func buildBalanceByIndex(meta *rpc.TransactionMeta) map[uint16]tokenBalanceInfo {
	out := map[uint16]tokenBalanceInfo{}
	if meta == nil {
		return out
	}
	for _, b := range meta.PreTokenBalances {
		out[b.AccountIndex] = tokenBalanceFromRPC(b)
	}
	for _, b := range meta.PostTokenBalances {
		if _, exists := out[b.AccountIndex]; exists {
			continue // pre wins
		}
		out[b.AccountIndex] = tokenBalanceFromRPC(b)
	}
	return out
}

func tokenBalanceFromRPC(b rpc.TokenBalance) tokenBalanceInfo {
	info := tokenBalanceInfo{mint: b.Mint.String()}
	if b.Owner != nil {
		info.owner = b.Owner.String()
	}
	if b.UiTokenAmount != nil {
		info.decimals = b.UiTokenAmount.Decimals
		info.hasDec = true
	}
	return info
}

func backfillTransfer(tr domain.Transfer, keys []solana.PublicKey, byIndex map[uint16]tokenBalanceInfo) domain.Transfer {
	lookup := func(account string) (tokenBalanceInfo, bool) {
		if account == "" {
			return tokenBalanceInfo{}, false
		}
		for i, k := range keys {
			if k.String() == account {
				info, ok := byIndex[uint16(i)]
				return info, ok
			}
		}
		return tokenBalanceInfo{}, false
	}
	if fromInfo, ok := lookup(tr.FromAccount); ok {
		tr.FromOwner = fromInfo.owner
		if tr.Mint == "" {
			tr.Mint = fromInfo.mint
		}
		if !tr.HasDecimals && fromInfo.hasDec {
			tr.Decimals = fromInfo.decimals
			tr.HasDecimals = true
		}
	}
	if toInfo, ok := lookup(tr.ToAccount); ok {
		tr.ToOwner = toInfo.owner
		if tr.Mint == "" {
			tr.Mint = toInfo.mint
		}
		if !tr.HasDecimals && toInfo.hasDec {
			tr.Decimals = toInfo.decimals
			tr.HasDecimals = true
		}
	}
	return tr
}
