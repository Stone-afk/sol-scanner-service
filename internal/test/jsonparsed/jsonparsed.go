package jsonparsed

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"

	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

// ParsedBlock is a lightly structured view of a jsonParsed getBlock result.
type ParsedBlock struct {
	Transactions []ParsedTx
}

// ParsedTx holds fields needed for P-07/P-08/A-02 cross-checks.
type ParsedTx struct {
	Signature   string
	Fee         uint64
	AccountKeys []string
	Failed      bool
	Version     string
	Raw         json.RawMessage
}

// FixturePath resolves <repo>/internal/processor/parser/testdata/<name>.
func FixturePath(t testing.TB, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/test/jsonparsed → repo root is two levels up from this package dir,
	// then internal/processor/parser/testdata.
	pkgDir := filepath.Dir(file)
	repoRoot := filepath.Clean(filepath.Join(pkgDir, "..", "..", ".."))
	return filepath.Join(repoRoot, "internal", "processor", "parser", "testdata", name)
}

// LoadBase64Block gunzips a JSON-RPC getBlock envelope and unmarshals result into GetBlockResult.
func LoadBase64Block(t testing.TB, path string) *rpc.GetBlockResult {
	t.Helper()
	raw := readGzipJSON(t, path)
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		t.Fatal("getBlock result is null")
	}
	var out rpc.GetBlockResult
	if err := json.Unmarshal(envelope.Result, &out); err != nil {
		t.Fatalf("unmarshal GetBlockResult: %v", err)
	}
	return &out
}

// LoadParsedBlock gunzips a jsonParsed getBlock envelope into ParsedBlock.
func LoadParsedBlock(t testing.TB, path string) ParsedBlock {
	t.Helper()
	raw := readGzipJSON(t, path)
	out, err := ParseParsedBlockJSON(raw)
	if err != nil {
		t.Fatalf("ParseParsedBlockJSON: %v", err)
	}
	return out
}

// ParseParsedBlockJSON parses a jsonParsed getBlock JSON-RPC envelope or bare result.
func ParseParsedBlockJSON(raw []byte) (ParsedBlock, error) {
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	result := raw
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Result) > 0 && string(envelope.Result) != "null" {
		result = envelope.Result
	}
	var body struct {
		Transactions []json.RawMessage `json:"transactions"`
	}
	if err := json.Unmarshal(result, &body); err != nil {
		return ParsedBlock{}, fmt.Errorf("unmarshal jsonParsed result: %w", err)
	}
	out := ParsedBlock{Transactions: make([]ParsedTx, 0, len(body.Transactions))}
	for i, twmRaw := range body.Transactions {
		tx, err := parseParsedTx(twmRaw)
		if err != nil {
			return ParsedBlock{}, fmt.Errorf("tx[%d]: %w", i, err)
		}
		out.Transactions = append(out.Transactions, tx)
	}
	return out, nil
}

