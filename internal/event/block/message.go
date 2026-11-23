package block

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const (
	MessageVersion      = 1
	CommitmentFinalized = "finalized"
	KindBlock           = "block"
	KindSkipped         = "skipped"
)

// BlockMessage is one slot on the block queue (design §5.3).
// Value semantics. Block holds getBlock result JSON when Kind=block.
type BlockMessage struct {
	Version    int    `json:"version"`
	Chain      string `json:"chain"`
	Slot       uint64 `json:"slot"`
	Commitment string `json:"commitment"`
	Kind       string `json:"kind"` // block / skipped
	Block      []byte `json:"block,omitempty"`
}

// Key returns the Kafka message key for a slot: decimal without leading zeros.
func Key(slot uint64) string {
	return strconv.FormatUint(slot, 10)
}

// Encode validates and JSON-encodes a BlockMessage.
// kind=block embeds Block as a JSON object (not base64); kind=skipped omits block.
func Encode(msg BlockMessage) ([]byte, error) {
	if err := validate(msg); err != nil {
		return nil, err
	}
	return json.Marshal(msg)
}

// Decode JSON-decodes and validates a BlockMessage.
func Decode(data []byte) (BlockMessage, error) {
	var msg BlockMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return BlockMessage{}, fmt.Errorf("decode block message: %w", err)
	}
	if err := validate(msg); err != nil {
		return BlockMessage{}, err
	}
	return msg, nil
}

type blockMessageWire struct {
	Version    int             `json:"version"`
	Chain      string          `json:"chain"`
	Slot       uint64          `json:"slot"`
	Commitment string          `json:"commitment"`
	Kind       string          `json:"kind"`
	Block      json.RawMessage `json:"block,omitempty"`
}

// MarshalJSON embeds Block as a raw JSON object, not a base64 string.
func (m BlockMessage) MarshalJSON() ([]byte, error) {
	w := blockMessageWire{
		Version:    m.Version,
		Chain:      m.Chain,
		Slot:       m.Slot,
		Commitment: m.Commitment,
		Kind:       m.Kind,
	}
	if m.Kind == KindBlock && len(m.Block) > 0 {
		if !json.Valid(m.Block) {
			return nil, fmt.Errorf("block field is not valid JSON")
		}
		w.Block = json.RawMessage(m.Block)
	}
	return json.Marshal(w)
}

// UnmarshalJSON reads Block as a raw JSON object into []byte.
func (m *BlockMessage) UnmarshalJSON(data []byte) error {
	var w blockMessageWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	m.Version = w.Version
	m.Chain = w.Chain
	m.Slot = w.Slot
	m.Commitment = w.Commitment
	m.Kind = w.Kind
	if len(w.Block) == 0 || string(w.Block) == "null" {
		m.Block = nil
	} else {
		m.Block = append([]byte(nil), w.Block...)
	}
	return nil
}

func validate(msg BlockMessage) error {
	if msg.Version != MessageVersion {
		return fmt.Errorf("block message version %d, want %d", msg.Version, MessageVersion)
	}
	if msg.Commitment != CommitmentFinalized {
		return fmt.Errorf("block message commitment %q, want %q", msg.Commitment, CommitmentFinalized)
	}
	switch msg.Kind {
	case KindSkipped:
		if len(msg.Block) != 0 {
			return fmt.Errorf("kind=skipped must not carry block")
		}
	case KindBlock:
		if len(msg.Block) == 0 {
			return fmt.Errorf("kind=block requires block JSON")
		}
		if !json.Valid(msg.Block) {
			return fmt.Errorf("kind=block block is not valid JSON")
		}
	default:
		return fmt.Errorf("block message kind %q, want %q or %q", msg.Kind, KindBlock, KindSkipped)
	}
	return nil
}
