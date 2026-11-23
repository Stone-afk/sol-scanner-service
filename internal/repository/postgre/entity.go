package postgre

// Block maps sol_blocks.
type Block struct {
	Slot              uint64 `gorm:"column:slot;primaryKey"`
	Blockhash         string `gorm:"column:blockhash"`
	ParentSlot        uint64 `gorm:"column:parent_slot"`
	PreviousBlockhash string `gorm:"column:previous_blockhash"`
	BlockHeight       *int64 `gorm:"column:block_height"`
	BlockTime         *int64 `gorm:"column:block_time"`
	TransactionCount  int    `gorm:"column:transaction_count"`
	CreatedAt         int64  `gorm:"column:created_at;autoCreateTime:false"`
}

func (Block) TableName() string { return "sol_blocks" }

// Transaction maps sol_transactions.
type Transaction struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Slot      uint64 `gorm:"column:slot"`
	TxIndex   int    `gorm:"column:tx_index"`
	Signature string `gorm:"column:signature"`
	Status    int16  `gorm:"column:status"`
	Version   int16  `gorm:"column:version"`
	Fee       uint64 `gorm:"column:fee"`
	FeePayer  string `gorm:"column:fee_payer"`
}

func (Transaction) TableName() string { return "sol_transactions" }

// Transfer maps sol_transfers.
type Transfer struct {
	ID          int64   `gorm:"column:id;primaryKey;autoIncrement"`
	Slot        uint64  `gorm:"column:slot"`
	TxIndex     int     `gorm:"column:tx_index"`
	IxIndex     int16   `gorm:"column:ix_index"`
	InnerIndex  int16   `gorm:"column:inner_index"`
	Program     int16   `gorm:"column:program"`
	Kind        int16   `gorm:"column:kind"`
	Mint        string  `gorm:"column:mint"`
	Decimals    *int16  `gorm:"column:decimals"`
	FromAccount string  `gorm:"column:from_account"`
	FromOwner   string  `gorm:"column:from_owner"`
	ToAccount   string  `gorm:"column:to_account"`
	ToOwner     string  `gorm:"column:to_owner"`
	Amount      string  `gorm:"column:amount;type:numeric(20,0)"`
	Fee         *string `gorm:"column:fee;type:numeric(20,0)"`
}

func (Transfer) TableName() string { return "sol_transfers" }

// TokenBalanceChange maps sol_token_balance_changes.
type TokenBalanceChange struct {
	ID           int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Slot         uint64 `gorm:"column:slot"`
	TxIndex      int    `gorm:"column:tx_index"`
	AccountIndex int16  `gorm:"column:account_index"`
	Program      int16  `gorm:"column:program"`
	Account      string `gorm:"column:account"`
	Owner        string `gorm:"column:owner"`
	Mint         string `gorm:"column:mint"`
	Decimals     int16  `gorm:"column:decimals"`
	PreAmount    string `gorm:"column:pre_amount;type:numeric(20,0)"`
	PostAmount   string `gorm:"column:post_amount;type:numeric(20,0)"`
}

func (TokenBalanceChange) TableName() string { return "sol_token_balance_changes" }

// Checkpoint maps sol_scan_checkpoint.
type Checkpoint struct {
	ID            int16  `gorm:"column:id;primaryKey"`
	LastBlockSlot uint64 `gorm:"column:last_block_slot"`
	LastBlockhash string `gorm:"column:last_blockhash"`
	UpdatedAt     int64  `gorm:"column:updated_at"`
}

func (Checkpoint) TableName() string { return "sol_scan_checkpoint" }

// Halt maps sol_scan_halts.
type Halt struct {
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Reason     string `gorm:"column:reason"`
	Slot       uint64 `gorm:"column:slot"`
	Detail     string `gorm:"column:detail"`
	CreatedAt  int64  `gorm:"column:created_at"`
	ResolvedAt *int64 `gorm:"column:resolved_at"`
}

func (Halt) TableName() string { return "sol_scan_halts" }

// ScanCursor maps sol_scan_cursor.
type ScanCursor struct {
	ID         int16  `gorm:"column:id;primaryKey"`
	HandedSlot uint64 `gorm:"column:handed_slot"`
	UpdatedAt  int64  `gorm:"column:updated_at"`
}

func (ScanCursor) TableName() string { return "sol_scan_cursor" }
