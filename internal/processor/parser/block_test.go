package parser

import (
	"errors"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/test/jsonparsed"
	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

const fixtureSlot = uint64(471991643)

func loadFixtureBlock(t *testing.T) *rpc.GetBlockResult {
	t.Helper()
	return jsonparsed.LoadBase64Block(t, jsonparsed.FixturePath(t, "block_471991643_base64.json.gz"))
}

func loadFixtureParsed(t *testing.T) jsonparsed.ParsedBlock {
	t.Helper()
	return jsonparsed.LoadParsedBlock(t, jsonparsed.FixturePath(t, "block_471991643_jsonparsed.json.gz"))
}

func TestParseBlockNil(t *testing.T) {
	t.Parallel()
	_, err := ParseBlock(fixtureSlot, nil)
	if !errors.Is(err, ErrDecode) {
		t.Fatalf("nil block: got err %v want ErrDecode", err)
	}
}

func TestParseBlockP01Header(t *testing.T) {
	// P-01 / AC-020
	t.Parallel()
	blk := loadFixtureBlock(t)
	batch, err := ParseBlock(fixtureSlot, blk)
	if err != nil {
		t.Fatalf("ParseBlock: %v", err)
	}
	b := batch.Block
	if b.Slot != fixtureSlot {
		t.Fatalf("Slot: got %d want %d", b.Slot, fixtureSlot)
	}
	if b.Blockhash != "JEH9Bbb2v8fvqLxQGVZnmqFbp33eAwXoxXyBugWYgTzw" {
		t.Fatalf("Blockhash: got %q", b.Blockhash)
	}
	if b.ParentSlot != 471991642 {
		t.Fatalf("ParentSlot: got %d want 471991642", b.ParentSlot)
	}
	if b.PreviousBlockhash != "E9PewXqNLx73rVD8ieGs2riz2ENxLziQTm29sLBCL2wr" {
		t.Fatalf("PreviousBlockhash: got %q", b.PreviousBlockhash)
	}
	if !b.HasBlockHeight || b.BlockHeight != 450005612 {
		t.Fatalf("BlockHeight: has=%v value=%d want 450005612", b.HasBlockHeight, b.BlockHeight)
	}
	if !b.HasBlockTime || b.BlockTime != 1795232417 {
		t.Fatalf("BlockTime: has=%v value=%d want 1795232417", b.HasBlockTime, b.BlockTime)
	}
	if b.TransactionCount != 457 {
		t.Fatalf("TransactionCount: got %d want 457", b.TransactionCount)
	}
	if len(batch.Transactions) != 457 {
		t.Fatalf("Transactions len: got %d want 457", len(batch.Transactions))
	}
	if batch.Transfers == nil {
		t.Fatal("Transfers must be non-nil")
	}
	if batch.BalanceChanges == nil {
		t.Fatal("BalanceChanges must be non-nil")
	}
	if len(batch.Transfers) == 0 {
		t.Fatal("Transfers: expected fixture transfers after PLAN-006")
	}
	if len(batch.BalanceChanges) == 0 {
		t.Fatal("BalanceChanges: expected fixture balance changes after PLAN-006")
	}
}

func TestParseBlockP02FeeTransaction(t *testing.T) {
	// P-02 / AC-021
	t.Parallel()
	blk := loadFixtureBlock(t)
	batch, err := ParseBlock(fixtureSlot, blk)
	if err != nil {
		t.Fatalf("ParseBlock: %v", err)
	}
	if len(batch.Transactions) <= 158 {
		t.Fatalf("need tx index 158, have %d", len(batch.Transactions))
	}
	tx := batch.Transactions[158]
	if tx.Slot != fixtureSlot || tx.TxIndex != 158 {
		t.Fatalf("slot/index: got slot=%d idx=%d", tx.Slot, tx.TxIndex)
	}
	wantSig := "53M1anGXE7xv438gWGcbPwDNwnd75f4Uj9Huh84xYNmzK9QDNF7PCsqkpZYUP2Xuq1jca6A5BsjPncXfHt13mQ5g"
	if tx.Signature != wantSig {
		t.Fatalf("Signature: got %q want %q", tx.Signature, wantSig)
	}
	if tx.Status != domain.TxStatusSuccess {
		t.Fatalf("Status: got %d want success", tx.Status)
	}
	if tx.Fee != 10000 {
		t.Fatalf("Fee: got %d want 10000", tx.Fee)
	}
	if tx.FeePayer != "9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq" {
		t.Fatalf("FeePayer: got %q", tx.FeePayer)
	}
	if tx.Version != domain.TxVersionLegacy {
		t.Fatalf("Version: got %d want legacy(-1)", tx.Version)
	}
}

func TestParseBlockP06FailedTransactions(t *testing.T) {
	// P-06 / AC-022
	t.Parallel()
	blk := loadFixtureBlock(t)
	parsed := loadFixtureParsed(t)
	wantFailed := 0
	for _, ptx := range parsed.Transactions {
		if ptx.Failed {
			wantFailed++
		}
	}
	batch, err := ParseBlock(fixtureSlot, blk)
	if err != nil {
		t.Fatalf("ParseBlock: %v", err)
	}
	gotFailed := 0
	for _, tx := range batch.Transactions {
		if tx.Status == domain.TxStatusFailed {
			gotFailed++
		}
	}
	if gotFailed != wantFailed {
		t.Fatalf("failed count: got %d want %d", gotFailed, wantFailed)
	}
	if wantFailed == 0 {
		t.Fatal("fixture has no failed txs")
	}
	failedIdx := map[int]bool{}
	for _, tx := range batch.Transactions {
		if tx.Status == domain.TxStatusFailed {
			failedIdx[tx.TxIndex] = true
		}
	}
	for _, tr := range batch.Transfers {
		if failedIdx[tr.TxIndex] {
			t.Fatalf("failed tx %d must not have transfers", tr.TxIndex)
		}
	}
	for _, bc := range batch.BalanceChanges {
		if failedIdx[bc.TxIndex] {
			t.Fatalf("failed tx %d must not have balance changes", bc.TxIndex)
		}
	}
}

func TestParseBlockP08AccountKeys(t *testing.T) {
	// P-08 / AC-023
	t.Parallel()
	blk := loadFixtureBlock(t)
	parsed := loadFixtureParsed(t)
	if len(blk.Transactions) != len(parsed.Transactions) {
		t.Fatalf("tx count mismatch base64=%d jsonparsed=%d", len(blk.Transactions), len(parsed.Transactions))
	}
	sawLegacy, sawV0, sawV1 := false, false, false
	for i, twm := range blk.Transactions {
		tx, err := twm.GetTransaction()
		if err != nil {
			t.Fatalf("tx[%d] GetTransaction: %v", i, err)
		}
		if twm.Meta == nil {
			t.Fatalf("tx[%d] meta nil", i)
		}
		keys := accountKeys(tx, twm.Meta)
		want := parsed.Transactions[i].AccountKeys
		if len(keys) != len(want) {
			t.Fatalf("tx[%d] accountKeys len: got %d want %d", i, len(keys), len(want))
		}
		for j := range keys {
			if keys[j].String() != want[j] {
				t.Fatalf("tx[%d] accountKeys[%d]: got %s want %s", i, j, keys[j], want[j])
			}
		}
		switch twm.Version {
		case rpc.LegacyTransactionVersion:
			sawLegacy = true
		case txVersionV0:
			sawV0 = true
		case txVersionV1:
			sawV1 = true
		}
	}
	if !sawLegacy || !sawV0 || !sawV1 {
		t.Fatalf("version coverage: legacy=%v v0=%v v1=%v", sawLegacy, sawV0, sawV1)
	}
}

func TestFixtureCoverage(t *testing.T) {
	// FX-01 / AC-024 — version coverage + in-scope inner instructions.
	// Inner transfer assertion deferred to PLAN-006.
	t.Parallel()
	blk := loadFixtureBlock(t)
	parsed := loadFixtureParsed(t)

	sawLegacy, sawV0, sawV1 := false, false, false
	for _, ptx := range parsed.Transactions {
		switch ptx.Version {
		case "legacy":
			sawLegacy = true
		case "0":
			sawV0 = true
		case "1":
			sawV1 = true
		}
	}
	if !sawLegacy || !sawV0 || !sawV1 {
		t.Fatalf("FX-01 version coverage missing: legacy=%v v0=%v v1=%v (RK-01)", sawLegacy, sawV0, sawV1)
	}

	sys := solana.SystemProgramID
	spl := solana.TokenProgramID
	t22 := solana.Token2022ProgramID
	inScope := map[string]bool{
		sys.String(): true,
		spl.String(): true,
		t22.String(): true,
	}
	hasInnerInScope := false
	for i, twm := range blk.Transactions {
		if twm.Meta == nil {
			continue
		}
		tx, err := twm.GetTransaction()
		if err != nil {
			t.Fatalf("tx[%d]: %v", i, err)
		}
		keys := accountKeys(tx, twm.Meta)
		for _, inner := range twm.Meta.InnerInstructions {
			for _, ix := range inner.Instructions {
				if int(ix.ProgramIDIndex) >= len(keys) {
					continue
				}
				prog := keys[ix.ProgramIDIndex].String()
				if inScope[prog] {
					hasInnerInScope = true
					break
				}
			}
			if hasInnerInScope {
				break
			}
		}
		if hasInnerInScope {
			break
		}
	}
	if !hasInnerInScope {
		t.Fatal("FX-01: no inner instruction with in-scope program (RK-01); inner transfer assert deferred to PLAN-006")
	}
}
