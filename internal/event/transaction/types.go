package transaction

import (
	"strconv"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
)

const (
	MessageVersion      = 1
	CommitmentFinalized = "finalized"

	StatusSuccess = "SUCCESS"
	StatusFailed  = "FAILED"

	TxVersionLegacy = "legacy"
	TxVersion0      = "0"
	TxVersion1      = "1"

	ProgramSystem    = "system"
	ProgramSPLToken  = "spl-token"
	ProgramToken2022 = "token-2022"
)

// TransactionMessage is one downstream Kafka payload (design §5.3).
type TransactionMessage struct {
	Version             int                  `json:"version"`
	Chain               string               `json:"chain"`
	Commitment          string               `json:"commitment"`
	Slot                uint64               `json:"slot"`
	Blockhash           string               `json:"blockhash"`
	BlockTime           int64                `json:"block_time"`
	TxIndex             int                  `json:"tx_index"`
	Signature           string               `json:"signature"`
	Status              string               `json:"status"`
	Fee                 uint64               `json:"fee"`
	FeePayer            string               `json:"fee_payer"`
	TxVersion           string               `json:"tx_version"`
	Transfers           []Transfer           `json:"transfers"`
	TokenBalanceChanges []TokenBalanceChange `json:"token_balance_changes"`
}

// Transfer is one in-scope instruction in a TransactionMessage.
type Transfer struct {
	Index       string `json:"index"`
	Program     string `json:"program"`
	Kind        string `json:"kind"`
	Mint        string `json:"mint"`
	Decimals    *int   `json:"decimals"`
	FromAccount string `json:"from_account"`
	FromOwner   string `json:"from_owner"`
	ToAccount   string `json:"to_account"`
	ToOwner     string `json:"to_owner"`
	Amount      string `json:"amount"`
	Fee         string `json:"fee,omitempty"`
}

// TokenBalanceChange is one Token-2022 balance delta in a TransactionMessage.
type TokenBalanceChange struct {
	Account    string `json:"account"`
	Owner      string `json:"owner"`
	Mint       string `json:"mint"`
	Program    string `json:"program"`
	Decimals   int    `json:"decimals"`
	PreAmount  string `json:"pre_amount"`
	PostAmount string `json:"post_amount"`
	Change     string `json:"change"`
}

// FromBlockBatch maps committed domain rows to design §5.3 TransactionMessage values.
// Amounts are decimal strings. Failed transactions always get empty transfer and
// balance-change slices (JSON []).
func FromBlockBatch(chain string, batch domain.BlockBatch) []TransactionMessage {
	blockTime := int64(0)
	if batch.Block.HasBlockTime {
		blockTime = batch.Block.BlockTime
	}

	out := make([]TransactionMessage, 0, len(batch.Transactions))
	for _, tx := range batch.Transactions {
		msg := TransactionMessage{
			Version:             MessageVersion,
			Chain:               chain,
			Commitment:          CommitmentFinalized,
			Slot:                batch.Block.Slot,
			Blockhash:           batch.Block.Blockhash,
			BlockTime:           blockTime,
			TxIndex:             tx.TxIndex,
			Signature:           tx.Signature,
			Status:              mapStatus(tx.Status),
			Fee:                 tx.Fee,
			FeePayer:            tx.FeePayer,
			TxVersion:           mapTxVersion(tx.Version),
			Transfers:           []Transfer{},
			TokenBalanceChanges: []TokenBalanceChange{},
		}
		if tx.Status == domain.TxStatusSuccess {
			msg.Transfers = mapTransfers(batch.Transfers, tx.TxIndex)
			msg.TokenBalanceChanges = mapBalanceChanges(batch.BalanceChanges, tx.TxIndex)
		}
		out = append(out, msg)
	}
	return out
}

func mapTransfers(rows []domain.Transfer, txIndex int) []Transfer {
	out := make([]Transfer, 0)
	for _, row := range rows {
		if row.TxIndex != txIndex {
			continue
		}
		tr := Transfer{
			Index:       formatTransferIndex(row.IxIndex, row.InnerIndex),
			Program:     mapProgram(row.Program),
			Kind:        mapKind(row.Kind),
			Mint:        row.Mint,
			FromAccount: row.FromAccount,
			FromOwner:   row.FromOwner,
			ToAccount:   row.ToAccount,
			ToOwner:     row.ToOwner,
			Amount:      strconv.FormatUint(row.Amount, 10),
		}
		if row.HasDecimals {
			d := int(row.Decimals)
			tr.Decimals = &d
		}
		if row.HasFee {
			tr.Fee = strconv.FormatUint(row.Fee, 10)
		}
		out = append(out, tr)
	}
	return out
}

func mapBalanceChanges(rows []domain.TokenBalanceChange, txIndex int) []TokenBalanceChange {
	out := make([]TokenBalanceChange, 0)
	for _, row := range rows {
		if row.TxIndex != txIndex {
			continue
		}
		out = append(out, TokenBalanceChange{
			Account:    row.Account,
			Owner:      row.Owner,
			Mint:       row.Mint,
			Program:    mapProgram(row.Program),
			Decimals:   int(row.Decimals),
			PreAmount:  strconv.FormatUint(row.PreAmount, 10),
			PostAmount: strconv.FormatUint(row.PostAmount, 10),
			Change:     formatChange(row.PreAmount, row.PostAmount),
		})
	}
	return out
}

func formatTransferIndex(ixIndex, innerIndex int) string {
	top := strconv.Itoa(ixIndex + 1)
	if innerIndex == domain.TopLevelInnerIndex {
		return top
	}
	return top + "." + strconv.Itoa(innerIndex+1)
}

func formatChange(pre, post uint64) string {
	if post >= pre {
		return strconv.FormatUint(post-pre, 10)
	}
	return "-" + strconv.FormatUint(pre-post, 10)
}

func mapStatus(s domain.TxStatus) string {
	switch s {
	case domain.TxStatusSuccess:
		return StatusSuccess
	case domain.TxStatusFailed:
		return StatusFailed
	default:
		return ""
	}
}

func mapTxVersion(v domain.TxVersion) string {
	switch v {
	case domain.TxVersionLegacy:
		return TxVersionLegacy
	case domain.TxVersion0:
		return TxVersion0
	case domain.TxVersion1:
		return TxVersion1
	default:
		return ""
	}
}

func mapProgram(p domain.Program) string {
	switch p {
	case domain.ProgramSystem:
		return ProgramSystem
	case domain.ProgramSPLToken:
		return ProgramSPLToken
	case domain.ProgramToken2022:
		return ProgramToken2022
	default:
		return ""
	}
}

func mapKind(k domain.TransferKind) string {
	switch k {
	case domain.KindTransfer:
		return "transfer"
	case domain.KindTransferWithSeed:
		return "transferWithSeed"
	case domain.KindCreateAccount:
		return "createAccount"
	case domain.KindCreateAccountWithSeed:
		return "createAccountWithSeed"
	case domain.KindWithdrawNonceAccount:
		return "withdrawNonceAccount"
	case domain.KindTransferChecked:
		return "transferChecked"
	case domain.KindTransferCheckedWithFee:
		return "transferCheckedWithFee"
	case domain.KindMintTo:
		return "mintTo"
	case domain.KindMintToChecked:
		return "mintToChecked"
	case domain.KindBurn:
		return "burn"
	case domain.KindBurnChecked:
		return "burnChecked"
	default:
		return ""
	}
}
