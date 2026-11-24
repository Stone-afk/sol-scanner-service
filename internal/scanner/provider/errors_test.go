package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Stone-afk/chain-api-go/apierror"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

func recordingSleep(dst *[]time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		*dst = append(*dst, d)
		return nil
	}
}

func TestRetryTransient(t *testing.T) {
	t.Run("PV-06 transport then success", func(t *testing.T) {
		var attempts int
		var sleeps []time.Duration
		err := do(context.Background(), "getSlot", callTimeout, recordingSleep(&sleeps), func(context.Context) error {
			attempts++
			if attempts < 3 {
				return apierror.Error{Kind: apierror.Transport, Message: "dial timeout"}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("do error = %v", err)
		}
		if attempts != 3 {
			t.Fatalf("attempts = %d, want 3", attempts)
		}
		wantSleeps := []time.Duration{250 * time.Millisecond, 500 * time.Millisecond}
		if len(sleeps) != len(wantSleeps) {
			t.Fatalf("sleeps = %v, want %v", sleeps, wantSleeps)
		}
		for i := range wantSleeps {
			if sleeps[i] != wantSleeps[i] {
				t.Fatalf("sleeps = %v, want %v", sleeps, wantSleeps)
			}
		}
	})

	t.Run("PV-07 HTTP 429 x3", func(t *testing.T) {
		var attempts int
		var sleeps []time.Duration
		err := do(context.Background(), "getSlot", callTimeout, recordingSleep(&sleeps), func(context.Context) error {
			attempts++
			return apierror.Error{Kind: apierror.HTTP, HTTPStatus: 429, Message: "rate limited"}
		})
		if attempts != 3 {
			t.Fatalf("attempts = %d, want 3", attempts)
		}
		wantSleeps := []time.Duration{250 * time.Millisecond, 500 * time.Millisecond}
		if len(sleeps) != len(wantSleeps) {
			t.Fatalf("sleeps = %v, want %v", sleeps, wantSleeps)
		}
		for i := range wantSleeps {
			if sleeps[i] != wantSleeps[i] {
				t.Fatalf("sleeps = %v, want %v", sleeps, wantSleeps)
			}
		}
		if KindOf(err) != ErrorKindTransient {
			t.Fatalf("KindOf = %q, want TRANSIENT", KindOf(err))
		}
		if strings.Contains(err.Error(), "超过最大重试次数") {
			t.Fatalf("Error() = %q, must not carry ekit retry-exhausted message", err.Error())
		}
		var pe Error
		if !errors.As(err, &pe) {
			t.Fatalf("error type = %T, want provider.Error", err)
		}
		if pe.HTTPStatus != 429 {
			t.Fatalf("HTTPStatus = %d, want 429", pe.HTTPStatus)
		}
	})

	t.Run("PV-08 HTTP 503 then success", func(t *testing.T) {
		var attempts int
		var sleeps []time.Duration
		err := do(context.Background(), "getSlot", callTimeout, recordingSleep(&sleeps), func(context.Context) error {
			attempts++
			if attempts == 1 {
				return apierror.Error{Kind: apierror.HTTP, HTTPStatus: 503, Message: "unavailable"}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("do error = %v", err)
		}
		if attempts != 2 {
			t.Fatalf("attempts = %d, want 2", attempts)
		}
		wantSleeps := []time.Duration{250 * time.Millisecond}
		if len(sleeps) != 1 || sleeps[0] != wantSleeps[0] {
			t.Fatalf("sleeps = %v, want %v", sleeps, wantSleeps)
		}
	})
}

func TestNoRetryClassification(t *testing.T) {
	tests := []struct {
		name      string
		fnErr     error
		wantKind  ErrorKind
		wantRPC   int
		wantHTTP  int
		checkRPC  bool
		checkHTTP bool
	}{
		{
			name:     "PV-09 RPC -32004",
			fnErr:    apierror.Error{Kind: apierror.RPC, RPCCode: -32004, HasRPCCode: true, Message: "slot skipped"},
			wantKind: ErrorKindRPC,
			wantRPC:  -32004,
			checkRPC: true,
		},
		{
			name:     "PV-10 ErrNotConfirmed",
			fnErr:    rpc.ErrNotConfirmed,
			wantKind: ErrorKindRPC,
		},
		{
			name:     "PV-11 RPC -32015",
			fnErr:    apierror.Error{Kind: apierror.RPC, RPCCode: rpcCodeUnsupportedVersion, HasRPCCode: true, Message: "unsupported version"},
			wantKind: ErrorKindUnsupportedVersion,
			wantRPC:  rpcCodeUnsupportedVersion,
			checkRPC: true,
		},
		{
			name:      "PV-12 HTTP 401",
			fnErr:     apierror.Error{Kind: apierror.HTTP, HTTPStatus: 401, Message: "unauthorized"},
			wantKind:  ErrorKindPermanent,
			wantHTTP:  401,
			checkHTTP: true,
		},
		{
			name:     "PV-12 protocol",
			fnErr:    apierror.Error{Kind: apierror.Protocol, Message: "invalid json"},
			wantKind: ErrorKindPermanent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var attempts int
			var sleeps []time.Duration
			err := do(context.Background(), "getBlock", blockCallTimeout, recordingSleep(&sleeps), func(context.Context) error {
				attempts++
				return tc.fnErr
			})
			if attempts != 1 {
				t.Fatalf("attempts = %d, want 1", attempts)
			}
			if len(sleeps) != 0 {
				t.Fatalf("sleeps = %v, want none", sleeps)
			}
			if KindOf(err) != tc.wantKind {
				t.Fatalf("KindOf = %q, want %q; err=%v", KindOf(err), tc.wantKind, err)
			}
			if classify(tc.fnErr) != tc.wantKind {
				t.Fatalf("classify = %q, want %q", classify(tc.fnErr), tc.wantKind)
			}
			var pe Error
			if !errors.As(err, &pe) {
				t.Fatalf("error type = %T, want provider.Error", err)
			}
			if tc.checkRPC && pe.RPCCode != tc.wantRPC {
				t.Fatalf("RPCCode = %d, want %d", pe.RPCCode, tc.wantRPC)
			}
			if tc.checkHTTP && pe.HTTPStatus != tc.wantHTTP {
				t.Fatalf("HTTPStatus = %d, want %d", pe.HTTPStatus, tc.wantHTTP)
			}
		})
	}
}

func TestCancellationStopsRetry(t *testing.T) {
	// PV-13
	ctx, cancel := context.WithCancel(context.Background())
	var attempts int
	var sleeps []time.Duration
	err := do(ctx, "getSlot", callTimeout, func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		cancel()
		return context.Canceled
	}, func(context.Context) error {
		attempts++
		return apierror.Error{Kind: apierror.Transport, Message: "connection reset"}
	})
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if len(sleeps) != 1 || sleeps[0] != 250*time.Millisecond {
		t.Fatalf("sleeps = %v, want [250ms]", sleeps)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(Canceled) = false; err=%v", err)
	}
	if KindOf(err) != ErrorKindCanceled {
		t.Fatalf("KindOf = %q, want CANCELED", KindOf(err))
	}
}

func TestErrorRedaction(t *testing.T) {
	// PV-14
	secretURL := "https://example.invalid/v3/SECRETKEY"
	err := do(context.Background(), "getSlot", callTimeout, recordingSleep(new([]time.Duration)), func(context.Context) error {
		return apierror.Error{
			Kind:       apierror.Transport,
			Message:    "dial " + secretURL + " failed",
			Cause:      errors.New("Get \"" + secretURL + "\": connection refused"),
			HTTPStatus: 0,
		}
	})
	if err == nil {
		t.Fatal("do error = nil, want error")
	}
	msg := err.Error()
	if strings.Contains(msg, "SECRETKEY") {
		t.Fatalf("Error() leaked SECRETKEY: %q", msg)
	}
	if strings.Contains(msg, "example.invalid") {
		t.Fatalf("Error() leaked example.invalid: %q", msg)
	}
	if strings.Contains(msg, secretURL) {
		t.Fatalf("Error() leaked URL: %q", msg)
	}
	if !strings.Contains(msg, "solana provider:") {
		t.Fatalf("Error() = %q, want provider prefix", msg)
	}
	if KindOf(err) != ErrorKindTransient {
		t.Fatalf("KindOf = %q, want TRANSIENT", KindOf(err))
	}
}

func TestKindOfNonProvider(t *testing.T) {
	if KindOf(errors.New("other")) != ErrorKindPermanent {
		t.Fatalf("KindOf(non-provider) = %q, want PERMANENT", KindOf(errors.New("other")))
	}
}

func TestErrorKindToString(t *testing.T) {
	if ErrorKindTransient.ToString() != "TRANSIENT" {
		t.Fatalf("ToString = %q, want TRANSIENT", ErrorKindTransient.ToString())
	}
}
