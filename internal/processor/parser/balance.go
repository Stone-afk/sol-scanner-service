package parser

import (
	"sort"
	"strconv"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

type mergedTokenBalance struct {
	accountIndex uint16
	programOK    bool
	owner        string
	mint         string
	decimals     uint8
	preAmount    uint64
	postAmount   uint64
	hasPre       bool
	hasPost      bool
}

// parseBalanceChanges emits Token-2022 pre/post deltas (plan signature; no error).
// ParseBlock uses parseBalanceChangesChecked so amount ParseUint failures become DecodeError.
func parseBalanceChanges(slot uint64, txIndex int, keys []solana.PublicKey, meta *rpc.TransactionMeta) []domain.TokenBalanceChange {
	changes, _ := parseBalanceChangesChecked(slot, txIndex, keys, meta)
	return changes
}

func parseBalanceChangesChecked(slot uint64, txIndex int, keys []solana.PublicKey, meta *rpc.TransactionMeta) ([]domain.TokenBalanceChange, error) {
	if meta == nil {
		return nil, nil
	}
	merged := map[uint16]*mergedTokenBalance{}
	apply := func(b rpc.TokenBalance, isPre bool) error {
		m, ok := merged[b.AccountIndex]
		if !ok {
			m = &mergedTokenBalance{accountIndex: b.AccountIndex}
			merged[b.AccountIndex] = m
		}
		if b.ProgramId != nil && b.ProgramId.Equals(solana.Token2022ProgramID) {
			m.programOK = true
		}
		if b.Owner != nil {
			if isPre {
				if m.owner == "" {
					m.owner = b.Owner.String()
				}
			} else {
				m.owner = b.Owner.String()
			}
		}
		if m.mint == "" {
			m.mint = b.Mint.String()
		}
		if b.UiTokenAmount != nil {
			m.decimals = b.UiTokenAmount.Decimals
			amt, err := strconv.ParseUint(b.UiTokenAmount.Amount, 10, 64)
			if err != nil {
				return DecodeError{TxIndex: txIndex, IxIndex: -1, InnerIndex: -1, Reason: "token_amount"}
			}
			if isPre {
				m.preAmount = amt
				m.hasPre = true
			} else {
				m.postAmount = amt
				m.hasPost = true
			}
		}
		return nil
	}
	for _, b := range meta.PreTokenBalances {
		if err := apply(b, true); err != nil {
			return nil, err
		}
	}
	for _, b := range meta.PostTokenBalances {
		if err := apply(b, false); err != nil {
			return nil, err
		}
	}

	out := make([]domain.TokenBalanceChange, 0)
	for _, m := range merged {
		if !m.programOK {
			continue
		}
		pre, post := m.preAmount, m.postAmount
		if !m.hasPre {
			pre = 0
		}
		if !m.hasPost {
			post = 0
		}
		if pre == post {
			continue
		}
		acct := ""
		if int(m.accountIndex) < len(keys) {
			acct = keys[m.accountIndex].String()
		}
		out = append(out, domain.TokenBalanceChange{
			Slot:         slot,
			TxIndex:      txIndex,
			AccountIndex: int(m.accountIndex),
			Program:      domain.ProgramToken2022,
			Account:      acct,
			Owner:        m.owner,
			Mint:         m.mint,
			Decimals:     m.decimals,
			PreAmount:    pre,
			PostAmount:   post,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].AccountIndex < out[j].AccountIndex
	})
	return out, nil
}
