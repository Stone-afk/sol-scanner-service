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
	"github.com/ecodeclub/ekit/pool"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

const gapMissLimit = 10

// shortcut: max 256 msgs per drain round (memory bound), raise if backlog regularly exceeds one round before apply catches up.
const maxDrainPerRound = 256

// defaultParseWorkers covers Config built without config.LoadForProcessor (tests).
const defaultParseWorkers = 4

// parseSlotHook, when set by tests, runs at the start of a block parse task (before decode).
// Production leaves it nil. Enables forcing out-of-order parse completion in AC-T01/AC-T02 tests.
var parseSlotHook func(slot uint64)

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
	ParseWorkers int // ekit/pool MaxGo / initGo for decode+ParseBlock
}

// applyState is the single writer's position: the next slot to apply, the last
// durable checkpoint, and the queue offset a halt rewinds to.
type applyState struct {
	slot      uint64
	cp        domain.Checkpoint
	hasCP     bool
	resume    block.Offset
	hasResume bool
}

// pendingMsg is one buffered queue message; parse is nil for skipped slots.
type pendingMsg struct {
	msg   block.BlockMessage
	off   block.Offset
	parse *parseResult
}

// parseResult is the async decode+ParseBlock outcome; read only after done closes.
type parseResult struct {
	done  chan struct{}
	batch domain.BlockBatch
	err   error
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
		if !halted {
			break
		}
		if err := p.wait(ctx, p.cfg.PollInterval); err != nil {
			return err
		}
	}
	if err := p.applyLoop(ctx); !errors.Is(err, errHalted) {
		return err
	}
	return nil
}

func (p *Processor) applyLoop(ctx context.Context) error {
	st, err := p.loadState(ctx)
	if err != nil {
		return err
	}
	parsePool, err := startParsePool(p.cfg.ParseWorkers)
	if err != nil {
		return err
	}
	defer shutdownParsePool(parsePool)

	pending := make(map[uint64]pendingMsg)
	misses := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		polled, err := p.drain(ctx)
		if errors.Is(err, block.ErrQueueNotReady) {
			// Topic auto-create race: wait, do not count toward GAP_UNRESOLVED.
			if err := p.wait(ctx, p.cfg.PollInterval); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		for _, item := range polled {
			if err := p.enqueue(ctx, parsePool, pending, st.slot, item); err != nil {
				return err
			}
		}

		next, err := p.applyReady(ctx, st, pending)
		if err != nil {
			return err
		}
		if next.slot != st.slot {
			st, misses = next, 0
			continue
		}
		// Out-of-order / covered traffic is progress on the queue — not a gap miss.
		if len(polled) > 0 {
			continue
		}
		misses++
		if misses >= gapMissLimit {
			return p.halt(ctx, st, domain.HaltReasonGapUnresolved, "apply slot missing for 10 polls")
		}
		if err := p.wait(ctx, p.cfg.PollInterval); err != nil {
			return err
		}
	}
}

func (p *Processor) loadState(ctx context.Context) (applyState, error) {
	cp, hasCP, err := p.repo.GetCheckpoint(ctx)
	if err != nil {
		return applyState{}, err
	}
	if !hasCP {
		return applyState{slot: p.cfg.StartSlot}, nil
	}
	return applyState{slot: cp.Slot + 1, cp: cp, hasCP: true}, nil
}

// drain polls one batch. An empty poll is not an error, and messages pulled
// before a not-ready error are kept; ErrQueueNotReady only surfaces when nothing arrived.
func (p *Processor) drain(ctx context.Context) ([]block.Polled, error) {
	polled, err := block.Drain(ctx, p.consumer, maxDrainPerRound, p.pollTimeout())
	switch {
	case err == nil:
		return polled, nil
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return polled, nil
	case errors.Is(err, block.ErrQueueNotReady) && len(polled) > 0:
		return polled, nil
	default:
		return nil, err
	}
}

// enqueue buffers one polled message and starts its parse. Already-applied slots
// are committed and dropped so the queue advances; repeats of a buffered slot are ignored.
func (p *Processor) enqueue(
	ctx context.Context,
	parsePool *pool.OnDemandBlockTaskPool,
	pending map[uint64]pendingMsg,
	applySlot uint64,
	item block.Polled,
) error {
	msg := item.Msg
	if msg.Slot < applySlot {
		p.logger.Debug("discard covered block message", "slot", msg.Slot, "apply_slot", applySlot)
		return p.consumer.Commit(ctx, item.Off)
	}
	if _, dup := pending[msg.Slot]; dup {
		return nil
	}
	m := pendingMsg{msg: msg, off: item.Off}
	if msg.Kind == block.KindBlock {
		r, err := submitParse(ctx, parsePool, msg)
		if err != nil {
			return err
		}
		m.parse = r
	}
	pending[msg.Slot] = m
	return nil
}

// applyReady applies buffered slots in slot order from st.slot until the first gap.
func (p *Processor) applyReady(ctx context.Context, st applyState, pending map[uint64]pendingMsg) (applyState, error) {
	for {
		m, ok := pending[st.slot]
		if !ok {
			return st, nil
		}
		if m.parse != nil {
			select {
			case <-m.parse.done:
			case <-ctx.Done():
				return st, ctx.Err()
			}
		}
		delete(pending, st.slot)
		next, err := p.applyOne(ctx, st, m)
		if err != nil {
			return st, err
		}
		st = next
	}
}

