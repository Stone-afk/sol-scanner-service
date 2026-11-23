package processor

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/domain"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
)

func TestHaltBeforeAlertAndRewindWithCheckpoint(t *testing.T) {
	// S-18 / AC-092
	t.Parallel()

	cpHash := hashForSlot(101, 1)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 101, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	// Seed a prior saved batch so we can assert rows unchanged.
	repo.batches = []domain.BlockBatch{{
		Block: domain.Block{Slot: 101, Blockhash: cpHash.String()},
	}}
	cursor := &fakeCursor{handedSlot: 200, hasHanded: true}
	alerter := &fakeAlerter{repo: repo}

	wrongPrev := hashForSlot(101, 9)
	bh := hashForSlot(102, 2)
	msg, o := makeBlockMsg(t, 102, 101, wrongPrev, bh, off(5))
	cons.enqueue(msg, o)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Apply(ctx)

	_, _, batches, halts, order := repo.snapshot()
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonHashMismatch {
		t.Fatalf("halts = %+v", halts)
	}
	if len(batches) != 1 || batches[0].Block.Slot != 101 {
		t.Fatalf("stored rows must be unchanged; batches=%+v", batches)
	}

	haltIdx, alertIdx := -1, -1
	for i, s := range order {
		switch s {
		case "CreateHalt":
			if haltIdx < 0 {
				haltIdx = i
			}
		case "SendAlert":
			if alertIdx < 0 {
				alertIdx = i
			}
		}
	}
	if haltIdx < 0 || alertIdx < 0 || haltIdx >= alertIdx {
		t.Fatalf("halt must precede alert; order=%v", order)
	}

	rewinds, handed := cursor.snapshot()
	if len(rewinds) != 1 || rewinds[0] != 101 || handed != 101 {
		t.Fatalf("RewindHandedSlot = %v handed=%d, want 101", rewinds, handed)
	}

	alerts := alerter.snapshot()
	if len(alerts) != 1 || alerts[0].Reason != string(domain.HaltReasonHashMismatch) {
		t.Fatalf("alerts = %+v", alerts)
	}
}

func TestHaltRewindWithoutCheckpointUsesStartSlotMinusOne(t *testing.T) {
	// S-19 / AC-092
	t.Parallel()

	cons := &fakeConsumer{}
	repo := &fakeRepo{}
	cursor := &fakeCursor{handedSlot: 600, hasHanded: true}
	alerter := &fakeAlerter{repo: repo}

	p := newTestProcessor(t, cons, repo, cursor, alerter, 500)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Apply(ctx)

	_, _, _, halts, _ := repo.snapshot()
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonGapUnresolved {
		t.Fatalf("halts = %+v, want GAP_UNRESOLVED", halts)
	}
	rewinds, handed := cursor.snapshot()
	if len(rewinds) != 1 || rewinds[0] != 499 || handed != 499 {
		t.Fatalf("RewindHandedSlot = %v handed=%d, want 499", rewinds, handed)
	}
}

func TestHaltAlertFailureKeepsHaltRow(t *testing.T) {
	// AC-092
	t.Parallel()

	cpHash := hashForSlot(101, 1)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 101, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 101, hasHanded: true}
	alerter := &fakeAlerter{repo: repo, err: errors.New("alert boom")}

	wrongPrev := hashForSlot(101, 9)
	msg, o := makeBlockMsg(t, 102, 101, wrongPrev, hashForSlot(102, 2), off(1))
	cons.enqueue(msg, o)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.Apply(ctx)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("Apply should not fail fatally on alert error; err=%v", err)
	}

	_, _, _, halts, order := repo.snapshot()
	if len(halts) != 1 {
		t.Fatalf("halt row must remain after alert failure; halts=%+v", halts)
	}
	// Rewind still happens after alert attempt.
	rewinds, _ := cursor.snapshot()
	if len(rewinds) != 1 || rewinds[0] != 101 {
		t.Fatalf("rewinds = %v, want [101]", rewinds)
	}
	foundAlert := false
	for _, s := range order {
		if s == "SendAlert" {
			foundAlert = true
		}
	}
	if !foundAlert {
		t.Fatalf("SendAlert must still be attempted; order=%v", order)
	}
}

