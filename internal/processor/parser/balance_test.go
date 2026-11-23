package parser

import (
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

func TestBalanceChangesP04(t *testing.T) {
	// P-04 / AC-027
	t.Parallel()
	blk := loadFixtureBlock(t)
	batch, err := ParseBlock(fixtureSlot, blk)
	if err != nil {
		t.Fatalf("ParseBlock: %v", err)
	}
	var got []domain.TokenBalanceChange
	for _, bc := range batch.BalanceChanges {
		if bc.TxIndex == 158 {
			got = append(got, bc)
		}
	}
	if len(got) != 2 {
		t.Fatalf("tx 158 balance changes: got %d want 2", len(got))
	}
	// Sorted by account_index: idx 2 (47AJo…), idx 3 (EnHd…) — wait, closed account only in pre.
	// pre has idx 2 and 3; post has only idx 2. Closed EnHd (idx 3): pre=98000000 post=0.
	// 47AJo (idx 2): pre=275870625812208 post=275870721852208.
	byAcct := map[string]domain.TokenBalanceChange{}
	for _, bc := range got {
		byAcct[bc.Account] = bc
	}
	src, ok := byAcct["EnHdUh7vi1Fo1ncfHB4HbMpmd4CW6fnVLv1fTgPEmczL"]
	if !ok {
		t.Fatal("missing EnHdUh7v… balance change")
	}
	if src.PreAmount != 98000000 || src.PostAmount != 0 {
		t.Fatalf("EnHd amounts: pre=%d post=%d", src.PreAmount, src.PostAmount)
	}
	dst, ok := byAcct["47AJoNqacLygyXKpTWZd7rzdcsuCcBx39Gdc6gn5FhND"]
	if !ok {
		t.Fatal("missing 47AJoNqa… balance change")
	}
	if dst.PreAmount != 275870625812208 || dst.PostAmount != 275870721852208 {
		t.Fatalf("47AJo amounts: pre=%d post=%d", dst.PreAmount, dst.PostAmount)
	}
	for _, bc := range got {
		if bc.Slot != fixtureSlot || bc.Program != domain.ProgramToken2022 {
			t.Fatalf("slot/program: %+v", bc)
		}
		if bc.Mint != "Ey59PH7Z4BFU4HjyKnyMdWt5GGN76KazTAwQihoUXRnk" || bc.Decimals != 9 {
			t.Fatalf("mint/decimals: %+v", bc)
		}
	}
	if src.Owner != "ETvhVmPeahyycn67xs3qaZK6PCD77V3UHY9ojEwQ3aAi" {
		t.Fatalf("src owner: %q", src.Owner)
	}
	if dst.Owner != "9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq" {
		t.Fatalf("dst owner: %q", dst.Owner)
	}
	// account_index ascending
	if got[0].AccountIndex > got[1].AccountIndex {
		t.Fatalf("not sorted by account_index: %d then %d", got[0].AccountIndex, got[1].AccountIndex)
	}
}

func TestBalanceChangeEdges(t *testing.T) {
	// BC-01..BC-04 / AC-033
	t.Parallel()
	mint := solana.MustPublicKeyFromBase58("Ey59PH7Z4BFU4HjyKnyMdWt5GGN76KazTAwQihoUXRnk")
	acctClosed := solana.MustPublicKeyFromBase58("EnHdUh7vi1Fo1ncfHB4HbMpmd4CW6fnVLv1fTgPEmczL")
	acctNew := solana.MustPublicKeyFromBase58("47AJoNqacLygyXKpTWZd7rzdcsuCcBx39Gdc6gn5FhND")
	acctSame := solana.MustPublicKeyFromBase58("9uAkfUJCYh1HyKaK19wce1PctPjXhRMCGwTTVeAuPTq")
	acctSPL := solana.MustPublicKeyFromBase58("ETvhVmPeahyycn67xs3qaZK6PCD77V3UHY9ojEwQ3aAi")
	owner := solana.MustPublicKeyFromBase58("ETvhVmPeahyycn67xs3qaZK6PCD77V3UHY9ojEwQ3aAi")
	keys := []solana.PublicKey{acctClosed, acctNew, acctSame, acctSPL}

	t.Run("BC01_pre_only", func(t *testing.T) {
		meta := &rpc.TransactionMeta{
			PreTokenBalances: []rpc.TokenBalance{
				tokenBal(0, mint, owner, solana.Token2022ProgramID, "50", 9),
			},
		}
		got := parseBalanceChanges(1, 0, keys, meta)
		if len(got) != 1 {
			t.Fatalf("len=%d want 1", len(got))
		}
		if got[0].PreAmount != 50 || got[0].PostAmount != 0 || got[0].Account != acctClosed.String() {
			t.Fatalf("got %+v", got[0])
		}
	})

	t.Run("BC02_post_only", func(t *testing.T) {
		meta := &rpc.TransactionMeta{
			PostTokenBalances: []rpc.TokenBalance{
				tokenBal(1, mint, owner, solana.Token2022ProgramID, "77", 9),
			},
		}
		got := parseBalanceChanges(1, 0, keys, meta)
		if len(got) != 1 {
			t.Fatalf("len=%d want 1", len(got))
		}
		if got[0].PreAmount != 0 || got[0].PostAmount != 77 || got[0].Account != acctNew.String() {
			t.Fatalf("got %+v", got[0])
		}
	})

	t.Run("BC03_pre_eq_post", func(t *testing.T) {
		meta := &rpc.TransactionMeta{
			PreTokenBalances: []rpc.TokenBalance{
				tokenBal(2, mint, owner, solana.Token2022ProgramID, "9", 9),
			},
			PostTokenBalances: []rpc.TokenBalance{
				tokenBal(2, mint, owner, solana.Token2022ProgramID, "9", 9),
			},
		}
		got := parseBalanceChanges(1, 0, keys, meta)
		if len(got) != 0 {
			t.Fatalf("pre=post must emit nothing, got %d", len(got))
		}
	})

	t.Run("BC04_spl_or_nil_program", func(t *testing.T) {
		meta := &rpc.TransactionMeta{
			PreTokenBalances: []rpc.TokenBalance{
				tokenBal(3, mint, owner, solana.TokenProgramID, "1", 9),
				{
					AccountIndex:  2,
					Mint:          mint,
					UiTokenAmount: &rpc.UiTokenAmount{Amount: "1", Decimals: 9},
					// ProgramId nil
				},
			},
			PostTokenBalances: []rpc.TokenBalance{
				tokenBal(3, mint, owner, solana.TokenProgramID, "2", 9),
				{
					AccountIndex:  2,
					Mint:          mint,
					UiTokenAmount: &rpc.UiTokenAmount{Amount: "2", Decimals: 9},
				},
			},
		}
		got := parseBalanceChanges(1, 0, keys, meta)
		if len(got) != 0 {
			t.Fatalf("SPL/nil programId must emit nothing, got %d", len(got))
		}
	})
}