func (p *Processor) applyOne(ctx context.Context, st applyState, m pendingMsg) (applyState, error) {
	switch m.msg.Kind {
	case block.KindSkipped:
		if err := p.consumer.Commit(ctx, m.off); err != nil {
			return st, err
		}
		// Halt rewinds apply to checkpoint+1 (or startSlot). Queue must land on that
		// same slot. resume is the last message that stays committed after rewind
		// (Seek+Commit → next read = rewinded apply). A real block sets that directly.
		// Skipped-only progress after a DB checkpoint has no in-session block offset, so
		// the predecessor of this first skipped message is the durable resume point.
		// Later skips after a known resume leave resume unchanged (they are undone).
		if !st.hasResume {
			st.resume = block.Offset{Topic: m.off.Topic, Partition: m.off.Partition, Offset: m.off.Offset - 1}
			st.hasResume = true
		}
		st.slot++
		return st, nil

	case block.KindBlock:
		if m.parse.err != nil {
			return st, p.halt(ctx, st, domain.HaltReasonDecodeError, m.parse.err.Error())
		}
		batch := m.parse.batch
		if reason, detail, bad := continuityFail(batch, st.cp, st.hasCP, p.cfg.StartSlot, st.slot); bad {
			return st, p.halt(ctx, st, reason, detail)
		}
		if err := p.repo.SaveBlockBatch(ctx, st.cp, batch); err != nil {
			return st, err
		}
		if err := p.consumer.Commit(ctx, m.off); err != nil {
			return st, err
		}
		cp, hasCP, err := p.repo.GetCheckpoint(ctx)
		if err != nil {
			return st, err
		}
		st.cp, st.hasCP = cp, hasCP
		st.resume, st.hasResume = m.off, true
		st.slot++
		return st, nil

	default:
		return st, p.halt(ctx, st, domain.HaltReasonDecodeError, fmt.Sprintf("unknown kind %q", m.msg.Kind))
	}
}

func startParsePool(workers int) (*pool.OnDemandBlockTaskPool, error) {
	if workers < 1 {
		workers = defaultParseWorkers
	}
	parsePool, err := pool.NewOnDemandBlockTaskPool(workers, maxDrainPerRound, pool.WithMaxGo(int32(workers)))
	if err != nil {
		return nil, fmt.Errorf("create parse pool: %w", err)
	}
	if err := parsePool.Start(); err != nil {
		return nil, fmt.Errorf("start parse pool: %w", err)
	}
	return parsePool, nil
}

// submitParse runs decode+ParseBlock on the pool. done always closes; a panic
// becomes a parse error so the apply loop halts with DECODE_ERROR instead of hanging.
func submitParse(ctx context.Context, parsePool *pool.OnDemandBlockTaskPool, msg block.BlockMessage) (*parseResult, error) {
	r := &parseResult{done: make(chan struct{})}
	task := pool.TaskFunc(func(context.Context) error {
		defer close(r.done)
		defer func() {
			if v := recover(); v != nil {
				r.err = fmt.Errorf("parse panic: %v", v)
			}
		}()
		if hook := parseSlotHook; hook != nil {
			hook(msg.Slot)
		}
		r.batch, r.err = decodeAndParse(msg)
		return nil
	})
	if err := parsePool.Submit(ctx, task); err != nil {
		return nil, fmt.Errorf("submit parse slot %d: %w", msg.Slot, err)
	}
	return r, nil
}

func shutdownParsePool(parsePool *pool.OnDemandBlockTaskPool) {
	done, err := parsePool.Shutdown()
	if err != nil {
		_, _ = parsePool.ShutdownNow()
		return
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_, _ = parsePool.ShutdownNow()
	}
}

func (p *Processor) pollTimeout() time.Duration {
	if p.cfg.PollInterval > 0 {
		return p.cfg.PollInterval
	}
	return 100 * time.Millisecond
}

func decodeAndParse(msg block.BlockMessage) (domain.BlockBatch, error) {
	var blk rpc.GetBlockResult
	if err := json.Unmarshal(msg.Block, &blk); err != nil {
		return domain.BlockBatch{}, fmt.Errorf("decode block json: %w", err)
	}
	return parser.ParseBlock(msg.Slot, &blk)
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

// halt records the halt, alerts, rewinds the handed cursor and the queue, then
// returns errHalted (or the first storage/queue error).
func (p *Processor) halt(ctx context.Context, st applyState, reason domain.HaltReason, detail string) error {
	h := domain.Halt{
		Reason:    reason,
		Slot:      st.slot,
		Detail:    detail,
		CreatedAt: time.Now().Unix(),
	}
	if err := p.repo.CreateHalt(ctx, h); err != nil {
		return err
	}

	al := alert.Alert{
		Level:      alert.LevelCritical,
		Title:      "sol-scanner halt",
		Chain:      p.cfg.Chain,
		Slot:       st.slot,
		Reason:     reason.ToString(),
		Detail:     detail,
		OccurredAt: time.Now().UTC(),
	}
	if err := p.alerter.SendAlert(ctx, al); err != nil {
		p.logger.Error("SendAlert failed after halt", "err", err, "slot", st.slot, "reason", reason)
	}

	rewindTo := p.cfg.StartSlot
	if st.hasCP {
		rewindTo = st.cp.Slot
	} else if rewindTo > 0 {
		rewindTo--
	}
	if err := p.cursor.RewindHandedSlot(ctx, rewindTo); err != nil {
		return err
	}

	if st.hasResume {
		// Offset < 0 means "before first message"; Seek would treat -1 as Kafka OffsetEnd.
		if st.resume.Offset >= 0 {
			if err := p.consumer.Seek(ctx, st.resume); err != nil {
				return err
			}
		}
		if err := p.consumer.Commit(ctx, st.resume); err != nil {
			return err
		}
	}

	p.logger.Error("processor halted", "slot", st.slot, "reason", reason, "detail", detail)
	return errHalted
}
