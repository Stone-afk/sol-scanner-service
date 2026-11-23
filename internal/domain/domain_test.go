package domain

import "testing"

func TestDomainCodes(t *testing.T) {
	// DM-01 / AC-025 — design.md §4 encoding constants
	t.Parallel()

	tests := []struct {
		name string
		got  int16
		want int16
	}{
		{"TxStatusSuccess", TxStatusSuccess.ToInt16(), 1},
		{"TxStatusFailed", TxStatusFailed.ToInt16(), 2},
		{"TxVersionLegacy", TxVersionLegacy.ToInt16(), -1},
		{"TxVersion0", TxVersion0.ToInt16(), 0},
		{"TxVersion1", TxVersion1.ToInt16(), 1},
		{"ProgramSystem", ProgramSystem.ToInt16(), 1},
		{"ProgramSPLToken", ProgramSPLToken.ToInt16(), 2},
		{"ProgramToken2022", ProgramToken2022.ToInt16(), 3},
		{"KindTransfer", KindTransfer.ToInt16(), 1},
		{"KindTransferWithSeed", KindTransferWithSeed.ToInt16(), 2},
		{"KindCreateAccount", KindCreateAccount.ToInt16(), 3},
		{"KindCreateAccountWithSeed", KindCreateAccountWithSeed.ToInt16(), 4},
		{"KindWithdrawNonceAccount", KindWithdrawNonceAccount.ToInt16(), 5},
		{"KindTransferChecked", KindTransferChecked.ToInt16(), 6},
		{"KindTransferCheckedWithFee", KindTransferCheckedWithFee.ToInt16(), 7},
		{"KindMintTo", KindMintTo.ToInt16(), 8},
		{"KindMintToChecked", KindMintToChecked.ToInt16(), 9},
		{"KindBurn", KindBurn.ToInt16(), 10},
		{"KindBurnChecked", KindBurnChecked.ToInt16(), 11},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Fatalf("got %d want %d", tt.got, tt.want)
			}
		})
	}

	haltTests := []struct {
		name string
		got  string
		want string
	}{
		{"HASH_MISMATCH", HaltReasonHashMismatch.ToString(), "HASH_MISMATCH"},
		{"PARENT_BEHIND", HaltReasonParentBehind.ToString(), "PARENT_BEHIND"},
		{"GAP_UNRESOLVED", HaltReasonGapUnresolved.ToString(), "GAP_UNRESOLVED"},
		{"DECODE_ERROR", HaltReasonDecodeError.ToString(), "DECODE_ERROR"},
	}
	for _, tt := range haltTests {
		tt := tt
		t.Run("HaltReason_"+tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Fatalf("got %q want %q", tt.got, tt.want)
			}
		})
	}

	if TopLevelInnerIndex != -1 {
		t.Fatalf("TopLevelInnerIndex: got %d want -1", TopLevelInnerIndex)
	}
}
