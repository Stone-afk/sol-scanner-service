package parser

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/test/jsonparsed"
	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

func TestParseTransfersP03(t *testing.T) {
	// P-03 / AC-026 — fixture tx 158 has exactly one transfer (transferChecked).
	// Fixture fact: transferChecked is top-level ix_index=1 (createIdempotent is ix 0);
	// AC-026's ix_index=0 / message index "1" is falsified by the recorded block (ASM-01).
	t.Parallel()
	blk := loadFixtureBlock(t)
	batch, err := ParseBlock(fixtureSlot, blk)
	if err != nil {
		t.Fatalf("ParseBlock: %v", err)
	}
	var got []domain.Transfer
	for _, tr := range batch.Transfers {
		if tr.TxIndex == 158 {
			got = append(got, tr)
		}
	}
	if len(got) != 1 {
		t.Fatalf("tx 158 transfers: got %d want 1", len(got))
	}
	tr := got[0]
	if tr.Slot != fixtureSlot || tr.TxIndex != 158 {
		t.Fatalf("slot/tx: got slot=%d tx=%d", tr.Slot, tr.TxIndex)
	}
	if tr.IxIndex != 1 || tr.InnerIndex != domain.TopLevelInnerIndex {
		t.Fatalf("indexes: got ix=%d inner=%d want ix=1 inner=-1", tr.IxIndex, tr.InnerIndex)
	}
	if tr.Program != domain.ProgramToken2022 {
		t.Fatalf("Program: got %d want Token2022", tr.Program)
	}
	if tr.Kind != domain.KindTransferChecked {
		t.Fatalf("Kind: got %d want transferChecked", tr.Kind)
	}
	if tr.Mint != "Ey59PH7Z4BFU4HjyKnyMdWt5GGN76KazTAwQihoUXRnk" {
		t.Fatalf("Mint: got %q", tr.Mint)
	}
	if !tr.HasDecimals || tr.Decimals != 9 {
		t.Fatalf("Decimals: has=%v value=%d want 9", tr.HasDecimals, tr.Decimals)
	}
	if tr.FromAccount != "EnHdUh7vi1Fo1ncfHB4HbMpmd4CW6fnVLv1fTgPEmczL" {
		t.Fatalf("FromAccount: got %q", tr.FromAccount)
	}
	if tr.FromOwner != "ETvhVmPeahyycn67xs3qaZK6PCD77V3UHY9ojEwQ3aAi" {
		t.Fatalf("FromOwner: got %q", tr.FromOwner)
	}
	if tr.ToAccount != "47AJoNqacLygyXKpTWZd7rzdcsuCcBx39Gdc6gn5FhND" {
		t.Fatalf("ToAccount: got %q", tr.ToAccount)
	}
	if tr.ToOwner != "9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq" {
		t.Fatalf("ToOwner: got %q", tr.ToOwner)
	}
	if tr.Amount != 98000000 {
		t.Fatalf("Amount: got %d want 98000000", tr.Amount)
	}
	if tr.HasFee {
		t.Fatalf("HasFee: got true want false")
	}
}

func TestParseTransfersP05CloseAccountIgnored(t *testing.T) {
	// P-05 / AC-028 — closeAccount / harvestWithheldTokensToMint produce no transfers.
	t.Parallel()
	blk := loadFixtureBlock(t)
	batch, err := ParseBlock(fixtureSlot, blk)
	if err != nil {
		t.Fatalf("ParseBlock: %v", err)
	}
	var got []domain.Transfer
	for _, tr := range batch.Transfers {
		if tr.TxIndex == 158 {
			got = append(got, tr)
		}
	}
	if len(got) != 1 {
		t.Fatalf("tx 158 transfers: got %d want 1 (closeAccount/harvest must be ignored)", len(got))
	}
	if got[0].Kind != domain.KindTransferChecked {
		t.Fatalf("only transferChecked expected, got kind=%d", got[0].Kind)
	}
}

