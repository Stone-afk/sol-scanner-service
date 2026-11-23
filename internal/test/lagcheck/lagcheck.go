package lagcheck

import (
	"context"
	"errors"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
)

// LagThreshold is the A-04 last_lag limit (slots). Live harness must use this constant.
const LagThreshold = 150

type Verdict string

const (
	VerdictPass     Verdict = "pass"
	VerdictFail     Verdict = "fail"
	VerdictRecorded Verdict = "recorded"
)

type Gate string

const (
	GateSufficient   Gate = "sufficient"
	GateInsufficient Gate = "insufficient"
	GateUnknown      Gate = "unknown"
)

// Source supplies tip and checkpoint samples. Value semantics; funcs injected by caller.
type Source struct {
	FinalizedSlot func(context.Context) (uint64, error)
	Checkpoint    func(context.Context) (uint64, bool, error)
}

// Clock is injectable time for offline Observe tests.
type Clock struct {
	Now   func() time.Time
	Sleep func(context.Context, time.Duration) error
}

// Observation is the result of a lag watch window.
type Observation struct {
	Samples      int
	LastLag      uint64
	MaxLag       uint64
	LimitSignals int
	Err          error
	TipSamples   int
	FirstTip     uint64
	LastTip      uint64
	Elapsed      time.Duration
}

// Probe is a post-observe throughput measurement against recent finalized slots.
type Probe struct {
	Slots        uint64
	Elapsed      time.Duration
	LimitSignals int
}

// IsRateLimit reports whether err is an RPC rate-limit / retryable provider error.
func IsRateLimit(err error) bool {
	var pe provider.Error
	if errors.As(err, &pe) {
		return pe.HTTPStatus == 429 || pe.Kind == provider.ErrorKindTransient
	}
	var pep *provider.Error
	if errors.As(err, &pep) && pep != nil {
		return pep.HTTPStatus == 429 || pep.Kind == provider.ErrorKindTransient
	}
	return false
}

// Observe polls Source every interval until window elapses.
// Non-rate-limit errors stop early and are stored in Observation.Err.
func Observe(ctx context.Context, src Source, window, interval time.Duration, clock Clock) Observation {
	start := clock.Now()
	var obs Observation
	for clock.Now().Sub(start) < window {
		tip, err := src.FinalizedSlot(ctx)
		if err != nil {
			if IsRateLimit(err) {
				obs.LimitSignals++
			} else {
				obs.Err = err
				obs.Elapsed = clock.Now().Sub(start)
				return obs
			}
		} else {
			if obs.TipSamples == 0 {
				obs.FirstTip = tip
			}
			obs.LastTip = tip
			obs.TipSamples++

			cp, ok, err := src.Checkpoint(ctx)
			if err != nil {
				obs.Err = err
				obs.Elapsed = clock.Now().Sub(start)
				return obs
			}
			if ok {
				var lag uint64
				if tip > cp {
					lag = tip - cp
				}
				obs.LastLag = lag
				if lag > obs.MaxLag {
					obs.MaxLag = lag
				}
				obs.Samples++
			}
		}
		if err := clock.Sleep(ctx, interval); err != nil {
			obs.Err = err
			obs.Elapsed = clock.Now().Sub(start)
			return obs
		}
	}
	obs.Elapsed = clock.Now().Sub(start)
	return obs
}

// ClassifyThroughput compares probe slot/s against tip slot/s from the observe window.
func ClassifyThroughput(p Probe, obs Observation) Gate {
	if p.LimitSignals > 0 {
		return GateInsufficient
	}
	if obs.TipSamples < 2 || p.Elapsed == 0 || obs.Elapsed == 0 {
		return GateUnknown
	}
	tipRate := float64(obs.LastTip-obs.FirstTip) / obs.Elapsed.Seconds()
	probeRate := float64(p.Slots) / p.Elapsed.Seconds()
	if probeRate < tipRate {
		return GateInsufficient
	}
	return GateSufficient
}

// Judge applies AC-076 priority over an Observation and throughput Gate.
func Judge(obs Observation, gate Gate) Verdict {
	if obs.Err != nil {
		return VerdictFail
	}
	if obs.LimitSignals > 0 || gate == GateInsufficient {
		return VerdictRecorded
	}
	if obs.Samples == 0 || obs.LastLag > LagThreshold {
		return VerdictFail
	}
	return VerdictPass
}

// FAStatus maps Judge verdict + gate to FA-04 status strings.
func FAStatus(v Verdict, gate Gate) string {
	switch v {
	case VerdictFail:
		return "failed"
	case VerdictRecorded:
		return "blocked"
	case VerdictPass:
		if gate == GateSufficient {
			return "passed"
		}
		return "blocked"
	default:
		return "failed"
	}
}
