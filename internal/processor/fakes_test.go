package processor

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/Stone-afk/sol-scanner-service/internal/pkg/alert"
	"github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

const testChain = "solana-mainnet"

var errFakeEmpty = context.DeadlineExceeded

type queuedMsg struct {
	msg block.BlockMessage
	off block.Offset
}

type fakeConsumer struct {
	mu       sync.Mutex
	queue    []queuedMsg
	pollErrs []error
	commits  []block.Offset
	seeks    []block.Offset
	pollN    int
}

func (c *fakeConsumer) enqueue(msg block.BlockMessage, off block.Offset) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queue = append(c.queue, queuedMsg{msg: msg, off: off})
}

func (c *fakeConsumer) enqueueErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pollErrs = append(c.pollErrs, err)
}

func (c *fakeConsumer) Poll(ctx context.Context) (block.BlockMessage, block.Offset, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pollN++
	if len(c.pollErrs) > 0 {
		err := c.pollErrs[0]
		c.pollErrs = c.pollErrs[1:]
		return block.BlockMessage{}, block.Offset{}, err
	}
	// Deliver already-queued messages even if the short poll deadline has fired.
	// Under -race, a 1ms WithTimeout often expires before Poll runs; returning
	// DeadlineExceeded here would miss the message and hang tests that park on wait.
	if len(c.queue) > 0 {
		item := c.queue[0]
		c.queue = c.queue[1:]
		return item.msg, item.off, nil
	}
	if err := ctx.Err(); err != nil {
		return block.BlockMessage{}, block.Offset{}, err
	}
	return block.BlockMessage{}, block.Offset{}, errFakeEmpty
}

func (c *fakeConsumer) Commit(_ context.Context, off block.Offset) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commits = append(c.commits, off)
	return nil
}

func (c *fakeConsumer) Seek(_ context.Context, off block.Offset) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seeks = append(c.seeks, off)
	return nil
}

func (c *fakeConsumer) snapshot() (commits, seeks []block.Offset, pollN int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	commits = append([]block.Offset(nil), c.commits...)
	seeks = append([]block.Offset(nil), c.seeks...)
	return commits, seeks, c.pollN
}

type fakeRepo struct {
	mu            sync.Mutex
	checkpoint    domain.Checkpoint
	hasCP         bool
	batches       []domain.BlockBatch
	halts         []domain.Halt
	unresolved    bool
	saveErr       error
	createHaltErr error
	callOrder     []string
}

func (r *fakeRepo) GetCheckpoint(context.Context) (domain.Checkpoint, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.checkpoint, r.hasCP, nil
}

func (r *fakeRepo) SaveBlockBatch(_ context.Context, prev domain.Checkpoint, batch domain.BlockBatch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callOrder = append(r.callOrder, "SaveBlockBatch")
	if r.saveErr != nil {
		return r.saveErr
	}
	if r.hasCP && (prev.Slot != r.checkpoint.Slot || prev.Blockhash != r.checkpoint.Blockhash) {
		return errors.New("checkpoint conflict")
	}
	r.batches = append(r.batches, batch)
	r.checkpoint = domain.Checkpoint{Slot: batch.Block.Slot, Blockhash: batch.Block.Blockhash}
	r.hasCP = true
	return nil
}

func (r *fakeRepo) CreateHalt(_ context.Context, halt domain.Halt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callOrder = append(r.callOrder, "CreateHalt")
	if r.createHaltErr != nil {
		return r.createHaltErr
	}
	r.halts = append(r.halts, halt)
	r.unresolved = true
	return nil
}

func (r *fakeRepo) HasUnresolvedHalt(context.Context) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.unresolved, nil
}

func (r *fakeRepo) snapshot() (cp domain.Checkpoint, hasCP bool, batches []domain.BlockBatch, halts []domain.Halt, order []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	batches = append([]domain.BlockBatch(nil), r.batches...)
	halts = append([]domain.Halt(nil), r.halts...)
	order = append([]string(nil), r.callOrder...)
	return r.checkpoint, r.hasCP, batches, halts, order
}

