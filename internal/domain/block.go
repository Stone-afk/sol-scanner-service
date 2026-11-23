package domain

// TxStatus is the on-disk / domain encoding of transaction success or failure.
type TxStatus int16

const (
	TxStatusSuccess TxStatus = 1
	TxStatusFailed  TxStatus = 2
)

// ToInt16 returns the SMALLINT encoding of the status.
func (s TxStatus) ToInt16() int16 { return int16(s) }

// TxVersion is the transaction message version encoding.
type TxVersion int16

const (
	TxVersionLegacy TxVersion = -1
	TxVersion0      TxVersion = 0
	TxVersion1      TxVersion = 1
)

// ToInt16 returns the SMALLINT encoding of the version.
func (v TxVersion) ToInt16() int16 { return int16(v) }

// Block is a finalized Solana block header as stored by the scanner.
type Block struct {
	Slot              uint64
	Blockhash         string
	ParentSlot        uint64
	PreviousBlockhash string
	BlockHeight       uint64
	HasBlockHeight    bool
	BlockTime         int64
	HasBlockTime      bool
	TransactionCount  int
}

// Transaction is a single transaction row within a block.
type Transaction struct {
	Slot      uint64
	TxIndex   int
	Signature string
	Status    TxStatus
	Version   TxVersion
	Fee       uint64
	FeePayer  string
}

// BlockBatch is the pure-function output of parsing one block.
type BlockBatch struct {
	Block          Block
	Transactions   []Transaction
	Transfers      []Transfer
	BalanceChanges []TokenBalanceChange
}

// Checkpoint is the last committed scan position. Zero value means no checkpoint yet.
type Checkpoint struct {
	Slot      uint64
	Blockhash string
}