func TestParseTransfersP07MatchesJSONParsed(t *testing.T) {
	// P-07 / AC-029 — success-tx transfer set matches jsonParsed.
	t.Parallel()
	blk := loadFixtureBlock(t)
	parsed := loadFixtureParsed(t)
	batch, err := ParseBlock(fixtureSlot, blk)
	if err != nil {
		t.Fatalf("ParseBlock: %v", err)
	}

	byTx := map[int][]domain.Transfer{}
	for _, tr := range batch.Transfers {
		byTx[tr.TxIndex] = append(byTx[tr.TxIndex], tr)
	}
	for i, ptx := range parsed.Transactions {
		want := jsonparsed.Transfers(ptx)
		got := byTx[i]
		if ptx.Failed {
			if len(got) != 0 {
				t.Fatalf("tx[%d] failed but has %d transfers", i, len(got))
			}
			continue
		}
		if len(got) != len(want) {
			t.Fatalf("tx[%d] transfer count: got %d want %d", i, len(got), len(want))
		}
		for j := range want {
			g, w := got[j], want[j]
			idx := formatMessageIndex(g.IxIndex, g.InnerIndex)
			if idx != w.Index {
				t.Fatalf("tx[%d] tr[%d] Index: got %q want %q", i, j, idx, w.Index)
			}
			if programName(g.Program) != w.Program {
				t.Fatalf("tx[%d] tr[%d] Program: got %q want %q", i, j, programName(g.Program), w.Program)
			}
			if kindName(g.Kind) != w.Kind {
				t.Fatalf("tx[%d] tr[%d] Kind: got %q want %q", i, j, kindName(g.Kind), w.Kind)
			}
			if g.FromAccount != w.From {
				t.Fatalf("tx[%d] tr[%d] From: got %q want %q", i, j, g.FromAccount, w.From)
			}
			if g.ToAccount != w.To {
				t.Fatalf("tx[%d] tr[%d] To: got %q want %q", i, j, g.ToAccount, w.To)
			}
			if strconv.FormatUint(g.Amount, 10) != w.Amount {
				t.Fatalf("tx[%d] tr[%d] Amount: got %d want %s", i, j, g.Amount, w.Amount)
			}
		}
	}
}

func TestDecodeTransferCheckedWithFeeP09(t *testing.T) {
	// P-09 / AC-030
	t.Parallel()
	src := solana.MustPublicKeyFromBase58("EnHdUh7vi1Fo1ncfHB4HbMpmd4CW6fnVLv1fTgPEmczL")
	mint := solana.MustPublicKeyFromBase58("Ey59PH7Z4BFU4HjyKnyMdWt5GGN76KazTAwQihoUXRnk")
	dst := solana.MustPublicKeyFromBase58("47AJoNqacLygyXKpTWZd7rzdcsuCcBx39Gdc6gn5FhND")
	auth := solana.MustPublicKeyFromBase58("ETvhVmPeahyycn67xs3qaZK6PCD77V3UHY9ojEwQ3aAi")
	prog := solana.Token2022ProgramID

	data := make([]byte, 2+8+1+8)
	data[0], data[1] = 26, 1
	binary.LittleEndian.PutUint64(data[2:], 1000)
	data[10] = 9
	binary.LittleEndian.PutUint64(data[11:], 42)

	dec, ok, err := decodeInstruction(prog, []solana.PublicKey{src, mint, dst, auth}, data)
	if err != nil {
		t.Fatalf("decodeInstruction: %v", err)
	}
	if !ok {
		t.Fatal("expected in-scope transferCheckedWithFee")
	}
	if dec.kind != domain.KindTransferCheckedWithFee || dec.program != domain.ProgramToken2022 {
		t.Fatalf("kind/program: got kind=%d prog=%d", dec.kind, dec.program)
	}
	if !dec.hasFee || dec.fee != 42 {
		t.Fatalf("fee: has=%v value=%d want 42", dec.hasFee, dec.fee)
	}
	if dec.amount != 1000 || !dec.hasDecimals || dec.decimals != 9 {
		t.Fatalf("amount/decimals: amount=%d dec=%d has=%v", dec.amount, dec.decimals, dec.hasDecimals)
	}
	if dec.from != src.String() || dec.to != dst.String() || dec.mint != mint.String() {
		t.Fatalf("accounts: from=%s to=%s mint=%s", dec.from, dec.to, dec.mint)
	}
}

