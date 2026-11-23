package lagcheck

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
)

func TestIsRateLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain", err: errors.New("boom"), want: false},
		{
			name: "permanent_value",
			err:  provider.Error{Kind: provider.ErrorKindPermanent, HTTPStatus: 400},
			want: false,
		},
		{
			name: "transient_value",
			err:  provider.Error{Kind: provider.ErrorKindTransient, HTTPStatus: 0},
			want: true,
		},
		{
			name: "http_429_value",
			err:  provider.Error{Kind: provider.ErrorKindPermanent, HTTPStatus: 429},
			want: true,
		},
		{
			name: "transient_429_value",
			err:  provider.Error{Kind: provider.ErrorKindTransient, HTTPStatus: 429},
			want: true,
		},
		{
			name: "transient_pointer",
			err:  &provider.Error{Kind: provider.ErrorKindTransient, HTTPStatus: 503},
			want: true,
		},
		{
			name: "wrapped_429",
			err:  fmt.Errorf("wrap: %w", provider.Error{Kind: provider.ErrorKindRPC, HTTPStatus: 429}),
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsRateLimit(tc.err); got != tc.want {
				t.Fatalf("IsRateLimit(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestClassifyThroughput(t *testing.T) {
	t.Parallel()
	obsOK := Observation{
		TipSamples: 2,
		FirstTip:   1000,
		LastTip:    1100,
		Elapsed:    10 * time.Second, // tip rate = 10 slot/s
	}
	cases := []struct {
		name string
		p    Probe
		obs  Observation
		want Gate
	}{
		{
			name: "probe_limit_signals",
			p:    Probe{Slots: 40, Elapsed: time.Second, LimitSignals: 1},
			obs:  obsOK,
			want: GateInsufficient,
		},
		{
			name: "probe_slower_than_tip",
			p:    Probe{Slots: 40, Elapsed: 10 * time.Second}, // 4 slot/s < 10
			obs:  obsOK,
			want: GateInsufficient,
		},
		{
			name: "probe_ge_tip",
			p:    Probe{Slots: 100, Elapsed: 10 * time.Second}, // 10 slot/s == tip
			obs:  obsOK,
			want: GateSufficient,
		},
		{
			name: "probe_faster_than_tip",
			p:    Probe{Slots: 200, Elapsed: 10 * time.Second}, // 20 > 10
			obs:  obsOK,
			want: GateSufficient,
		},
		{
			name: "tip_samples_lt_2",
			p:    Probe{Slots: 40, Elapsed: time.Second},
			obs:  Observation{TipSamples: 1, FirstTip: 1, LastTip: 1, Elapsed: time.Second},
			want: GateUnknown,
		},
		{
			name: "probe_elapsed_zero",
			p:    Probe{Slots: 40, Elapsed: 0},
			obs:  obsOK,
			want: GateUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyThroughput(tc.p, tc.obs); got != tc.want {
				t.Fatalf("ClassifyThroughput=%s want %s", got, tc.want)
			}
		})
	}
}

// Acceptance ①②②′③④⑤⑥⑦⑧ via Observe + Judge + FAStatus.
func TestJudgeAcceptanceBranches(t *testing.T) {
	t.Parallel()
	interval := time.Second
	window := 5 * interval // 5 ticks (n=0..4)

	cases := []struct {
		name       string
		src        Source
		gate       Gate
		wantV      Verdict
		wantFA     string
		wantLimMin int // optional lower bound on LimitSignals
		checkLag   bool
		wantLast   uint64
	}{
		{
			// ① tip +1 / step, checkpoint=tip−10, gate sufficient → pass/passed
			name:   "01_catch_up_sufficient",
			src:    stepSource(1000, 1, 10, 1, nil, nil),
			gate:   GateSufficient,
			wantV:  VerdictPass,
			wantFA: "passed",
		},
		{
			// ② tip +50, checkpoint +5 → last_lag>150, gate sufficient → fail/failed
			name:     "02_lag_exceed_sufficient",
			src:      stepSource(1000, 50, 10, 5, nil, nil),
			gate:     GateSufficient,
			wantV:    VerdictFail,
			wantFA:   "failed",
			checkLag: true,
			wantLast: 190, // 10+45*4
		},
		{
			// ②′ same lag, gate unknown → still fail (AUD-028-02)
			name:   "02p_lag_exceed_unknown_not_recorded",
			src:    stepSource(1000, 50, 10, 5, nil, nil),
			gate:   GateUnknown,
			wantV:  VerdictFail,
			wantFA: "failed",
		},
		{
			// ③ same growth, some ticks 429 → recorded/blocked
			name: "03_rate_limit_429_recorded",
			src: stepSource(1000, 50, 10, 5, map[int]error{
				1: provider.Error{Kind: provider.ErrorKindTransient, HTTPStatus: 429},
				3: provider.Error{Kind: provider.ErrorKindTransient, HTTPStatus: 429},
			}, nil),
			gate:       GateSufficient,
			wantV:      VerdictRecorded,
			wantFA:     "blocked",
			wantLimMin: 2,
		},
		{
			// ④ permanent RPC error → fail
			name: "04_permanent_rpc_fail",
			src: stepSource(1000, 1, 10, 1, map[int]error{
				2: provider.Error{Kind: provider.ErrorKindPermanent, HTTPStatus: 500},
			}, nil),
			gate:   GateSufficient,
			wantV:  VerdictFail,
			wantFA: "failed",
		},
		{
			// ⑤ checkpoint error → fail
			name: "05_checkpoint_error_fail",
			src: stepSource(1000, 1, 10, 1, nil, map[int]error{
				1: errors.New("db down"),
			}),
			gate:   GateSufficient,
			wantV:  VerdictFail,
			wantFA: "failed",
		},
		{
			// ⑦ lag OK, gate insufficient → recorded/blocked
			name:   "07_lag_ok_insufficient_recorded",
			src:    stepSource(1000, 1, 10, 1, nil, nil),
			gate:   GateInsufficient,
			wantV:  VerdictRecorded,
			wantFA: "blocked",
		},
		{
			// ⑧ lag OK, gate unknown → pass/blocked
			name:   "08_lag_ok_unknown_pass_blocked",
			src:    stepSource(1000, 1, 10, 1, nil, nil),
			gate:   GateUnknown,
			wantV:  VerdictPass,
			wantFA: "blocked",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			obs := Observe(context.Background(), tc.src, window, interval, fakeClock())
			if tc.wantLimMin > 0 && obs.LimitSignals < tc.wantLimMin {
				t.Fatalf("LimitSignals=%d want >= %d", obs.LimitSignals, tc.wantLimMin)
			}
			if tc.checkLag && obs.LastLag != tc.wantLast {
				t.Fatalf("LastLag=%d want %d (samples=%d max=%d)", obs.LastLag, tc.wantLast, obs.Samples, obs.MaxLag)
			}
			got := Judge(obs, tc.gate)
			if got != tc.wantV {
				t.Fatalf("Judge=%s want %s (samples=%d last=%d lim=%d err=%v)",
					got, tc.wantV, obs.Samples, obs.LastLag, obs.LimitSignals, obs.Err)
			}
			if fa := FAStatus(got, tc.gate); fa != tc.wantFA {
				t.Fatalf("FAStatus=%s want %s", fa, tc.wantFA)
			}
		})
	}
}

