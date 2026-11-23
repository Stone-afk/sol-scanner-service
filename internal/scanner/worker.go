package scanner

import (
	"context"
	"encoding/json"

	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

// runSegment fetches one inclusive slot span once via BlockSlots, then produces
// kind=block for listed slots and kind=skipped for the rest. On BlockSlots
// failure it produces nothing. On getBlock failure it stops without producing
// remaining messages for this segment.
func (s *Scanner) runSegment(ctx context.Context, left, right uint64) error {
	slots, err := s.reader.BlockSlots(ctx, left, right)
	if err != nil {
		return err
	}
	present := make(map[uint64]struct{}, len(slots))
	for _, slot := range slots {
		present[slot] = struct{}{}
	}

	msgs := make([]block.BlockMessage, 0, right-left+1)
	for slot := left; slot <= right; slot++ {
		if _, ok := present[slot]; !ok {
			msgs = append(msgs, skippedMessage(s.cfg.Chain, slot))
			continue
		}
		raw, err := s.reader.Block(ctx, slot)
		if err != nil {
			return err
		}
		msg, err := blockMessage(s.cfg.Chain, slot, raw)
		if err != nil {
			return err
		}
		msgs = append(msgs, msg)
	}
	return s.producer.Produce(ctx, msgs)
}

func skippedMessage(chain string, slot uint64) block.BlockMessage {
	return block.BlockMessage{
		Version:    block.MessageVersion,
		Chain:      chain,
		Slot:       slot,
		Commitment: block.CommitmentFinalized,
		Kind:       block.KindSkipped,
	}
}

func blockMessage(chain string, slot uint64, raw *rpc.GetBlockResult) (block.BlockMessage, error) {
	payload, err := json.Marshal(raw)
	if err != nil {
		return block.BlockMessage{}, err
	}
	return block.BlockMessage{
		Version:    block.MessageVersion,
		Chain:      chain,
		Slot:       slot,
		Commitment: block.CommitmentFinalized,
		Kind:       block.KindBlock,
		Block:      payload,
	}, nil
}

// isUnsupported reports whether err is a fatal unsupported transaction version.
func isUnsupported(err error) bool {
	return provider.KindOf(err) == provider.ErrorKindUnsupportedVersion
}