func TestDecodeTruncatedP10(t *testing.T) {
	// P-10 / AC-031 — truncated in-scope instruction data → ErrDecode
	t.Parallel()
	src := solana.MustPublicKeyFromBase58("EnHdUh7vi1Fo1ncfHB4HbMpmd4CW6fnVLv1fTgPEmczL")
	dst := solana.MustPublicKeyFromBase58("47AJoNqacLygyXKpTWZd7rzdcsuCcBx39Gdc6gn5FhND")
	keys := []solana.PublicKey{src, dst, solana.SystemProgramID}
	tx := &solana.Transaction{
		Signatures: make([]solana.Signature, 1),
		Message: solana.Message{
			AccountKeys: keys,
			Instructions: []solana.CompiledInstruction{{
				ProgramIDIndex: 2,
				Accounts:       []uint16{0, 1},
				Data:           solana.Base58{2, 0, 0}, // System transfer disc incomplete / truncated lamports
			}},
		},
	}
	meta := &rpc.TransactionMeta{}
	_, err := parseTransfers(fixtureSlot, 0, keys, tx, meta)
	if !errors.Is(err, ErrDecode) {
		t.Fatalf("truncated transfer: got err %v want ErrDecode", err)
	}
}

func TestTransferOwnerLookup(t *testing.T) {
	// TR-01..TR-03 / AC-032
	t.Parallel()
	t.Run("TR01_backfill", func(t *testing.T) {
		src := solana.MustPublicKeyFromBase58("EnHdUh7vi1Fo1ncfHB4HbMpmd4CW6fnVLv1fTgPEmczL")
		dst := solana.MustPublicKeyFromBase58("47AJoNqacLygyXKpTWZd7rzdcsuCcBx39Gdc6gn5FhND")
		mint := solana.MustPublicKeyFromBase58("Ey59PH7Z4BFU4HjyKnyMdWt5GGN76KazTAwQihoUXRnk")
		ownerFrom := solana.MustPublicKeyFromBase58("ETvhVmPeahyycn67xs3qaZK6PCD77V3UHY9ojEwQ3aAi")
		ownerTo := solana.MustPublicKeyFromBase58("9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq")
		keys := []solana.PublicKey{src, dst, mint, solana.TokenProgramID, ownerFrom, ownerTo}
		data := make([]byte, 1+8)
		data[0] = 3 // SPL transfer
		binary.LittleEndian.PutUint64(data[1:], 7)
		tx := &solana.Transaction{
			Signatures: make([]solana.Signature, 1),
			Message: solana.Message{
				AccountKeys: keys,
				Instructions: []solana.CompiledInstruction{{
					ProgramIDIndex: 3,
					Accounts:       []uint16{0, 1, 4},
					Data:           solana.Base58(data),
				}},
			},
		}
		meta := &rpc.TransactionMeta{
			PreTokenBalances: []rpc.TokenBalance{
				tokenBal(0, mint, ownerFrom, solana.TokenProgramID, "100", 6),
				tokenBal(1, mint, ownerTo, solana.TokenProgramID, "0", 6),
			},
		}
		trs, err := parseTransfers(1, 0, keys, tx, meta)
		if err != nil {
			t.Fatalf("parseTransfers: %v", err)
		}
		if len(trs) != 1 {
			t.Fatalf("len=%d", len(trs))
		}
		tr := trs[0]
		if tr.Mint != mint.String() || !tr.HasDecimals || tr.Decimals != 6 {
			t.Fatalf("mint/decimals backfill: mint=%q dec=%d has=%v", tr.Mint, tr.Decimals, tr.HasDecimals)
		}
		if tr.FromOwner != ownerFrom.String() || tr.ToOwner != ownerTo.String() {
			t.Fatalf("owners: from=%q to=%q", tr.FromOwner, tr.ToOwner)
		}
		if tr.Amount != 7 || tr.Program != domain.ProgramSPLToken || tr.Kind != domain.KindTransfer {
			t.Fatalf("amount/program/kind: %+v", tr)
		}
	})

	t.Run("TR02_missing_balance", func(t *testing.T) {
		src := solana.MustPublicKeyFromBase58("EnHdUh7vi1Fo1ncfHB4HbMpmd4CW6fnVLv1fTgPEmczL")
		dst := solana.MustPublicKeyFromBase58("47AJoNqacLygyXKpTWZd7rzdcsuCcBx39Gdc6gn5FhND")
		keys := []solana.PublicKey{src, dst, solana.TokenProgramID}
		data := make([]byte, 1+8)
		data[0] = 3
		binary.LittleEndian.PutUint64(data[1:], 1)
		tx := &solana.Transaction{
			Signatures: make([]solana.Signature, 1),
			Message: solana.Message{
				AccountKeys: keys,
				Instructions: []solana.CompiledInstruction{{
					ProgramIDIndex: 2,
					Accounts:       []uint16{0, 1},
					Data:           solana.Base58(data),
				}},
			},
		}
		trs, err := parseTransfers(1, 0, keys, tx, &rpc.TransactionMeta{})
		if err != nil {
			t.Fatalf("parseTransfers: %v", err)
		}
		if len(trs) != 1 {
			t.Fatalf("len=%d", len(trs))
		}
		tr := trs[0]
		if tr.Mint != "" || tr.FromOwner != "" || tr.ToOwner != "" || tr.HasDecimals {
			t.Fatalf("expected empty mint/owners and NULL decimals: %+v", tr)
		}
	})

	t.Run("TR03_system_transfer", func(t *testing.T) {
		from := solana.MustPublicKeyFromBase58("9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq")
		to := solana.MustPublicKeyFromBase58("ETvhVmPeahyycn67xs3qaZK6PCD77V3UHY9ojEwQ3aAi")
		keys := []solana.PublicKey{from, to, solana.SystemProgramID}
		data := make([]byte, 4+8)
		binary.LittleEndian.PutUint32(data[0:], 2)
		binary.LittleEndian.PutUint64(data[4:], 1)
		tx := &solana.Transaction{
			Signatures: make([]solana.Signature, 1),
			Message: solana.Message{
				AccountKeys: keys,
				Instructions: []solana.CompiledInstruction{{
					ProgramIDIndex: 2,
					Accounts:       []uint16{0, 1},
					Data:           solana.Base58(data),
				}},
			},
		}
		trs, err := parseTransfers(1, 0, keys, tx, &rpc.TransactionMeta{})
		if err != nil {
			t.Fatalf("parseTransfers: %v", err)
		}
		if len(trs) != 1 {
			t.Fatalf("len=%d", len(trs))
		}
		tr := trs[0]
		if tr.Program != domain.ProgramSystem || tr.Kind != domain.KindTransfer {
			t.Fatalf("program/kind: got %d/%d", tr.Program, tr.Kind)
		}
		if tr.Mint != "" || tr.HasDecimals || tr.FromOwner != "" || tr.ToOwner != "" {
			t.Fatalf("system transfer must leave mint/decimals/owners empty: %+v", tr)
		}
		if tr.Amount != 1 || tr.FromAccount != from.String() || tr.ToAccount != to.String() {
			t.Fatalf("amount/accounts: %+v", tr)
		}
	})
}

func tokenBal(idx uint16, mint, owner, program solana.PublicKey, amount string, decimals uint8) rpc.TokenBalance {
	o, p := owner, program
	return rpc.TokenBalance{
		AccountIndex: idx,
		Owner:        &o,
		ProgramId:    &p,
		Mint:         mint,
		UiTokenAmount: &rpc.UiTokenAmount{
			Amount:   amount,
			Decimals: decimals,
		},
	}
}

func formatMessageIndex(ixIndex, innerIndex int) string {
	if innerIndex < 0 {
		return strconv.Itoa(ixIndex + 1)
	}
	return fmt.Sprintf("%d.%d", ixIndex+1, innerIndex+1)
}

func programName(p domain.Program) string {
	switch p {
	case domain.ProgramSystem:
		return "system"
	case domain.ProgramSPLToken:
		return "spl-token"
	case domain.ProgramToken2022:
		return "token-2022"
	default:
		return fmt.Sprintf("program(%d)", p)
	}
}

func kindName(k domain.TransferKind) string {
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
		return fmt.Sprintf("kind(%d)", k)
	}
}