func TestCommitOffsetOnlyAfterSaveOrSkipped(t *testing.T) {
	// AC-093
	t.Parallel()

	cpHash := hashForSlot(100, 1)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
	alerter := &fakeAlerter{repo: repo}

	bh := hashForSlot(101, 2)
	msg, o := makeBlockMsg(t, 101, 100, cpHash, bh, off(7))
	cons.enqueue(msg, o)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	blockUntilCancelWait(p)

	saved := make(chan struct{})
	go func() {
		for {
			_, _, batches, _, _ := repo.snapshot()
			if len(batches) == 1 {
				close(saved)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- p.Apply(ctx) }()

	select {
	case <-saved:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timeout waiting for save")
	}
	<-errCh

	commits, _, _ := cons.snapshot()
	if len(commits) != 1 || commits[0].Offset != 7 {
		t.Fatalf("commit only after save; commits=%+v", commits)
	}
}

func TestHaltSeeksResumeOffsetAfterLastSuccessfulBlock(t *testing.T) {
	// AC-093
	t.Parallel()

	cpHash := hashForSlot(100, 1)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
	alerter := &fakeAlerter{repo: repo}

	bh101 := hashForSlot(101, 2)
	msg101, o7 := makeBlockMsg(t, 101, 100, cpHash, bh101, off(7))
	sk102, o8 := makeSkippedMsg(102, off(8))
	// 103 parent behind → halt; resumeOffset should be offset of last successful block (7)
	msg103, o9 := makeBlockMsg(t, 103, 99, bh101, hashForSlot(103, 3), off(9))
	cons.enqueue(msg101, o7)
	cons.enqueue(sk102, o8)
	cons.enqueue(msg103, o9)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Apply(ctx)

	_, _, batches, halts, _ := repo.snapshot()
	if len(batches) != 1 || batches[0].Block.Slot != 101 {
		t.Fatalf("batches = %+v, want only 101", batches)
	}
	if len(halts) != 1 {
		t.Fatalf("halts = %+v", halts)
	}

	commits, seeks, _ := cons.snapshot()
	// Successful block + skipped committed before halt.
	if len(commits) < 2 {
		t.Fatalf("commits before halt = %+v", commits)
	}
	if len(seeks) != 1 || seeks[0].Offset != 7 {
		t.Fatalf("Seek(resumeOffset) = %+v, want offset 7 (last successful block)", seeks)
	}
	// After Seek, Commit that resumeOffset.
	foundSeekCommit := false
	for i := len(commits) - 1; i >= 0; i-- {
		if commits[i].Offset == 7 {
			// last commits should include the post-halt Commit(resumeOffset)
			foundSeekCommit = true
			break
		}
	}
	if !foundSeekCommit {
		t.Fatalf("expected Commit(resumeOffset=7) after Seek; commits=%+v", commits)
	}
}

func TestHaltSeekAfterSkippedOnlyFromCheckpoint(t *testing.T) {
	// AUD-022-001 / S-14 / AC-093: cold-start from DB checkpoint, apply skipped, then halt.
	// Rewinded apply position is checkpoint+1 (=102). Queue must Seek+Commit so next
	// read is the skipped-102 message again — not remain past it.
	t.Parallel()

	cpHash := hashForSlot(101, 1)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 101, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 200, hasHanded: true}
	alerter := &fakeAlerter{repo: repo}

	sk102, o5 := makeSkippedMsg(102, off(5))
	bh := hashForSlot(103, 3)
	blk103, o6 := makeBlockMsg(t, 103, 102, hashForSlot(102, 2), bh, off(6))
	cons.enqueue(sk102, o5)
	cons.enqueue(blk103, o6)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Apply(ctx)

	_, _, batches, halts, _ := repo.snapshot()
	if len(batches) != 0 {
		t.Fatalf("must not save 103; batches=%d", len(batches))
	}
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonGapUnresolved {
		t.Fatalf("halts = %+v, want GAP_UNRESOLVED", halts)
	}
	rewinds, handed := cursor.snapshot()
	if len(rewinds) != 1 || rewinds[0] != 101 || handed != 101 {
		t.Fatalf("RewindHandedSlot = %v handed=%d, want 101", rewinds, handed)
	}

	// Last durable kept message before rewinded apply slot 102 is the predecessor of
	// skipped-102's offset (5-1=4). Seek+Commit(4) → next read offset 5 (= slot 102).
	wantResume := int64(4)
	commits, seeks, _ := cons.snapshot()
	if len(seeks) != 1 || seeks[0].Offset != wantResume {
		t.Fatalf("Seek(resumeOffset) = %+v, want offset %d (align queue with apply slot 102)", seeks, wantResume)
	}
	found := false
	for i := len(commits) - 1; i >= 0; i-- {
		if commits[i].Offset == wantResume {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected post-halt Commit(resumeOffset=%d); commits=%+v", wantResume, commits)
	}
	// Skipped-102 was committed during apply, then rewound via Seek — must not leave
	// consumer only at offset 5 without a rewind Seek.
	sawSkipCommit := false
	for _, c := range commits {
		if c.Offset == 5 {
			sawSkipCommit = true
			break
		}
	}
	if !sawSkipCommit {
		t.Fatalf("expected Commit of skipped offset 5 during apply; commits=%+v", commits)
	}
}

func TestDuplicateCoveredBlockDoesNotFail(t *testing.T) {
	// AC-093
	t.Parallel()

	cpHash := hashForSlot(101, 1)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 101, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 101, hasHanded: true}
	alerter := &fakeAlerter{repo: repo}

	// Duplicate of already-covered 101, then skipped 102 so we can observe progress.
	dup, o0 := makeBlockMsg(t, 101, 100, hashForSlot(100, 1), cpHash, off(0))
	sk102, o1 := makeSkippedMsg(102, off(1))
	cons.enqueue(dup, o0)
	cons.enqueue(sk102, o1)

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	p.wait = func(ctx context.Context, _ time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		commits, _, _ := cons.snapshot()
		for _, c := range commits {
			if c.Offset == 1 {
				<-ctx.Done()
				return ctx.Err()
			}
		}
		return nil
	}

	skippedDone := make(chan struct{})
	go func() {
		for {
			commits, _, _ := cons.snapshot()
			for _, c := range commits {
				if c.Offset == 1 {
					close(skippedDone)
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- p.Apply(ctx) }()

	select {
	case <-skippedDone:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timeout; duplicate may have blocked apply")
	}
	err := <-errCh
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("duplicate must not fail Apply; err=%v", err)
	}

	_, _, batches, _, _ := repo.snapshot()
	if len(batches) != 0 {
		t.Fatalf("duplicate must not write; batches=%d", len(batches))
	}
	commits, _, _ := cons.snapshot()
	sawDupCommit := false
	for _, c := range commits {
		if c.Offset == 0 {
			sawDupCommit = true
			break
		}
	}
	if !sawDupCommit {
		t.Fatalf("covered discard must Commit offset 0; commits=%+v", commits)
	}
}

func TestHaltResumeBeforeFirstMessageSkipsSeek(t *testing.T) {
	// First skipped at Kafka offset 0 → resumeOffset=-1 (Kafka OffsetEnd).
	// Seek(-1) would jump to end; only Commit(-1)→next=0 is safe.
	t.Parallel()

	cons := &fakeConsumer{}
	repo := &fakeRepo{}
	cursor := &fakeCursor{}
	alerter := &fakeAlerter{repo: repo}

	sk500, o0 := makeSkippedMsg(500, off(0))
	bad := block.BlockMessage{
		Version:    block.MessageVersion,
		Chain:      testChain,
		Slot:       501,
		Commitment: block.CommitmentFinalized,
		Kind:       block.KindBlock,
		Block:      []byte(`{`),
	}
	cons.enqueue(sk500, o0)
	cons.enqueue(bad, off(1))

	p := newTestProcessor(t, cons, repo, cursor, alerter, 500)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Apply(ctx)

	_, _, _, halts, _ := repo.snapshot()
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonDecodeError {
		t.Fatalf("halts = %+v, want DECODE_ERROR", halts)
	}
	commits, seeks, _ := cons.snapshot()
	if len(seeks) != 0 {
		t.Fatalf("Seek = %+v, want none when resumeOffset < 0", seeks)
	}
	found := false
	for _, c := range commits {
		if c.Offset == -1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want Commit(resumeOffset=-1) so next read is 0; commits=%+v", commits)
	}
}

func TestDecodeErrorHaltsWithoutCommit(t *testing.T) {
	// SX-01 / AC-093 companion
	t.Parallel()

	cpHash := hashForSlot(100, 1)
	cons := &fakeConsumer{}
	repo := &fakeRepo{
		checkpoint: domain.Checkpoint{Slot: 100, Blockhash: cpHash.String()},
		hasCP:      true,
	}
	cursor := &fakeCursor{handedSlot: 100, hasHanded: true}
	alerter := &fakeAlerter{repo: repo}

	msg := block.BlockMessage{
		Version:    block.MessageVersion,
		Chain:      testChain,
		Slot:       101,
		Commitment: block.CommitmentFinalized,
		Kind:       block.KindBlock,
		Block:      []byte(`{"blockhash":"not-a-valid-hash-shape"`), // invalid / truncated JSON
	}
	cons.enqueue(msg, off(3))

	p := newTestProcessor(t, cons, repo, cursor, alerter, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Apply(ctx)

	_, _, batches, halts, _ := repo.snapshot()
	if len(batches) != 0 {
		t.Fatalf("DECODE_ERROR must not commit block; batches=%d", len(batches))
	}
	if len(halts) != 1 || halts[0].Reason != domain.HaltReasonDecodeError {
		t.Fatalf("halts = %+v, want DECODE_ERROR", halts)
	}
	commits, seeks, _ := cons.snapshot()
	for _, c := range commits {
		if c.Offset == 3 {
			t.Fatalf("must not Commit bad block offset 3; commits=%+v", commits)
		}
	}
	for _, s := range seeks {
		if s.Offset == 3 {
			t.Fatalf("must not Seek to bad block offset 3; seeks=%+v", seeks)
		}
	}
	// No in-session durable progress: no resume Seek past the failed message.
	if len(seeks) != 0 {
		t.Fatalf("unexpected Seek without prior durable apply; seeks=%+v", seeks)
	}
}

func TestProductionProcessorImportsExcludeProviderAndChainAPI(t *testing.T) {
	// AC-094 processor half
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(".", name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		file, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "internal/scanner/provider") ||
				strings.Contains(path, "internal/rpc/provider") ||
				strings.Contains(path, "chain-api-go") {
				t.Fatalf("%s imports forbidden %q", name, path)
			}
		}
	}
}
