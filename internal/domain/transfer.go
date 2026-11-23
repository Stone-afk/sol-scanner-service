package domain

// Program encodes which on-chain program produced a transfer row.
type Program int16

const (
	ProgramSystem    Program = 1
	ProgramSPLToken  Program = 2
	ProgramToken2022 Program = 3
)

// ToInt16 returns the SMALLINT encoding of the program.
func (p Program) ToInt16() int16 { return int16(p) }

// TransferKind encodes the in-scope instruction type.
type TransferKind int16

const (
	KindTransfer               TransferKind = 1
	KindTransferWithSeed       TransferKind = 2
	KindCreateAccount          TransferKind = 3
	KindCreateAccountWithSeed  TransferKind = 4
	KindWithdrawNonceAccount   TransferKind = 5
	KindTransferChecked        TransferKind = 6
	KindTransferCheckedWithFee TransferKind = 7
	KindMintTo                 TransferKind = 8
	KindMintToChecked          TransferKind = 9
	KindBurn                   TransferKind = 10
	KindBurnChecked            TransferKind = 11
)

// ToInt16 returns the SMALLINT encoding of the kind.
func (k TransferKind) ToInt16() int16 { return int16(k) }

// TopLevelInnerIndex marks a top-level instruction (not an inner instruction).
const TopLevelInnerIndex = -1

// Transfer is one in-scope instruction extracted from a successful transaction.
type Transfer struct {
	Slot        uint64
	TxIndex     int
	IxIndex     int
	InnerIndex  int
	Program     Program
	Kind        TransferKind
	Mint        string
	Decimals    uint8
	HasDecimals bool
	FromAccount string
	FromOwner   string
	ToAccount   string
	ToOwner     string
	Amount      uint64
	Fee         uint64
	HasFee      bool
}

// TokenBalanceChange is a Token-2022 account pre/post balance delta.
type TokenBalanceChange struct {
	Slot         uint64
	TxIndex      int
	AccountIndex int
	Program      Program
	Account      string
	Owner        string
	Mint         string
	Decimals     uint8
	PreAmount    uint64
	PostAmount   uint64
}
