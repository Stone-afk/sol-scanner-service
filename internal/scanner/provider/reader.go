package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Stone-afk/chain-api-go/jsonrpc"
	"github.com/Stone-afk/chain-api-go/solana"
	solanago "github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

type Reader interface {
	GenesisHash(ctx context.Context) (string, error)
	FinalizedSlot(ctx context.Context) (uint64, error)
	FirstAvailableBlock(ctx context.Context) (uint64, error)
	BlockSlots(ctx context.Context, from, to uint64) ([]uint64, error)
	Block(ctx context.Context, slot uint64) (*rpc.GetBlockResult, error)
}

type reader struct {
	client *solana.Client
	sleep  func(context.Context, time.Duration) error
}

func NewReader(endpoint string) (Reader, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, Error{Operation: "connect", Kind: ErrorKindPermanent}
	}
	transport := base.Clone()
	transport.MaxIdleConnsPerHost = 64
	caller, err := jsonrpc.New(jsonrpc.Config{
		Endpoint:         endpoint,
		MaxResponseBytes: 64 << 20,
		Version:          jsonrpc.V2,
	}, &http.Client{Transport: transport})
	if err != nil {
		return nil, Error{Operation: "connect", Kind: ErrorKindPermanent, cause: err}
	}
	client, err := solana.New(caller)
	if err != nil {
		return nil, Error{Operation: "connect", Kind: ErrorKindPermanent, cause: err}
	}
	return newReader(client, defaultSleep), nil
}

func newReader(client *solana.Client, sleep func(context.Context, time.Duration) error) *reader {
	return &reader{client: client, sleep: sleep}
}

func defaultSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *reader) GenesisHash(ctx context.Context) (string, error) {
	var out string
	err := do(ctx, "getGenesisHash", callTimeout, r.sleep, func(callCtx context.Context) error {
		h, err := r.client.GetGenesisHash(callCtx)
		if err != nil {
			return err
		}
		out = h.String()
		return nil
	})
	return out, err
}

func (r *reader) FinalizedSlot(ctx context.Context) (uint64, error) {
	var out uint64
	err := do(ctx, "getSlot", callTimeout, r.sleep, func(callCtx context.Context) error {
		slot, err := r.client.GetSlotWithOpts(callCtx, &rpc.GetSlotOpts{Commitment: rpc.CommitmentFinalized})
		if err != nil {
			return err
		}
		out = slot
		return nil
	})
	return out, err
}

func (r *reader) FirstAvailableBlock(ctx context.Context) (uint64, error) {
	var out uint64
	err := do(ctx, "getFirstAvailableBlock", callTimeout, r.sleep, func(callCtx context.Context) error {
		slot, err := r.client.GetFirstAvailableBlock(callCtx)
		if err != nil {
			return err
		}
		out = slot
		return nil
	})
	return out, err
}

func (r *reader) BlockSlots(ctx context.Context, from, to uint64) ([]uint64, error) {
	var out []uint64
	err := do(ctx, "getBlocks", callTimeout, r.sleep, func(callCtx context.Context) error {
		end := to
		slots, err := r.client.GetBlocks(callCtx, from, &end, rpc.CommitmentFinalized)
		if err != nil {
			return err
		}
		if err := validateBlockSlots(from, to, slots); err != nil {
			return err
		}
		out = append([]uint64(nil), slots...)
		return nil
	})
	return out, err
}

func (r *reader) Block(ctx context.Context, slot uint64) (*rpc.GetBlockResult, error) {
	var out *rpc.GetBlockResult
	rewards := false
	err := do(ctx, "getBlock", blockCallTimeout, r.sleep, func(callCtx context.Context) error {
		blk, err := r.client.GetBlockWithOpts(callCtx, slot, &rpc.GetBlockOpts{
			Encoding:                       solanago.EncodingBase64,
			TransactionDetails:             rpc.TransactionDetailsFull,
			Rewards:                        &rewards,
			Commitment:                     rpc.CommitmentFinalized,
			MaxSupportedTransactionVersion: &rpc.MaxSupportedTransactionVersion1,
		})
		if err != nil {
			return err
		}
		out = blk
		return nil
	})
	return out, err
}

func validateBlockSlots(from, to uint64, slots []uint64) error {
	var prev uint64
	for i, s := range slots {
		if s < from || s > to {
			return fmt.Errorf("slot %d outside [%d,%d]", s, from, to)
		}
		if i > 0 && s <= prev {
			return fmt.Errorf("slots not strictly increasing")
		}
		prev = s
	}
	return nil
}
