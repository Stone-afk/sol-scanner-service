package scanner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/ctxwait"
	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
)

// errCaughtUp means next slot is past finalized; Run should poll-wait.
var errCaughtUp = errors.New("scanner: caught up")

// errCursorStore marks GetHandedSlot / AdvanceHandedSlot failures so Run can
// treat them as fatal while Produce and non-unsupported provider errors retry.
var errCursorStore = errors.New("scanner: cursor store")

// BlockProducer publishes block-queue messages. *block.Producer satisfies this.
type BlockProducer interface {
	Produce(ctx context.Context, msgs []block.BlockMessage) error
}

// HaltChecker reports whether an unresolved scan halt exists.
type HaltChecker interface {
	HasUnresolvedHalt(ctx context.Context) (bool, error)
}

// Config holds scanner handoff loop settings.
type Config struct {
	Chain          string
	GenesisHash    string
	StartSlot      uint64
	PollInterval   time.Duration
	StepSlots      uint64
	SlotsPerWorker uint64
}

// Scanner runs the FR-10 segmented handoff loop.
// Pointer receiver: owns loop lifecycle and injectable wait clock.
type Scanner struct {
	reader   provider.Reader
	producer BlockProducer
	cursor   repository.ScanCursorRepository
	halts    HaltChecker
	cfg      Config
	logger   *slog.Logger
	wait     func(ctx context.Context, d time.Duration) error
}

// New constructs a Scanner. Dependencies must be non-nil from IoC assembly.
func New(
	reader provider.Reader,
	producer BlockProducer,
	cursor repository.ScanCursorRepository,
	halts HaltChecker,
	cfg Config,
	logger *slog.Logger,
) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{
		reader:   reader,
		producer: producer,
		cursor:   cursor,
		halts:    halts,
		cfg:      cfg,
		logger:   logger,
		wait:     ctxwait.Wait,
	}
}

// Run loops until ctx is cancelled or a fatal error occurs.
// Unresolved halt waits without exiting. Fatal (return, no halt write):
// UNSUPPORTED_VERSION, handed-slot conflict, and cursor/halt DB errors.
// Retriable (log, wait pollInterval): other provider failures and Produce failures.
func (s *Scanner) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		halted, err := s.halts.HasUnresolvedHalt(ctx)
		if err != nil {
			return err // halt-check DB errors are fatal (AC-069)
		}
		if halted {
			if waitErr := s.wait(ctx, s.cfg.PollInterval); waitErr != nil {
				return waitErr
			}
			continue
		}

		err = s.Step(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, errCaughtUp) {
			if waitErr := s.wait(ctx, s.cfg.PollInterval); waitErr != nil {
				return waitErr
			}
			continue
		}
		if isFatalStep(err) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.logger.Error("handoff step failed", "err", err)
		if waitErr := s.wait(ctx, s.cfg.PollInterval); waitErr != nil {
			return waitErr
		}
	}
}

// isFatalStep reports Run-fatal Step errors: unsupported version and cursor-store
// failures (including ErrHandedSlotConflict and Get/Advance DB errors).
func isFatalStep(err error) bool {
	if isUnsupported(err) {
		return true
	}
	if errors.Is(err, errCursorStore) {
		return true
	}
	return errors.Is(err, repository.ErrHandedSlotConflict)
}

func wrapCursorErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", errCursorStore, err)
}

// Step performs one handoff step: FinalizedSlot first, segment workers, then
// AdvanceHandedSlot only if every segment succeeds. Returns errCaughtUp when
// there is nothing to hand off yet.
func (s *Scanner) Step(ctx context.Context) error {
	handed, hasCursor, err := s.cursor.GetHandedSlot(ctx)
	if err != nil {
		return wrapCursorErr(err)
	}
	var next uint64
	var prev uint64
	if hasCursor {
		next = handed + 1
		prev = handed
	} else {
		next = s.cfg.StartSlot
		prev = s.cfg.StartSlot - 1
	}

	finalized, err := s.reader.FinalizedSlot(ctx)
	if err != nil {
		return err
	}
	if next > finalized {
		return errCaughtUp
	}

	right := next + s.cfg.StepSlots - 1
	if right < next { // overflow
		right = finalized
	}
	if right > finalized {
		right = finalized
	}

	segs := Segments(next, right, s.cfg.SlotsPerWorker)
	if len(segs) == 0 {
		return errCaughtUp
	}

	if err := s.runWorkers(ctx, segs); err != nil {
		return err
	}
	if err := s.cursor.AdvanceHandedSlot(ctx, prev, right); err != nil {
		return wrapCursorErr(err)
	}
	return nil
}

func (s *Scanner) runWorkers(ctx context.Context, segs [][2]uint64) error {
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(segs))
	var wg sync.WaitGroup
	for _, seg := range segs {
		seg := seg
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.runSegment(workerCtx, seg[0], seg[1]); err != nil {
				cancel() // cancel siblings in this step
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	var first error
	for err := range errCh {
		if first == nil {
			first = err
		}
		if isUnsupported(err) {
			return err
		}
	}
	return first
}
