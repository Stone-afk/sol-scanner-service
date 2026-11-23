package domain

// HaltReason identifies why the scanner stopped.
type HaltReason string

const (
	HaltReasonHashMismatch  HaltReason = "HASH_MISMATCH"
	HaltReasonParentBehind  HaltReason = "PARENT_BEHIND"
	HaltReasonGapUnresolved HaltReason = "GAP_UNRESOLVED"
	HaltReasonDecodeError   HaltReason = "DECODE_ERROR"
)

// ToString returns the VARCHAR encoding of the halt reason.
func (r HaltReason) ToString() string { return string(r) }

// Halt is an unresolved or historical scan-halt record.
type Halt struct {
	Reason    HaltReason
	Slot      uint64
	Detail    string
	CreatedAt int64
}
