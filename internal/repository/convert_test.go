package repository

import (
	"math"
	"reflect"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/repository/postgre"
)

func TestEntityRoundTrip(t *testing.T) {
	t.Parallel()

	const maxUint64 = uint64(math.MaxUint64) // 18446744073709551615

	tests := []struct {
		name  string
		batch domain.BlockBatch
		check func(t *testing.T, batch postgre.ScanBatch)
	}{
		{
			name: "uint64_max_amounts_and_optional_nulls",
			batch: domain.BlockBatch{
				Block: domain.Block{
					Slot:              42,
					Blockhash:         "BlockHash42xxxxxxxxxxxxxxxxxxxxxxxxxxx",
					ParentSlot:        41,
					PreviousBlockhash: "PrevHash41xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
					HasBlockHeight:    false,
					HasBlockTime:      false,
					TransactionCount:  1,
				},
				Transactions: []domain.Transaction{
					{
						Slot:      42,
						TxIndex:   0,
						Signature: "SigMaxAmountxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Status:    domain.TxStatusSuccess,
						Version:   domain.TxVersionLegacy,
						Fee:       5000,
						FeePayer:  "FeePayerxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
					},
				},
				Transfers: []domain.Transfer{
					{
						Slot:        42,
						TxIndex:     0,
						IxIndex:     0,
						InnerIndex:  domain.TopLevelInnerIndex,
						Program:     domain.ProgramSystem,
						Kind:        domain.KindTransfer,
						Mint:        "",
						HasDecimals: false,
						FromAccount: "FromAccxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						FromOwner:   "FromOwnerxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						ToAccount:   "ToAccxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						ToOwner:     "ToOwnerxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Amount:      maxUint64,
						HasFee:      false,
					},
				},
				BalanceChanges: []domain.TokenBalanceChange{
					{
						Slot:         42,
						TxIndex:      0,
						AccountIndex: 0,
						Program:      domain.ProgramToken2022,
						Account:      "TokenAccxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Owner:        "TokenOwnerxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Mint:         "Mintxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Decimals:     0,
						PreAmount:    maxUint64,
						PostAmount:   0,
					},
				},
			},
			check: func(t *testing.T, batch postgre.ScanBatch) {
				t.Helper()
				if batch.Block.BlockHeight != nil {
					t.Fatalf("BlockHeight: want nil, got %#v", batch.Block.BlockHeight)
				}
				if batch.Block.BlockTime != nil {
					t.Fatalf("BlockTime: want nil, got %#v", batch.Block.BlockTime)
				}
				if len(batch.Transfers) != 1 {
					t.Fatalf("transfers len=%d", len(batch.Transfers))
				}
				if batch.Transfers[0].Amount != "18446744073709551615" {
					t.Fatalf("Amount: got %q", batch.Transfers[0].Amount)
				}
				if batch.Transfers[0].Fee != nil {
					t.Fatalf("Fee: want nil, got %#v", batch.Transfers[0].Fee)
				}
				if batch.Transfers[0].Decimals != nil {
					t.Fatalf("Decimals: want nil, got %#v", batch.Transfers[0].Decimals)
				}
				if batch.Transfers[0].InnerIndex != -1 {
					t.Fatalf("InnerIndex: got %d want -1", batch.Transfers[0].InnerIndex)
				}
				if len(batch.BalanceChanges) != 1 {
					t.Fatalf("balances len=%d", len(batch.BalanceChanges))
				}
				if batch.BalanceChanges[0].PreAmount != "18446744073709551615" {
					t.Fatalf("PreAmount: got %q", batch.BalanceChanges[0].PreAmount)
				}
				if batch.BalanceChanges[0].PostAmount != "0" {
					t.Fatalf("PostAmount: got %q", batch.BalanceChanges[0].PostAmount)
				}
			},
		},
		{
			name: "has_flags_present_with_zero_values",
			batch: domain.BlockBatch{
				Block: domain.Block{
					Slot:              100,
					Blockhash:         "BlockHash100xxxxxxxxxxxxxxxxxxxxxxxxxx",
					ParentSlot:        99,
					PreviousBlockhash: "PrevHash99xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
					BlockHeight:       0,
					HasBlockHeight:    true,
					BlockTime:         0,
					HasBlockTime:      true,
					TransactionCount:  1,
				},
				Transactions: []domain.Transaction{
					{
						Slot:      100,
						TxIndex:   0,
						Signature: "SigZeroFlagsxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Status:    domain.TxStatusFailed,
						Version:   domain.TxVersion0,
						Fee:       0,
						FeePayer:  "FeePayer0xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
					},
				},
				Transfers: []domain.Transfer{
					{
						Slot:        100,
						TxIndex:     0,
						IxIndex:     1,
						InnerIndex:  0,
						Program:     domain.ProgramToken2022,
						Kind:        domain.KindTransferCheckedWithFee,
						Mint:        "Mint0xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Decimals:    0,
						HasDecimals: true,
						FromAccount: "From0xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						FromOwner:   "FromOwner0xxxxxxxxxxxxxxxxxxxxxxxxxx",
						ToAccount:   "To0xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						ToOwner:     "ToOwner0xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Amount:      0,
						Fee:         0,
						HasFee:      true,
					},
				},
				BalanceChanges: nil,
			},
			check: func(t *testing.T, batch postgre.ScanBatch) {
				t.Helper()
				if batch.Block.BlockHeight == nil || *batch.Block.BlockHeight != 0 {
					t.Fatalf("BlockHeight: want &0, got %#v", batch.Block.BlockHeight)
				}
				if batch.Block.BlockTime == nil || *batch.Block.BlockTime != 0 {
					t.Fatalf("BlockTime: want &0, got %#v", batch.Block.BlockTime)
				}
				if len(batch.Transfers) != 1 {
					t.Fatalf("transfers len=%d", len(batch.Transfers))
				}
				if batch.Transfers[0].Decimals == nil || *batch.Transfers[0].Decimals != 0 {
					t.Fatalf("Decimals: want &0, got %#v", batch.Transfers[0].Decimals)
				}
				if batch.Transfers[0].Fee == nil || *batch.Transfers[0].Fee != "0" {
					t.Fatalf("Fee: want &\"0\", got %#v", batch.Transfers[0].Fee)
				}
				if batch.Transfers[0].InnerIndex != 0 {
					t.Fatalf("InnerIndex: got %d want 0", batch.Transfers[0].InnerIndex)
				}
			},
		},
		{
			name: "fee_five_and_decimals_nonzero",
			batch: domain.BlockBatch{
				Block: domain.Block{
					Slot:              7,
					Blockhash:         "BlockHash7xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
					ParentSlot:        6,
					PreviousBlockhash: "PrevHash6xxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
					BlockHeight:       12345,
					HasBlockHeight:    true,
					BlockTime:         1_700_000_000,
					HasBlockTime:      true,
					TransactionCount:  0,
				},
				Transfers: []domain.Transfer{
					{
						Slot:        7,
						TxIndex:     2,
						IxIndex:     3,
						InnerIndex:  domain.TopLevelInnerIndex,
						Program:     domain.ProgramSPLToken,
						Kind:        domain.KindTransferCheckedWithFee,
						Mint:        "UsdcMintxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Decimals:    6,
						HasDecimals: true,
						FromAccount: "From7xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						FromOwner:   "FromOwner7xxxxxxxxxxxxxxxxxxxxxxxxxx",
						ToAccount:   "To7xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						ToOwner:     "ToOwner7xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
						Amount:      5,
						Fee:         5,
						HasFee:      true,
					},
				},
			},
			check: func(t *testing.T, batch postgre.ScanBatch) {
				t.Helper()
				if batch.Block.BlockHeight == nil || *batch.Block.BlockHeight != 12345 {
					t.Fatalf("BlockHeight: got %#v", batch.Block.BlockHeight)
				}
				if batch.Block.BlockTime == nil || *batch.Block.BlockTime != 1_700_000_000 {
					t.Fatalf("BlockTime: got %#v", batch.Block.BlockTime)
				}
				if len(batch.Transfers) != 1 {
					t.Fatalf("transfers len=%d", len(batch.Transfers))
				}
				if batch.Transfers[0].Fee == nil || *batch.Transfers[0].Fee != "5" {
					t.Fatalf("Fee: want &\"5\", got %#v", batch.Transfers[0].Fee)
				}
				if batch.Transfers[0].Decimals == nil || *batch.Transfers[0].Decimals != 6 {
					t.Fatalf("Decimals: want &6, got %#v", batch.Transfers[0].Decimals)
				}
				if batch.Transfers[0].InnerIndex != -1 {
					t.Fatalf("InnerIndex: got %d want -1", batch.Transfers[0].InnerIndex)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entities := toEntities(tt.batch)
			if tt.check != nil {
				tt.check(t, entities)
			}

			got := fromEntities(entities)
			if !reflect.DeepEqual(got, tt.batch) {
				t.Fatalf("round-trip mismatch\ngot:  %#v\nwant: %#v", got, tt.batch)
			}
		})
	}
}
