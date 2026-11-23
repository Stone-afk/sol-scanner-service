package block

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubPoller struct {
	msgs []BlockMessage
	n    int
}

func (s *stubPoller) Poll(ctx context.Context) (BlockMessage, Offset, error) {
	if s.n >= len(s.msgs) {
		if err := ctx.Err(); err != nil {
			return BlockMessage{}, Offset{}, err
		}
		return BlockMessage{}, Offset{}, context.DeadlineExceeded
	}
	msg := s.msgs[s.n]
	off := Offset{Topic: "t", Partition: 0, Offset: int64(s.n)}
	s.n++
	return msg, off, nil
}

func TestDrainPullsUpToMaxThenStops(t *testing.T) {
	t.Parallel()
	p := &stubPoller{msgs: []BlockMessage{
		{Version: MessageVersion, Chain: "c", Slot: 1, Commitment: CommitmentFinalized, Kind: KindSkipped},
		{Version: MessageVersion, Chain: "c", Slot: 2, Commitment: CommitmentFinalized, Kind: KindSkipped},
		{Version: MessageVersion, Chain: "c", Slot: 3, Commitment: CommitmentFinalized, Kind: KindSkipped},
	}}
	got, err := Drain(context.Background(), p, 2, time.Millisecond)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(got) != 2 || got[0].Msg.Slot != 1 || got[1].Msg.Slot != 2 {
		t.Fatalf("got = %+v, want slots 1,2", got)
	}
	if p.n != 2 {
		t.Fatalf("polls = %d, want 2", p.n)
	}
}

func TestDrainStopsOnEmpty(t *testing.T) {
	t.Parallel()
	p := &stubPoller{msgs: []BlockMessage{
		{Version: MessageVersion, Chain: "c", Slot: 9, Commitment: CommitmentFinalized, Kind: KindSkipped},
	}}
	got, err := Drain(context.Background(), p, 8, time.Millisecond)
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(got) != 1 || got[0].Msg.Slot != 9 {
		t.Fatalf("got = %+v, want one slot 9", got)
	}
}

func TestDrainPropagatesQueueNotReady(t *testing.T) {
	t.Parallel()
	p := pollFunc(func(context.Context) (BlockMessage, Offset, error) {
		return BlockMessage{}, Offset{}, ErrQueueNotReady
	})
	got, err := Drain(context.Background(), p, 4, time.Millisecond)
	if !errors.Is(err, ErrQueueNotReady) {
		t.Fatalf("err = %v, want ErrQueueNotReady", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want empty", got)
	}
}

type pollFunc func(context.Context) (BlockMessage, Offset, error)

func (f pollFunc) Poll(ctx context.Context) (BlockMessage, Offset, error) {
	return f(ctx)
}
