package block

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/processor/parser"
	"github.com/Stone-afk/sol-scanner-service/internal/test/jsonparsed"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

const (
	testChain      = "solana-mainnet"
	fixtureSlot    = uint64(471991643)
	skippedSlot    = uint64(102)
	fixtureTxIndex = 158
)

func TestBlockMessageKeySkipped102(t *testing.T) {
	// B-01 / AC-083
	t.Parallel()
	if got := Key(skippedSlot); got != "102" {
		t.Fatalf("Key(102) = %q, want %q", got, "102")
	}
}

func TestBlockMessageSkippedOmitsBlock(t *testing.T) {
	// B-01 / AC-083
	t.Parallel()
	msg := BlockMessage{
		Version:    1,
		Chain:      testChain,
		Slot:       skippedSlot,
		Commitment: "finalized",
		Kind:       "skipped",
	}
	raw, err := Encode(msg)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal encoded skipped: %v", err)
	}
	if _, ok := obj["block"]; ok {
		t.Fatalf("skipped JSON must omit block; got keys %v", keysOf(obj))
	}
	got, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Kind != "skipped" || got.Slot != skippedSlot || len(got.Block) != 0 {
		t.Fatalf("Decode skipped = %+v", got)
	}
}

func TestBlockMessageBlockIsResultObjectNotBase64(t *testing.T) {
	// AC-083 / FR-11: block is getBlock result JSON object, not base64 string
	t.Parallel()
	blk := loadFixtureBlock(t)
	blockJSON, err := json.Marshal(blk)
	if err != nil {
		t.Fatalf("marshal GetBlockResult: %v", err)
	}
	msg := BlockMessage{
		Version:    1,
		Chain:      testChain,
		Slot:       fixtureSlot,
		Commitment: "finalized",
		Kind:       "block",
		Block:      blockJSON,
	}
	raw, err := Encode(msg)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal encoded block msg: %v", err)
	}
	blockField, ok := obj["block"]
	if !ok {
		t.Fatal("kind=block JSON must include block")
	}
	trim := bytes.TrimSpace(blockField)
	if len(trim) == 0 || trim[0] != '{' {
		t.Fatalf("block must be a JSON object, got %s", preview(trim, 64))
	}
	var asString string
	if err := json.Unmarshal(blockField, &asString); err == nil {
		t.Fatalf("block must not be a JSON string (would be base64); got prefix %q", preview([]byte(asString), 32))
	}
}

func TestBlockMessageFixtureRoundTripParseBlock(t *testing.T) {
	// B-02 / AC-083 / FV-04
	t.Parallel()
	orig := loadFixtureBlock(t)

	direct, err := parser.ParseBlock(fixtureSlot, orig)
	if err != nil {
		t.Fatalf("ParseBlock(direct): %v", err)
	}

	// FV-04: GetBlockResult via encoding/json round trip
	marshaled, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal GetBlockResult: %v", err)
	}
	var roundTrip rpc.GetBlockResult
	if err := json.Unmarshal(marshaled, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal GetBlockResult: %v", err)
	}
	viaJSON, err := parser.ParseBlock(fixtureSlot, &roundTrip)
	if err != nil {
		t.Fatalf("ParseBlock(json round trip): %v", err)
	}
	if !blockBatchesEqual(direct, viaJSON) {
		t.Fatalf("FV-04 FAIL: ParseBlock after encoding/json round trip differs from direct\n%s",
			batchDiff(direct, viaJSON))
	}

	// BlockMessage Encode/Decode must preserve the same ParseBlock result
	msg := BlockMessage{
		Version:    1,
		Chain:      testChain,
		Slot:       fixtureSlot,
		Commitment: "finalized",
		Kind:       "block",
		Block:      marshaled,
	}
	encoded, err := Encode(msg)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if decoded.Version != 1 || decoded.Commitment != "finalized" || decoded.Kind != "block" {
		t.Fatalf("decoded meta = version=%d commitment=%q kind=%q", decoded.Version, decoded.Commitment, decoded.Kind)
	}
	if decoded.Slot != fixtureSlot {
		t.Fatalf("decoded Slot = %d, want %d", decoded.Slot, fixtureSlot)
	}
	var fromMsg rpc.GetBlockResult
	if err := json.Unmarshal(decoded.Block, &fromMsg); err != nil {
		t.Fatalf("unmarshal decoded.Block: %v", err)
	}
	viaMsg, err := parser.ParseBlock(fixtureSlot, &fromMsg)
	if err != nil {
		t.Fatalf("ParseBlock(from BlockMessage): %v", err)
	}
	if !blockBatchesEqual(direct, viaMsg) {
		t.Fatalf("ParseBlock via BlockMessage differs from direct\n%s", batchDiff(direct, viaMsg))
	}
}

func TestBlockMessageRejectsInvalidKind(t *testing.T) {
	t.Parallel()
	_, err := Encode(BlockMessage{
		Version:    1,
		Chain:      testChain,
		Slot:       1,
		Commitment: "finalized",
		Kind:       "unknown",
	})
	if err == nil {
		t.Fatal("Encode(kind=unknown): want error")
	}
}

func loadFixtureBlock(t *testing.T) *rpc.GetBlockResult {
	t.Helper()
	return jsonparsed.LoadBase64Block(t, jsonparsed.FixturePath(t, "block_471991643_base64.json.gz"))
}

func blockBatchesEqual(a, b domain.BlockBatch) bool {
	return reflect.DeepEqual(a, b)
}

func batchDiff(a, b domain.BlockBatch) string {
	var buf bytes.Buffer
	if !reflect.DeepEqual(a.Block, b.Block) {
		buf.WriteString("Block header differs\n")
	}
	if len(a.Transactions) != len(b.Transactions) {
		buf.WriteString("TransactionCount differs\n")
	}
	a158 := transfersAt(a.Transfers, fixtureTxIndex)
	b158 := transfersAt(b.Transfers, fixtureTxIndex)
	if !reflect.DeepEqual(a158, b158) {
		buf.WriteString("transfers at tx_index 158 differ\n")
	}
	if buf.Len() == 0 {
		buf.WriteString("DeepEqual failed (see field counts)\n")
	}
	buf.WriteString("direct: ")
	buf.WriteString(batchSummary(a))
	buf.WriteString("\nvia: ")
	buf.WriteString(batchSummary(b))
	return buf.String()
}

func batchSummary(b domain.BlockBatch) string {
	return fmt.Sprintf("tx=%d transfers=%d bal=%d hash=%s",
		len(b.Transactions), len(b.Transfers), len(b.BalanceChanges), b.Block.Blockhash)
}

func transfersAt(trs []domain.Transfer, txIndex int) []domain.Transfer {
	out := make([]domain.Transfer, 0)
	for _, tr := range trs {
		if tr.TxIndex == txIndex {
			out = append(out, tr)
		}
	}
	return out
}

func keysOf(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func preview(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
