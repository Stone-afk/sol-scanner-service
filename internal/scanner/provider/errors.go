package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Stone-afk/chain-api-go/apierror"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

const (
	blockCallTimeout = 30 * time.Second
	callTimeout      = 10 * time.Second
)

type ErrorKind string

const (
	ErrorKindTransient          ErrorKind = "TRANSIENT"
	ErrorKindRPC                ErrorKind = "RPC"
	ErrorKindUnsupportedVersion ErrorKind = "UNSUPPORTED_VERSION"
	ErrorKindPermanent          ErrorKind = "PERMANENT"
	ErrorKindCanceled           ErrorKind = "CANCELED"
)

func (k ErrorKind) ToString() string { return string(k) }

type Error struct {
	Operation  string
	Kind       ErrorKind
	RPCCode    int
	HTTPStatus int
	cause      error
}

func (e Error) Error() string {
	msg := fmt.Sprintf("solana provider: %s failed (%s", e.Operation, e.Kind.ToString())
	if e.RPCCode != 0 {
		msg += fmt.Sprintf(", rpc code %d", e.RPCCode)
	}
	if e.HTTPStatus != 0 {
		msg += fmt.Sprintf(", http status %d", e.HTTPStatus)
	}
	return msg + ")"
}

func (e Error) Unwrap() error { return e.cause }

func KindOf(err error) ErrorKind {
	var pe Error
	if errors.As(err, &pe) {
		return pe.Kind
	}
	return ErrorKindPermanent
}

func classify(err error) ErrorKind {
	if err == nil {
		return ErrorKindPermanent
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrorKindCanceled
	}
	if errors.Is(err, rpc.ErrNotConfirmed) {
		return ErrorKindRPC
	}
	var ae apierror.Error
	if errors.As(err, &ae) {
		switch ae.Kind {
		case apierror.Transport:
			return ErrorKindTransient
		case apierror.HTTP:
			if ae.HTTPStatus == 429 || ae.HTTPStatus >= 500 {
				return ErrorKindTransient
			}
			return ErrorKindPermanent
		case apierror.RPC:
			if ae.RPCCode == -32015 {
				return ErrorKindUnsupportedVersion
			}
			return ErrorKindRPC
		default:
			return ErrorKindPermanent
		}
	}
	return ErrorKindPermanent
}

func wrapError(op string, err error) Error {
	out := Error{
		Operation: op,
		Kind:      classify(err),
		cause:     err,
	}
	var ae apierror.Error
	if errors.As(err, &ae) {
		out.HTTPStatus = ae.HTTPStatus
		if ae.HasRPCCode {
			out.RPCCode = ae.RPCCode
		}
	}
	return out
}

func do(
	ctx context.Context,
	op string,
	timeout time.Duration,
	sleep func(context.Context, time.Duration) error,
	fn func(context.Context) error,
) error {
	const maxAttempts = 3
	waits := []time.Duration{250 * time.Millisecond, time.Second}
	var last error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		err := fn(callCtx)
		cancel()
		if err == nil {
			return nil
		}
		last = err
		kind := classify(err)
		if kind != ErrorKindTransient || attempt == maxAttempts-1 {
			return wrapError(op, err)
		}
		if sleepErr := sleep(ctx, waits[attempt]); sleepErr != nil {
			return wrapError(op, sleepErr)
		}
	}
	return wrapError(op, last)
}
