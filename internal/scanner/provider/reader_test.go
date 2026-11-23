package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Stone-afk/chain-api-go/apierror"
	"github.com/Stone-afk/chain-api-go/jsonrpc"
	"github.com/Stone-afk/chain-api-go/solana"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

const (
	testGenesisHash = "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d"
	testBlockJSON   = `{"blockhash":"5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d","previousBlockhash":"5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d","parentSlot":6,"transactions":[]}`
)

type recordedCall struct {
	method string
	params json.RawMessage
}

type fakeCaller struct {
	calls   []recordedCall
	replies []any // string JSON reply or error
}

func (f *fakeCaller) Call(_ context.Context, method string, params, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	f.calls = append(f.calls, recordedCall{method: method, params: raw})
	if len(f.replies) == 0 {
		return apierror.Error{Kind: apierror.Protocol, Method: method, Message: "no scripted reply"}
	}
	reply := f.replies[0]
	f.replies = f.replies[1:]
	if err, ok := reply.(error); ok {
		return err
	}
	s, ok := reply.(string)
	if !ok {
		return apierror.Error{Kind: apierror.Protocol, Method: method, Message: "invalid scripted reply"}
	}
	if err := json.Unmarshal([]byte(s), out); err != nil {
		return apierror.Error{Kind: apierror.Protocol, Method: method, Cause: err}
	}
	return nil
}

func newTestReader(t *testing.T, f *fakeCaller) *reader {
	t.Helper()
	client, err := solana.New(f)
	if err != nil {
		t.Fatalf("solana.New: %v", err)
	}
	return newReader(client, func(context.Context, time.Duration) error { return nil })
}

func sameJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	decode := func(b []byte) any {
		var v any
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		if err := d.Decode(&v); err != nil {
			t.Fatalf("json decode: %v", err)
		}
		return v
	}
	if !reflect.DeepEqual(decode(got), decode([]byte(want))) {
		t.Fatalf("params JSON mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

func TestReaderWireContracts(t *testing.T) {
	tests := []struct {
		name       string
		reply      string
		wantMethod string
		wantParams string
		call       func(context.Context, Reader) error
	}{
		{
			name:       "PV-01 GenesisHash",
			reply:      `"` + testGenesisHash + `"`,
			wantMethod: "getGenesisHash",
			wantParams: `null`,
			call: func(ctx context.Context, r Reader) error {
				got, err := r.GenesisHash(ctx)
				if err != nil {
					return err
				}
				if got != testGenesisHash {
					t.Fatalf("GenesisHash = %q, want %q", got, testGenesisHash)
				}
				return nil
			},
		},
		{
			name:       "PV-02 FinalizedSlot",
			reply:      `42`,
			wantMethod: "getSlot",
			wantParams: `[{"commitment":"finalized"}]`,
			call: func(ctx context.Context, r Reader) error {
				got, err := r.FinalizedSlot(ctx)
				if err != nil {
					return err
				}
				if got != 42 {
					t.Fatalf("FinalizedSlot = %d, want 42", got)
				}
				return nil
			},
		},
		{
			name:       "PV-03 FirstAvailableBlock",
			reply:      `0`,
			wantMethod: "getFirstAvailableBlock",
			wantParams: `null`,
			call: func(ctx context.Context, r Reader) error {
				got, err := r.FirstAvailableBlock(ctx)
				if err != nil {
					return err
				}
				if got != 0 {
					t.Fatalf("FirstAvailableBlock = %d, want 0", got)
				}
				return nil
			},
		},
		{
			name:       "PV-04 BlockSlots",
			reply:      `[5,7,10]`,
			wantMethod: "getBlocks",
			wantParams: `[5,10,{"commitment":"finalized"}]`,
			call: func(ctx context.Context, r Reader) error {
				got, err := r.BlockSlots(ctx, 5, 10)
				if err != nil {
					return err
				}
				want := []uint64{5, 7, 10}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("BlockSlots = %#v, want %#v", got, want)
				}
				return nil
			},
		},
		{
			name:       "PV-05 Block",
			reply:      testBlockJSON,
			wantMethod: "getBlock",
			wantParams: `[7,{"commitment":"finalized","encoding":"base64","maxSupportedTransactionVersion":1,"rewards":false,"transactionDetails":"full"}]`,
			call: func(ctx context.Context, r Reader) error {
				got, err := r.Block(ctx, 7)
				if err != nil {
					return err
				}
				if got == nil {
					t.Fatal("Block returned nil result")
				}
				if got.ParentSlot != 6 {
					t.Fatalf("ParentSlot = %d, want 6", got.ParentSlot)
				}
				return nil
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeCaller{replies: []any{tc.reply}}
			r := newTestReader(t, f)
			if err := tc.call(context.Background(), r); err != nil {
				t.Fatalf("call error: %v", err)
			}
			if len(f.calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(f.calls))
			}
			if f.calls[0].method != tc.wantMethod {
				t.Fatalf("method = %q, want %q", f.calls[0].method, tc.wantMethod)
			}
			sameJSON(t, f.calls[0].params, tc.wantParams)
		})
	}
}

