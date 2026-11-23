package scanner

import (
	"context"
	"fmt"
)

// Preflight validates scanner startup gates: genesis hash, startSlot when no
// handoff cursor exists, and earliest available block. Unresolved halt is not
// a preflight failure; Run waits instead.
func (s *Scanner) Preflight(ctx context.Context) error {
	genesis, err := s.reader.GenesisHash(ctx)
	if err != nil {
		return err
	}
	if genesis != s.cfg.GenesisHash {
		return fmt.Errorf("scanner: genesis hash mismatch")
	}

	_, hasCursor, err := s.cursor.GetHandedSlot(ctx)
	if err != nil {
		return err
	}
	if hasCursor {
		return nil
	}
	if s.cfg.StartSlot == 0 {
		return fmt.Errorf("scanner: startSlot is required when handoff cursor is empty")
	}
	earliest, err := s.reader.FirstAvailableBlock(ctx)
	if err != nil {
		return err
	}
	if s.cfg.StartSlot < earliest {
		return fmt.Errorf("scanner: startSlot %d is before earliest available block %d", s.cfg.StartSlot, earliest)
	}
	return nil
}