func parseParsedTx(twmRaw json.RawMessage) (ParsedTx, error) {
	var twm struct {
		Meta *struct {
			Err any     `json:"err"`
			Fee *uint64 `json:"fee"`
		} `json:"meta"`
		Transaction struct {
			Signatures []string `json:"signatures"`
			Message    struct {
				AccountKeys []json.RawMessage `json:"accountKeys"`
			} `json:"message"`
		} `json:"transaction"`
		Version json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(twmRaw, &twm); err != nil {
		return ParsedTx{}, err
	}
	keys := make([]string, 0, len(twm.Transaction.Message.AccountKeys))
	for j, keyRaw := range twm.Transaction.Message.AccountKeys {
		var asString string
		if err := json.Unmarshal(keyRaw, &asString); err == nil {
			keys = append(keys, asString)
			continue
		}
		var asObj struct {
			Pubkey string `json:"pubkey"`
		}
		if err := json.Unmarshal(keyRaw, &asObj); err != nil || asObj.Pubkey == "" {
			return ParsedTx{}, fmt.Errorf("accountKeys[%d]: unsupported shape", j)
		}
		keys = append(keys, asObj.Pubkey)
	}
	version := "legacy"
	if len(twm.Version) > 0 && string(twm.Version) != "null" {
		var asString string
		if err := json.Unmarshal(twm.Version, &asString); err == nil {
			version = asString
		} else {
			var asInt int
			if err := json.Unmarshal(twm.Version, &asInt); err != nil {
				return ParsedTx{}, fmt.Errorf("version: %w", err)
			}
			version = strconv.Itoa(asInt)
		}
	}
	sig := ""
	if len(twm.Transaction.Signatures) > 0 {
		sig = twm.Transaction.Signatures[0]
	}
	var fee uint64
	if twm.Meta != nil && twm.Meta.Fee != nil {
		fee = *twm.Meta.Fee
	}
	failed := twm.Meta != nil && twm.Meta.Err != nil
	return ParsedTx{
		Signature:   sig,
		Fee:         fee,
		AccountKeys: keys,
		Failed:      failed,
		Version:     version,
		Raw:         twmRaw,
	}, nil
}

// NormalizedTransfer is a jsonParsed transfer row for P-07 cross-checks.
type NormalizedTransfer struct {
	Index   string
	Program string
	Kind    string
	From    string
	To      string
	Amount  string
}

var (
	token2022ProgramID   = solana.Token2022ProgramID.String()
	inScopeTransferKinds = map[string]bool{
		"transfer":               true,
		"transferWithSeed":       true,
		"createAccount":          true,
		"createAccountWithSeed":  true,
		"withdrawFromNonce":      true,
		"withdrawNonceAccount":   true,
		"transferChecked":        true,
		"mintTo":                 true,
		"mintToChecked":          true,
		"burn":                   true,
		"burnChecked":            true,
		"transferCheckedWithFee": true,
	}
)

// Transfers extracts in-scope transfers from a jsonParsed transaction (success only).
func Transfers(tx ParsedTx) []NormalizedTransfer {
	if tx.Failed {
		return nil
	}
	var twm struct {
		Transaction struct {
			Message struct {
				Instructions []json.RawMessage `json:"instructions"`
			} `json:"message"`
		} `json:"transaction"`
		Meta *struct {
			InnerInstructions []struct {
				Index        uint16            `json:"index"`
				Instructions []json.RawMessage `json:"instructions"`
			} `json:"innerInstructions"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(tx.Raw, &twm); err != nil {
		return nil
	}
	innersByTop := map[uint16][]json.RawMessage{}
	if twm.Meta != nil {
		for _, inn := range twm.Meta.InnerInstructions {
			innersByTop[inn.Index] = append(innersByTop[inn.Index], inn.Instructions...)
		}
	}
	out := make([]NormalizedTransfer, 0)
	for i, ixRaw := range twm.Transaction.Message.Instructions {
		if nt, ok := normalizedFromIX(ixRaw, i, -1); ok {
			out = append(out, nt)
		}
		for j, innerRaw := range innersByTop[uint16(i)] {
			if nt, ok := normalizedFromIX(innerRaw, i, j); ok {
				out = append(out, nt)
			}
		}
	}
	return out
}

func normalizedFromIX(raw json.RawMessage, ixIndex, innerIndex int) (NormalizedTransfer, bool) {
	var ix struct {
		Program   string `json:"program"`
		ProgramID string `json:"programId"`
		Parsed    *struct {
			Type string          `json:"type"`
			Info json.RawMessage `json:"info"`
		} `json:"parsed"`
	}
	if err := json.Unmarshal(raw, &ix); err != nil || ix.Parsed == nil {
		return NormalizedTransfer{}, false
	}
	kind := ix.Parsed.Type
	if !inScopeTransferKinds[kind] {
		return NormalizedTransfer{}, false
	}
	program, ok := normalizeProgram(ix.Program, ix.ProgramID)
	if !ok {
		return NormalizedTransfer{}, false
	}
	if kind == "withdrawFromNonce" {
		kind = "withdrawNonceAccount"
	}
	from, to, amount := extractTransferFields(kind, ix.Parsed.Info)
	return NormalizedTransfer{
		Index:   formatTransferIndex(ixIndex, innerIndex),
		Program: program,
		Kind:    kind,
		From:    from,
		To:      to,
		Amount:  amount,
	}, true
}

func normalizeProgram(program, programID string) (string, bool) {
	switch program {
	case "system":
		return "system", true
	case "spl-token-2022":
		return "token-2022", true
	case "spl-token":
		// Fixture fact (FV-02): Token-2022 often appears as program=spl-token + programId=Tokenz…
		if programID == token2022ProgramID {
			return "token-2022", true
		}
		return "spl-token", true
	default:
		return "", false
	}
}

func extractTransferFields(kind string, infoRaw json.RawMessage) (from, to, amount string) {
	var info map[string]json.RawMessage
	if err := json.Unmarshal(infoRaw, &info); err != nil {
		return "", "", ""
	}
	getStr := func(keys ...string) string {
		for _, k := range keys {
			raw, ok := info[k]
			if !ok {
				continue
			}
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				return s
			}
		}
		return ""
	}
	amount = getStr("lamports", "amount")
	if amount == "" {
		if ta, ok := info["tokenAmount"]; ok {
			var tokenAmount struct {
				Amount string `json:"amount"`
			}
			if err := json.Unmarshal(ta, &tokenAmount); err == nil {
				amount = tokenAmount.Amount
			}
		}
	}
	if amount == "" {
		// lamports may be a JSON number
		if raw, ok := info["lamports"]; ok {
			var n json.Number
			if err := json.Unmarshal(raw, &n); err == nil {
				amount = n.String()
			} else {
				var u uint64
				if err := json.Unmarshal(raw, &u); err == nil {
					amount = strconv.FormatUint(u, 10)
				}
			}
		}
	}
	if amount == "" {
		if raw, ok := info["amount"]; ok {
			var n json.Number
			if err := json.Unmarshal(raw, &n); err == nil {
				amount = n.String()
			}
		}
	}

	switch kind {
	case "transfer", "transferWithSeed", "transferChecked", "transferCheckedWithFee":
		from, to = getStr("source"), getStr("destination")
	case "createAccount", "createAccountWithSeed":
		from, to = getStr("source"), getStr("newAccount")
	case "withdrawNonceAccount":
		from, to = getStr("nonceAccount"), getStr("destination")
	case "mintTo", "mintToChecked":
		to = getStr("account")
	case "burn", "burnChecked":
		from = getStr("account")
	}
	_ = getStr("mint") // recognized field; mint not part of NormalizedTransfer
	return from, to, amount
}

func formatTransferIndex(ixIndex, innerIndex int) string {
	if innerIndex < 0 {
		return strconv.Itoa(ixIndex + 1)
	}
	return fmt.Sprintf("%d.%d", ixIndex+1, innerIndex+1)
}

// TokenBalanceDelta is a Token-2022 pre/post amount change from jsonParsed meta.
type TokenBalanceDelta struct {
	AccountIndex int
	Account      string
	Owner        string
	Mint         string
	Decimals     uint8
	PreAmount    string
	PostAmount   string
}

// Token2022BalanceChanges extracts Token-2022 account deltas (pre≠post) from meta.
func Token2022BalanceChanges(tx ParsedTx) []TokenBalanceDelta {
	var twm struct {
		Meta *struct {
			PreTokenBalances  []jsonTokenBalance `json:"preTokenBalances"`
			PostTokenBalances []jsonTokenBalance `json:"postTokenBalances"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(tx.Raw, &twm); err != nil || twm.Meta == nil {
		return nil
	}
	type merged struct {
		accountIndex int
		programOK    bool
		owner        string
		mint         string
		decimals     uint8
		preAmount    string
		postAmount   string
		hasPre       bool
		hasPost      bool
	}
	byIdx := map[int]*merged{}
	apply := func(b jsonTokenBalance, isPre bool) {
		m, ok := byIdx[b.AccountIndex]
		if !ok {
			m = &merged{accountIndex: b.AccountIndex}
			byIdx[b.AccountIndex] = m
		}
		if b.ProgramID == token2022ProgramID {
			m.programOK = true
		}
		if b.Owner != "" {
			if isPre {
				if m.owner == "" {
					m.owner = b.Owner
				}
			} else {
				m.owner = b.Owner
			}
		}
		if m.mint == "" {
			m.mint = b.Mint
		}
		if b.UiTokenAmount != nil {
			m.decimals = b.UiTokenAmount.Decimals
			amt := b.UiTokenAmount.Amount
			if isPre {
				m.preAmount = amt
				m.hasPre = true
			} else {
				m.postAmount = amt
				m.hasPost = true
			}
		}
	}
	for _, b := range twm.Meta.PreTokenBalances {
		apply(b, true)
	}
	for _, b := range twm.Meta.PostTokenBalances {
		apply(b, false)
	}
	out := make([]TokenBalanceDelta, 0)
	for _, m := range byIdx {
		if !m.programOK {
			continue
		}
		pre, post := m.preAmount, m.postAmount
		if !m.hasPre {
			pre = "0"
		}
		if !m.hasPost {
			post = "0"
		}
		if pre == post {
			continue
		}
		acct := ""
		if m.accountIndex >= 0 && m.accountIndex < len(tx.AccountKeys) {
			acct = tx.AccountKeys[m.accountIndex]
		}
		out = append(out, TokenBalanceDelta{
			AccountIndex: m.accountIndex,
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
	return out
}

type jsonTokenBalance struct {
	AccountIndex  int    `json:"accountIndex"`
	Mint          string `json:"mint"`
	Owner         string `json:"owner"`
	ProgramID     string `json:"programId"`
	UiTokenAmount *struct {
		Amount   string `json:"amount"`
		Decimals uint8  `json:"decimals"`
	} `json:"uiTokenAmount"`
}

func readGzipJSON(t testing.TB, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", filepath.Base(path), err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip %s: %v", filepath.Base(path), err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	return raw
}