func TestBlockNullNotConfirmed(t *testing.T) {
	// PV-10 via real solana.Client null → ErrNotConfirmed wrapping chain.
	f := &fakeCaller{replies: []any{`null`}}
	r := newTestReader(t, f)
	_, err := r.Block(context.Background(), 7)
	if KindOf(err) != ErrorKindRPC {
		t.Fatalf("KindOf = %q, want RPC; err=%v", KindOf(err), err)
	}
	if !errors.Is(err, rpc.ErrNotConfirmed) {
		t.Fatalf("errors.Is(ErrNotConfirmed) = false; err=%v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(f.calls))
	}
}

func TestReaderClassifiesWrappedAPIError(t *testing.T) {
	// Plan risk: apierror survives solana.Client callerError wrapping.
	f := &fakeCaller{replies: []any{
		apierror.Error{Kind: apierror.RPC, RPCCode: -32004, HasRPCCode: true, Message: "slot skipped"},
	}}
	var sleeps []time.Duration
	client, err := solana.New(f)
	if err != nil {
		t.Fatalf("solana.New: %v", err)
	}
	r := newReader(client, recordingSleep(&sleeps))
	_, err = r.FinalizedSlot(context.Background())
	if KindOf(err) != ErrorKindRPC {
		t.Fatalf("KindOf = %q, want RPC; err=%v", KindOf(err), err)
	}
	var pe Error
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want provider.Error", err)
	}
	if pe.RPCCode != -32004 {
		t.Fatalf("RPCCode = %d, want -32004", pe.RPCCode)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (no retry)", len(f.calls))
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %v, want none", sleeps)
	}
}

func TestBlockSlotsValidation(t *testing.T) {
	// PV-15
	tests := []struct {
		name  string
		reply string
	}{
		{name: "out_of_range", reply: `[4]`},
		{name: "not_increasing", reply: `[9,7]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeCaller{replies: []any{tc.reply}}
			r := newTestReader(t, f)
			_, err := r.BlockSlots(context.Background(), 5, 10)
			if KindOf(err) != ErrorKindPermanent {
				t.Fatalf("KindOf = %q, want PERMANENT; err=%v", KindOf(err), err)
			}
			if len(f.calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(f.calls))
			}
		})
	}
}

func TestNewReaderRejectsBadEndpoint(t *testing.T) {
	// Fragment makes jsonrpc.New reject configuration without dialing.
	endpoint := "https://example.invalid/v3/SECRETKEY#bad"
	_, err := NewReader(endpoint)
	if err == nil {
		t.Fatal("NewReader(bad endpoint) error = nil, want error")
	}
	msg := err.Error()
	if strings.Contains(msg, "SECRETKEY") || strings.Contains(msg, "example.invalid") || strings.Contains(msg, endpoint) {
		t.Fatalf("error leaked endpoint/secret: %q", msg)
	}
	if KindOf(err) != ErrorKindPermanent {
		t.Fatalf("KindOf = %q, want PERMANENT", KindOf(err))
	}
	var pe Error
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want provider.Error", err)
	}
	if pe.Operation != "connect" {
		t.Fatalf("Operation = %q, want connect", pe.Operation)
	}
}

// Ensure jsonrpc.Caller is implemented by fakeCaller at compile time.
var _ jsonrpc.Caller = (*fakeCaller)(nil)
