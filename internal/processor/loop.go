package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/alert"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/ctxwait"
	"github.com/Stone-afk/sol-scanner-service/internal/processor/parser"
	"github.com/Stone-afk/sol-scanner-service/internal/repository"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

const gapMissLimit = 10

// BlockConsumer is the block-queue consumer surface used by the apply loop.
// *block.Consumer satisfies this interface.
type BlockConsumer interface {
	Poll(ctx context.Context) (block.BlockMessage, block.Offset, error)
	Commit(ctx context.Context, off block.Offset) error
	Seek(ctx context.Context, off block.Offset) error
}

// Config holds processor apply-loop settings.
type Config struct {
	Chain        string
	StartSlot    uint64
	PollInterval time.Duration
}

type bufferedMsg struct {
	msg block.BlockMessage
	off block.Offset
}

// Processor runs the single-goroutine FR-12 apply loop.
// Pointer receiver: owns loop lifecycle and injectable wait clock.
type Processor struct {
	consumer BlockConsumer
	repo     repository.ScanRepository
	cursor   repository.ScanCursorRepository
	alerter  alert.Alerter
	cfg      Config
	logger   *slog.Logger
	wait     func(ctx context.Context, d time.Duration) error
}

// New constructs a Processor. Dependencies must be non-nil from IoC assembly.
func New(
	consumer BlockConsumer,
	repo repository.ScanRepository,
	cursor repository.ScanCursorRepository,
	alerter alert.Alerter,
	cfg Config,
	logger *slog.Logger,
) *Processor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Processor{
		consumer: consumer,
		repo:     repo,
		cursor:   cursor,
		alerter:  alerter,
		cfg:      cfg,
		logger:   logger,
		wait:     ctxwait.Wait,
	}
}

// errHalted is an internal signal that halt+rewind finished; Apply returns nil.
var errHalted = errors.New("processor: halted")

// Apply runs the single apply goroutine until ctx cancel or a continuity halt.
// Unresolved halt waits without starting apply and without exiting the process.
func (p *Processor) Apply(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		halted, err := p.repo.HasUnresolvedHalt(ctx)
		if err != nil {
			return err
		}
		if halted {
			if waitErr := p.wait(ctx, p.cfg.PollInterval); waitErr != nil {
				return waitErr
			}
			continue
		}
		break
	}
	return p.applyLoop(ctx)
}

func (p *Processor) applyLoop(ctx context.Context) error {
	cp, hasCP, err := p.repo.GetCheckpoint(ctx)
	if err != nil {
		return err
	}

	var applySlot uint64
	if hasCP {
		applySlot = cp.Slot + 1
	} else {
		applySlot = p.cfg.StartSlot
	}

	var (
		resumeOffset block.Offset
		hasResume    bool
		misses       int
		buf          = make(map[uint64]bufferedMsg)
	)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		pollCtx, cancel := context.WithTimeout(ctx, p.pollTimeout())
		msg, off, pollErr := p.consumer.Poll(pollCtx)
		cancel()
		gotMsg := false
		if pollErr != nil {
			if errors.Is(pollErr, block.ErrQueueNotReady) {
				// Topic auto-create race: wait, do not count toward GAP_UNRESOLVED.
				if waitErr := p.wait(ctx, p.cfg.PollInterval); waitErr != nil {
					return waitErr
				}
				continue
			}
			if !isEmptyPoll(pollErr) {
				if errors.Is(pollErr, context.Canceled) || errors.Is(pollErr, context.DeadlineExceeded) {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					// Timed-out poll with parent ctx still alive: empty round.
				} else {
					return pollErr
				}
			}
		} else {
			gotMsg = true
			if msg.Slot < applySlot {
				// Duplicate / already-covered: discard and Commit so the queue advances.
				p.logger.Debug("discard covered block message", "slot", msg.Slot, "apply_slot", applySlot)
				if err := p.consumer.Commit(ctx, off); err != nil {
					return err
				}
			} else if _, exists := buf[msg.Slot]; !exists {
				buf[msg.Slot] = bufferedMsg{msg: msg, off: off}
			}
		}

		applied := false
		for {
			entry, ok := buf[applySlot]
			if !ok {
				break
			}
			delete(buf, applySlot)

			nextSlot, nextResume, resumeOK, applyErr := p.applyOne(ctx, entry, applySlot, cp, hasCP, resumeOffset, hasResume)
			if applyErr != nil {
				if errors.Is(applyErr, errHalted) {
					return nil
				}
				return applyErr
			}
			applySlot = nextSlot
			resumeOffset = nextResume
			hasResume = resumeOK
			cp2, has2, cpErr := p.repo.GetCheckpoint(ctx)
			if cpErr != nil {
				return cpErr
			}
			cp, hasCP = cp2, has2
			applied = true
		}

		if applied {
			misses = 0
			continue
		}
		// Out-of-order / covered traffic is progress on the queue — not a gap miss.
		if gotMsg {
			continue
		}

		misses++
		if misses >= gapMissLimit {
			if err := p.halt(ctx, domain.HaltReasonGapUnresolved, applySlot, "apply slot missing for 10 polls", resumeOffset, hasResume, cp, hasCP); err != nil {
				if errors.Is(err, errHalted) {
					return nil
				}
				return err
			}
			return nil
		}
		if waitErr := p.wait(ctx, p.cfg.PollInterval); waitErr != nil {
			return waitErr
		}
	}
}