type fakeCursor struct {
	mu         sync.Mutex
	handedSlot uint64
	hasHanded  bool
	rewinds    []uint64
}

func (c *fakeCursor) GetHandedSlot(context.Context) (uint64, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handedSlot, c.hasHanded, nil
}

func (c *fakeCursor) AdvanceHandedSlot(context.Context, uint64, uint64) error {
	return nil
}

func (c *fakeCursor) RewindHandedSlot(_ context.Context, slot uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rewinds = append(c.rewinds, slot)
	c.handedSlot = slot
	c.hasHanded = true
	return nil
}

func (c *fakeCursor) snapshot() (rewinds []uint64, handed uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]uint64(nil), c.rewinds...), c.handedSlot
}

type fakeAlerter struct {
	mu     sync.Mutex
	alerts []alert.Alert
	err    error
	repo   *fakeRepo
}

func (a *fakeAlerter) SendAlert(_ context.Context, al alert.Alert) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.repo != nil {
		a.repo.mu.Lock()
		a.repo.callOrder = append(a.repo.callOrder, "SendAlert")
		a.repo.mu.Unlock()
	}
	a.alerts = append(a.alerts, al)
	return a.err
}

func (a *fakeAlerter) snapshot() []alert.Alert {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]alert.Alert(nil), a.alerts...)
}

func hashForSlot(slot uint64, tag byte) solana.Hash {
	var h solana.Hash
	binary.BigEndian.PutUint64(h[24:], slot)
	h[0] = tag
	return h
}

func blockJSON(t *testing.T, slot, parentSlot uint64, prevHash, blockHash solana.Hash) []byte {
	t.Helper()
	blk := &rpc.GetBlockResult{
		Blockhash:         blockHash,
		PreviousBlockhash: prevHash,
		ParentSlot:        parentSlot,
	}
	raw, err := json.Marshal(blk)
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}
	return raw
}

func makeBlockMsg(t *testing.T, slot, parentSlot uint64, prevHash, blockHash solana.Hash, off block.Offset) (block.BlockMessage, block.Offset) {
	t.Helper()
	return block.BlockMessage{
		Version:    block.MessageVersion,
		Chain:      testChain,
		Slot:       slot,
		Commitment: block.CommitmentFinalized,
		Kind:       block.KindBlock,
		Block:      blockJSON(t, slot, parentSlot, prevHash, blockHash),
	}, off
}

func makeSkippedMsg(slot uint64, off block.Offset) (block.BlockMessage, block.Offset) {
	return block.BlockMessage{
		Version:    block.MessageVersion,
		Chain:      testChain,
		Slot:       slot,
		Commitment: block.CommitmentFinalized,
		Kind:       block.KindSkipped,
	}, off
}

func off(n int64) block.Offset {
	return block.Offset{Topic: "sol-scanner.blocks", Partition: 0, Offset: n}
}

func newTestProcessor(t *testing.T, cons *fakeConsumer, repo *fakeRepo, cursor *fakeCursor, alerter *fakeAlerter, startSlot uint64) *Processor {
	t.Helper()
	if alerter.repo == nil {
		alerter.repo = repo
	}
	p := New(cons, repo, cursor, alerter, Config{
		Chain:     testChain,
		StartSlot: startSlot,
		// Generous vs -race scheduling; waits are injected and do not sleep this long.
		PollInterval: 100 * time.Millisecond,
	}, nil)
	// Instant wait that still honors cancel (avoids racing into a post-success gap halt).
	p.wait = func(ctx context.Context, _ time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	}
	return p
}

// blockUntilCancelWait parks Apply on the first miss-wait until ctx is canceled.
func blockUntilCancelWait(p *Processor) {
	p.wait = func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}
}

// waitWhileNoCheckpoint then parks once a checkpoint exists (for out-of-order drains).
func waitWhileNoCheckpoint(p *Processor, repo *fakeRepo) {
	p.wait = func(ctx context.Context, _ time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, hasCP, _, _, _ := repo.snapshot()
		if hasCP {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
}
