package jsonparsed

import (
	"encoding/json"
	"testing"
)

func TestLoadParsedBlockFixture(t *testing.T) {
	// FV-02 — fixture yields 457 txs with non-empty AccountKeys
	t.Parallel()
	path := FixturePath(t, "block_471991643_jsonparsed.json.gz")
	parsed := LoadParsedBlock(t, path)
	if len(parsed.Transactions) != 457 {
		t.Fatalf("tx count: got %d want 457", len(parsed.Transactions))
	}
	for i, tx := range parsed.Transactions {
		if len(tx.AccountKeys) == 0 {
			t.Fatalf("tx[%d] AccountKeys empty", i)
		}
		if tx.Signature == "" {
			t.Fatalf("tx[%d] Signature empty", i)
		}
	}
	base64Path := FixturePath(t, "block_471991643_base64.json.gz")
	blk := LoadBase64Block(t, base64Path)
	if len(blk.Transactions) != 457 {
		t.Fatalf("base64 tx count: got %d want 457", len(blk.Transactions))
	}
}

func TestParseParsedBlockJSONFromFixtureBytes(t *testing.T) {
	t.Parallel()
	path := FixturePath(t, "block_471991643_jsonparsed.json.gz")
	raw := readGzipJSON(t, path)
	parsed, err := ParseParsedBlockJSON(raw)
	if err != nil {
		t.Fatalf("ParseParsedBlockJSON: %v", err)
	}
	if len(parsed.Transactions) != 457 {
		t.Fatalf("tx count: got %d want 457", len(parsed.Transactions))
	}
	tx0 := parsed.Transactions[0]
	if tx0.Signature == "" || tx0.Fee == 0 {
		t.Fatalf("tx[0] signature/fee empty: sig=%q fee=%d", tx0.Signature, tx0.Fee)
	}
	failed := parsed.Transactions[8]
	if !failed.Failed {
		t.Fatal("fixture tx[8] expected failed")
	}
	if n := len(Transfers(failed)); n != 0 {
		t.Fatalf("failed tx transfers=%d want 0", n)
	}
	var foundT22 bool
	for _, tx := range parsed.Transactions {
		if len(Token2022BalanceChanges(tx)) > 0 {
			foundT22 = true
			break
		}
	}
	if !foundT22 {
		t.Fatal("expected at least one Token-2022 balance change in fixture")
	}
}

func TestParseParsedBlockJSONBareResult(t *testing.T) {
	t.Parallel()
	path := FixturePath(t, "block_471991643_jsonparsed.json.gz")
	raw := readGzipJSON(t, path)
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	parsed, err := ParseParsedBlockJSON(envelope.Result)
	if err != nil {
		t.Fatalf("bare result: %v", err)
	}
	if len(parsed.Transactions) != 457 {
		t.Fatalf("tx count: got %d want 457", len(parsed.Transactions))
	}
}