func (p *Processor) pollTimeout() time.Duration {
	if p.cfg.PollInterval > 0 {
		return p.cfg.PollInterval
	}
	return 100 * time.Millisecond
}

func isEmptyPoll(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func (p *Processor) applyOne(
	ctx context.Context,
	entry bufferedMsg,
	applySlot uint64,
	cp domain.Checkpoint,
	hasCP bool,
	resumeOffset block.Offset,
	hasResume bool,
) (nextSlot uint64, nextResume block.Offset, resumeOK bool, err error) {
	nextSlot = applySlot
	nextResume = resumeOffset
	resumeOK = hasResume

	switch entry.msg.Kind {
	case block.KindSkipped:
		if err := p.consumer.Commit(ctx, entry.off); err != nil {
			return applySlot, resumeOffset, hasResume, err
		}
		// Halt rewinds apply to checkpoint+1 (or startSlot). Queue must land on that
		// same slot. resumeOffset is the last message that stays committed after rewind
		// (Seek+Commit → next read = rewinded apply). A real block sets that directly.
		// Skipped-only progress after a DB checkpoint has no in-session block offset, so
		// the predecessor of this first skipped message is the durable resume point.
		// Later skips after a known resume leave resumeOffset unchanged (they are undone).
		nextResume := resumeOffset
		nextHas := hasResume
		if !hasResume {
			nextResume = block.Offset{
				Topic:     entry.off.Topic,
				Partition: entry.off.Partition,
				Offset:    entry.off.Offset - 1,
			}
			nextHas = true
		}
		return applySlot + 1, nextResume, nextHas, nil

	case block.KindBlock:
		batch, decErr := decodeAndParse(entry.msg)
		if decErr != nil {
			haltErr := p.halt(ctx, domain.HaltReasonDecodeError, applySlot, decErr.Error(), resumeOffset, hasResume, cp, hasCP)
			if haltErr == nil || errors.Is(haltErr, errHalted) {
				return applySlot, resumeOffset, hasResume, errHalted
			}
			return applySlot, resumeOffset, hasResume, haltErr
		}

		if reason, detail, bad := continuityFail(batch, cp, hasCP, p.cfg.StartSlot, applySlot); bad {
			haltErr := p.halt(ctx, reason, applySlot, detail, resumeOffset, hasResume, cp, hasCP)
			if haltErr == nil || errors.Is(haltErr, errHalted) {
				return applySlot, resumeOffset, hasResume, errHalted
			}
			return applySlot, resumeOffset, hasResume, haltErr
		}

		prev := domain.Checkpoint{}
		if hasCP {
			prev = cp
		}
		if err := p.repo.SaveBlockBatch(ctx, prev, batch); err != nil {
			return applySlot, resumeOffset, hasResume, err
		}
		if err := p.consumer.Commit(ctx, entry.off); err != nil {
			return applySlot, resumeOffset, hasResume, err
		}
		return applySlot + 1, entry.off, true, nil

	default:
		haltErr := p.halt(ctx, domain.HaltReasonDecodeError, applySlot, fmt.Sprintf("unknown kind %q", entry.msg.Kind), resumeOffset, hasResume, cp, hasCP)
		if haltErr == nil || errors.Is(haltErr, errHalted) {
			return applySlot, resumeOffset, hasResume, errHalted
		}
		return applySlot, resumeOffset, hasResume, haltErr
	}
}

func decodeAndParse(msg block.BlockMessage) (domain.BlockBatch, error) {
	var blk rpc.GetBlockResult
	if err := json.Unmarshal(msg.Block, &blk); err != nil {
		return domain.BlockBatch{}, fmt.Errorf("decode block json: %w", err)
	}
	batch, err := parser.ParseBlock(msg.Slot, &blk)
	if err != nil {
		return domain.BlockBatch{}, err
	}
	return batch, nil
}

func continuityFail(
	batch domain.BlockBatch,
	cp domain.Checkpoint,
	hasCP bool,
	startSlot uint64,
	applySlot uint64,
) (domain.HaltReason, string, bool) {
	if !hasCP {
		// No parent-hash compare. Prefix from startSlot to block-1 must already be applied as skipped
		// (i.e. applySlot must equal the block slot — missing prefix keeps applySlot behind).
		if applySlot != batch.Block.Slot {
			return domain.HaltReasonGapUnresolved, "missing skipped prefix before first block", true
		}
		if batch.Block.Slot < startSlot {
			return domain.HaltReasonGapUnresolved, "block slot before startSlot", true
		}
		return "", "", false
	}

	parent := batch.Block.ParentSlot
	switch {
	case parent > cp.Slot:
		return domain.HaltReasonGapUnresolved,
			fmt.Sprintf("parentSlot %d > checkpoint %d", parent, cp.Slot), true
	case parent < cp.Slot:
		return domain.HaltReasonParentBehind,
			fmt.Sprintf("parentSlot %d < checkpoint %d", parent, cp.Slot), true
	case batch.Block.PreviousBlockhash != cp.Blockhash:
		return domain.HaltReasonHashMismatch,
			"previousBlockhash does not match checkpoint", true
	default:
		return "", "", false
	}
}

func (p *Processor) halt(
	ctx context.Context,
	reason domain.HaltReason,
	slot uint64,
	detail string,
	resumeOffset block.Offset,
	hasResume bool,
	cp domain.Checkpoint,
	hasCP bool,
) error {
	halt := domain.Halt{
		Reason:    reason,
		Slot:      slot,
		Detail:    detail,
		CreatedAt: time.Now().Unix(),
	}
	if err := p.repo.CreateHalt(ctx, halt); err != nil {
		return err
	}

	al := alert.Alert{
		Level:      alert.LevelCritical,
		Title:      "sol-scanner halt",
		Chain:      p.cfg.Chain,
		Slot:       slot,
		Reason:     reason.ToString(),
		Detail:     detail,
		OccurredAt: time.Now().UTC(),
	}
	if err := p.alerter.SendAlert(ctx, al); err != nil {
		p.logger.Error("SendAlert failed after halt", "err", err, "slot", slot, "reason", reason)
	}

	var rewindTo uint64
	if hasCP {
		rewindTo = cp.Slot
	} else {
		if p.cfg.StartSlot == 0 {
			rewindTo = 0
		} else {
			rewindTo = p.cfg.StartSlot - 1
		}
	}
	if err := p.cursor.RewindHandedSlot(ctx, rewindTo); err != nil {
		return err
	}

	if hasResume {
		// Offset < 0 means "before first message"; Seek would treat -1 as Kafka OffsetEnd.
		if resumeOffset.Offset >= 0 {
			if err := p.consumer.Seek(ctx, resumeOffset); err != nil {
				return err
			}
		}
		if err := p.consumer.Commit(ctx, resumeOffset); err != nil {
			return err
		}
	}

	p.logger.Error("processor halted", "slot", slot, "reason", reason, "detail", detail)
	return errHalted
}