func TestJudgeNoSamples(t *testing.T) {
	t.Parallel()
	t.Run("06_no_samples_no_limit_fail", func(t *testing.T) {
		t.Parallel()
		// Checkpoint never ready → Samples==0, no limit signals.
		tick := 0
		src := Source{
			FinalizedSlot: func(context.Context) (uint64, error) {
				tick++
				return 1000 + uint64(tick), nil
			},
			Checkpoint: func(context.Context) (uint64, bool, error) {
				return 0, false, nil
			},
		}
		obs := Observe(context.Background(), src, 3*time.Second, time.Second, fakeClock())
		if obs.Samples != 0 {
			t.Fatalf("Samples=%d want 0", obs.Samples)
		}
		if obs.LimitSignals != 0 {
			t.Fatalf("LimitSignals=%d want 0", obs.LimitSignals)
		}
		if got := Judge(obs, GateSufficient); got != VerdictFail {
			t.Fatalf("Judge=%s want fail", got)
		}
		if fa := FAStatus(VerdictFail, GateSufficient); fa != "failed" {
			t.Fatalf("FAStatus=%s want failed", fa)
		}
	})
	t.Run("06_no_samples_with_429_recorded", func(t *testing.T) {
		t.Parallel()
		src := Source{
			FinalizedSlot: func(context.Context) (uint64, error) {
				return 0, provider.Error{Kind: provider.ErrorKindTransient, HTTPStatus: 429}
			},
			Checkpoint: func(context.Context) (uint64, bool, error) {
				t.Fatal("Checkpoint must not run after tip rate-limit")
				return 0, false, nil
			},
		}
		obs := Observe(context.Background(), src, 3*time.Second, time.Second, fakeClock())
		if obs.Samples != 0 {
			t.Fatalf("Samples=%d want 0", obs.Samples)
		}
		if obs.LimitSignals == 0 {
			t.Fatal("LimitSignals want > 0")
		}
		if got := Judge(obs, GateSufficient); got != VerdictRecorded {
			t.Fatalf("Judge=%s want recorded", got)
		}
		if fa := FAStatus(VerdictRecorded, GateSufficient); fa != "blocked" {
			t.Fatalf("FAStatus=%s want blocked", fa)
		}
	})
}

func TestLagThresholdConstant(t *testing.T) {
	t.Parallel()
	if LagThreshold != 150 {
		t.Fatalf("LagThreshold=%d want 150", LagThreshold)
	}
}

// stepSource: tip = tip0 + tipStep*n; checkpoint = tip0 - lag0 + cpStep*n (when found).
// tipErrs / cpErrs keyed by tick index (0-based) override the call for that tick.
func stepSource(
	tip0 uint64,
	tipStep uint64,
	lag0 uint64,
	cpStep uint64,
	tipErrs map[int]error,
	cpErrs map[int]error,
) Source {
	n := 0
	return Source{
		FinalizedSlot: func(context.Context) (uint64, error) {
			i := n
			if err, ok := tipErrs[i]; ok {
				n++
				return 0, err
			}
			tip := tip0 + tipStep*uint64(i)
			n++
			return tip, nil
		},
		Checkpoint: func(context.Context) (uint64, bool, error) {
			// n already advanced by FinalizedSlot for this tick
			i := n - 1
			if err, ok := cpErrs[i]; ok {
				return 0, false, err
			}
			cp := tip0 - lag0 + cpStep*uint64(i)
			return cp, true, nil
		},
	}
}

func fakeClock() Clock {
	now := time.Unix(1_700_000_000, 0).UTC()
	return Clock{
		Now: func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error {
			now = now.Add(d)
			return nil
		},
	}
}
