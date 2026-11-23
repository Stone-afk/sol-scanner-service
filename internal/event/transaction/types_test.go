package transaction

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
)

const (
	testChain   = "solana-mainnet"
	fixtureSlot = uint64(471991643)
)

func TestTransactionMessage(t *testing.T) {
	t.Parallel()

	t.Run("golden_tx158_matches_design_5_3", func(t *testing.T) {
		// AC-056 / K-05 / FR-15: fixture slot 471991643 tx158 matches design §5.3
		// after stripping formatting whitespace; transfers[0].index must be "2".
		t.Parallel()
		batch := fixtureTx158Batch()
		msgs := FromBlockBatch(testChain, batch)
		if len(msgs) != 1 {
			t.Fatalf("FromBlockBatch len = %d, want 1", len(msgs))
		}
		got, err := json.Marshal(msgs[0])
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		golden, err := os.ReadFile(filepath.Join("testdata", "k05_message.json"))
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		want, err := compactJSON(golden)
		if err != nil {
			t.Fatalf("compact golden: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("JSON mismatch after whitespace strip:\ngot:  %s\nwant: %s", got, want)
		}
		if msgs[0].Transfers[0].Index != "2" {
			t.Fatalf("transfers[0].index = %q, want %q", msgs[0].Transfers[0].Index, "2")
		}
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal(got, &parsed); err != nil {
			t.Fatalf("unmarshal got: %v", err)
		}
		var transfers []map[string]json.RawMessage
		if err := json.Unmarshal(parsed["transfers"], &transfers); err != nil {
			t.Fatalf("unmarshal transfers: %v", err)
		}
		var index string
		if err := json.Unmarshal(transfers[0]["index"], &index); err != nil {
			t.Fatalf("unmarshal index: %v", err)
		}
		if index != "2" {
			t.Fatalf(`JSON transfers[0].index = %q, want "2"`, index)
		}
	})

	t.Run("same_rows_byte_identical", func(t *testing.T) {
		// FR-15: identical domain rows → byte-identical JSON.
		t.Parallel()
		batch := fixtureTx158Batch()
		a, err := json.Marshal(FromBlockBatch(testChain, batch)[0])
		if err != nil {
			t.Fatalf("Marshal a: %v", err)
		}
		b, err := json.Marshal(FromBlockBatch(testChain, batch)[0])
		if err != nil {
			t.Fatalf("Marshal b: %v", err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("re-encode not byte-identical:\na: %s\nb: %s", a, b)
		}
	})

	t.Run("inner_index_and_omit_fee_decimals_null", func(t *testing.T) {
		// AC-057 / FR-15: inner index "<ix+1>.<inner+1>"; omit fee; decimals null.
		t.Parallel()
		batch := domain.BlockBatch{
			Block: domain.Block{
				Slot:         100,
				Blockhash:    "Hash100",
				BlockTime:    0,
				HasBlockTime: false,
			},
			Transactions: []domain.Transaction{{
				Slot:      100,
				TxIndex:   0,
				Signature: "SigInner",
				Status:    domain.TxStatusSuccess,
				Version:   domain.TxVersion0,
				Fee:       5000,
				FeePayer:  "Payer",
			}},
			Transfers: []domain.Transfer{{
				Slot:        100,
				TxIndex:     0,
				IxIndex:     2,
				InnerIndex:  0, // message index "3.1"
				Program:     domain.ProgramSystem,
				Kind:        domain.KindTransfer,
				Mint:        "",
				HasDecimals: false,
				FromAccount: "From",
				FromOwner:   "FromOwner",
				ToAccount:   "To",
				ToOwner:     "ToOwner",
				Amount:      42,
				HasFee:      false,
			}},
			BalanceChanges: nil,
		}
		msgs := FromBlockBatch(testChain, batch)
		if len(msgs) != 1 {
			t.Fatalf("len = %d, want 1", len(msgs))
		}
		msg := msgs[0]
		if msg.BlockTime != 0 {
			t.Fatalf("block_time = %d, want 0 when HasBlockTime=false", msg.BlockTime)
		}
		if msg.TxVersion != "0" {
			t.Fatalf("tx_version = %q, want %q", msg.TxVersion, "0")
		}
		if len(msg.Transfers) != 1 {
			t.Fatalf("transfers len = %d, want 1", len(msg.Transfers))
		}
		tr := msg.Transfers[0]
		if tr.Index != "3.1" {
			t.Fatalf("inner index = %q, want %q", tr.Index, "3.1")
		}
		if tr.Decimals != nil {
			t.Fatalf("decimals = %v, want null pointer", tr.Decimals)
		}
		if tr.Fee != "" {
			t.Fatalf("fee = %q, want empty for omitempty", tr.Fee)
		}
		raw, err := json.Marshal(tr)
		if err != nil {
			t.Fatalf("Marshal transfer: %v", err)
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatalf("unmarshal transfer: %v", err)
		}
		if _, ok := obj["fee"]; ok {
			t.Fatalf("fee must be omitted when HasFee=false; got %s", raw)
		}
		if string(obj["decimals"]) != "null" {
			t.Fatalf("decimals JSON = %s, want null", obj["decimals"])
		}
	})

	t.Run("signed_change_negative_and_positive", func(t *testing.T) {
		// AC-057 / FR-15: change = post-pre; negative with "-", positive without "+".
		t.Parallel()
		batch := domain.BlockBatch{
			Block: domain.Block{
				Slot:         200,
				Blockhash:    "Hash200",
				BlockTime:    1,
				HasBlockTime: true,
			},
			Transactions: []domain.Transaction{{
				Slot:      200,
				TxIndex:   1,
				Signature: "SigChange",
				Status:    domain.TxStatusSuccess,
				Version:   domain.TxVersion1,
				Fee:       1,
				FeePayer:  "Payer",
			}},
			Transfers: []domain.Transfer{{
				Slot:        200,
				TxIndex:     1,
				IxIndex:     0,
				InnerIndex:  domain.TopLevelInnerIndex,
				Program:     domain.ProgramToken2022,
				Kind:        domain.KindTransferCheckedWithFee,
				Mint:        "Mint",
				Decimals:    6,
				HasDecimals: true,
				FromAccount: "A",
				FromOwner:   "AO",
				ToAccount:   "B",
				ToOwner:     "BO",
				Amount:      100,
				Fee:         7,
				HasFee:      true,
			}},
			BalanceChanges: []domain.TokenBalanceChange{
				{
					Slot: 200, TxIndex: 1, AccountIndex: 0,
					Program: domain.ProgramToken2022,
					Account: "A", Owner: "AO", Mint: "Mint", Decimals: 6,
					PreAmount: 100, PostAmount: 0,
				},
				{
					Slot: 200, TxIndex: 1, AccountIndex: 1,
					Program: domain.ProgramToken2022,
					Account: "B", Owner: "BO", Mint: "Mint", Decimals: 6,
					PreAmount: 0, PostAmount: 93,
				},
			},
		}
		msgs := FromBlockBatch(testChain, batch)
		if len(msgs) != 1 {
			t.Fatalf("len = %d, want 1", len(msgs))
		}
		msg := msgs[0]
		if msg.TxVersion != "1" {
			t.Fatalf("tx_version = %q, want %q", msg.TxVersion, "1")
		}
		if msg.Transfers[0].Fee != "7" {
			t.Fatalf("transfer fee = %q, want %q", msg.Transfers[0].Fee, "7")
		}
		if msg.Transfers[0].Index != "1" {
			t.Fatalf("top-level index = %q, want %q", msg.Transfers[0].Index, "1")
		}
		if len(msg.TokenBalanceChanges) != 2 {
			t.Fatalf("balance changes len = %d, want 2", len(msg.TokenBalanceChanges))
		}
		if msg.TokenBalanceChanges[0].Change != "-100" {
			t.Fatalf("negative change = %q, want %q", msg.TokenBalanceChanges[0].Change, "-100")
		}
		if msg.TokenBalanceChanges[1].Change != "93" {
			t.Fatalf("positive change = %q, want %q (no +)", msg.TokenBalanceChanges[1].Change, "93")
		}
		raw, err := json.Marshal(msg.TokenBalanceChanges[1])
		if err != nil {
			t.Fatalf("Marshal change: %v", err)
		}
		if bytes.Contains(raw, []byte(`"+`)) || bytes.Contains(raw, []byte(`"change":"+`)) {
			t.Fatalf("positive change must not include +; got %s", raw)
		}
	})
}

func TestFailedTransactionJSON(t *testing.T) {
	// AC-057 / FR-15: failed transaction transfers and token_balance_changes encode as [].
	t.Parallel()
	batch := domain.BlockBatch{
		Block: domain.Block{
			Slot:         300,
			Blockhash:    "Hash300",
			BlockTime:    9,
			HasBlockTime: true,
		},
		Transactions: []domain.Transaction{{
			Slot:      300,
			TxIndex:   0,
			Signature: "SigFailed",
			Status:    domain.TxStatusFailed,
			Version:   domain.TxVersionLegacy,
			Fee:       5000,
			FeePayer:  "FailedPayer",
		}},
		// Domain rows for failed txs should not appear; even if present, message arrays stay [].
		Transfers: []domain.Transfer{{
			Slot: 300, TxIndex: 0, IxIndex: 0, InnerIndex: domain.TopLevelInnerIndex,
			Program: domain.ProgramSystem, Kind: domain.KindTransfer,
			FromAccount: "X", ToAccount: "Y", Amount: 1,
		}},
		BalanceChanges: []domain.TokenBalanceChange{{
			Slot: 300, TxIndex: 0, Account: "X", Owner: "O", Mint: "M",
			Program: domain.ProgramToken2022, Decimals: 0, PreAmount: 1, PostAmount: 0,
		}},
	}
	msgs := FromBlockBatch(testChain, batch)
	if len(msgs) != 1 {
		t.Fatalf("len = %d, want 1", len(msgs))
	}
	msg := msgs[0]
	if msg.Status != "FAILED" {
		t.Fatalf("status = %q, want FAILED", msg.Status)
	}
	if msg.Transfers == nil || msg.TokenBalanceChanges == nil {
		t.Fatalf("arrays must be non-nil empty slices; transfers=%v changes=%v", msg.Transfers, msg.TokenBalanceChanges)
	}
	if len(msg.Transfers) != 0 || len(msg.TokenBalanceChanges) != 0 {
		t.Fatalf("failed arrays must be empty; transfers=%d changes=%d", len(msg.Transfers), len(msg.TokenBalanceChanges))
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(obj["transfers"]) != "[]" {
		t.Fatalf("transfers JSON = %s, want []", obj["transfers"])
	}
	if string(obj["token_balance_changes"]) != "[]" {
		t.Fatalf("token_balance_changes JSON = %s, want []", obj["token_balance_changes"])
	}
}

func fixtureTx158Batch() domain.BlockBatch {
	decimals := uint8(9)
	return domain.BlockBatch{
		Block: domain.Block{
			Slot:              fixtureSlot,
			Blockhash:         "JEH9Bbb2v8fvqLxQGVZnmqFbp33eAwXoxXyBugWYgTzw",
			ParentSlot:        471991642,
			PreviousBlockhash: "E9PewXqNLx73rVD8ieGs2riz2ENxLziQTm29sLBCL2wr",
			BlockHeight:       450005612,
			HasBlockHeight:    true,
			BlockTime:         1795232417,
			HasBlockTime:      true,
			TransactionCount:  457,
		},
		Transactions: []domain.Transaction{{
			Slot:      fixtureSlot,
			TxIndex:   158,
			Signature: "53M1anGXE7xv438gWGcbPwDNwnd75f4Uj9Huh84xYNmzK9QDNF7PCsqkpZYUP2Xuq1jca6A5BsjPncXfHt13mQ5g",
			Status:    domain.TxStatusSuccess,
			Version:   domain.TxVersionLegacy,
			Fee:       10000,
			FeePayer:  "9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq",
		}},
		Transfers: []domain.Transfer{{
			Slot:        fixtureSlot,
			TxIndex:     158,
			IxIndex:     1, // message index "2"
			InnerIndex:  domain.TopLevelInnerIndex,
			Program:     domain.ProgramToken2022,
			Kind:        domain.KindTransferChecked,
			Mint:        "Ey59PH7Z4BFU4HjyKnyMdWt5GGN76KazTAwQihoUXRnk",
			Decimals:    decimals,
			HasDecimals: true,
			FromAccount: "EnHdUh7vi1Fo1ncfHB4HbMpmd4CW6fnVLv1fTgPEmczL",
			FromOwner:   "ETvhVmPeahyycn67xs3qaZK6PCD77V3UHY9ojEwQ3aAi",
			ToAccount:   "47AJoNqacLygyXKpTWZd7rzdcsuCcBx39Gdc6gn5FhND",
			ToOwner:     "9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq",
			Amount:      98000000,
			HasFee:      false,
		}},
		// Order matches design §5.3 example (EnHd then 47AJo), not account_index sort.
		BalanceChanges: []domain.TokenBalanceChange{
			{
				Slot:         fixtureSlot,
				TxIndex:      158,
				AccountIndex: 3,
				Program:      domain.ProgramToken2022,
				Account:      "EnHdUh7vi1Fo1ncfHB4HbMpmd4CW6fnVLv1fTgPEmczL",
				Owner:        "ETvhVmPeahyycn67xs3qaZK6PCD77V3UHY9ojEwQ3aAi",
				Mint:         "Ey59PH7Z4BFU4HjyKnyMdWt5GGN76KazTAwQihoUXRnk",
				Decimals:     9,
				PreAmount:    98000000,
				PostAmount:   0,
			},
			{
				Slot:         fixtureSlot,
				TxIndex:      158,
				AccountIndex: 2,
				Program:      domain.ProgramToken2022,
				Account:      "47AJoNqacLygyXKpTWZd7rzdcsuCcBx39Gdc6gn5FhND",
				Owner:        "9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq",
				Mint:         "Ey59PH7Z4BFU4HjyKnyMdWt5GGN76KazTAwQihoUXRnk",
				Decimals:     9,
				PreAmount:    275870625812208,
				PostAmount:   275870721852208,
			},
		},
	}
}

func compactJSON(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
