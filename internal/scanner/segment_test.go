package scanner

import (
	"math"
	"testing"
)

func TestHandoffWindow(t *testing.T) {
	// AC-S03
	t.Parallel()

	cases := []struct {
		name                   string
		handed                 uint64
		hasCursor              bool
		start, step, finalized uint64
		wantPrev, wantNext     uint64
		wantRight              uint64
		wantOK                 bool
	}{
		{name: "cursor_full_step", handed: 1000, hasCursor: true, step: 20000, finalized: 30000,
			wantPrev: 1000, wantNext: 1001, wantRight: 21000, wantOK: true},
		{name: "no_cursor_caught_up", start: 500, step: 20000, finalized: 400},
		{name: "finalized_caps_right", handed: 1000, hasCursor: true, step: 20000, finalized: 1500,
			wantPrev: 1000, wantNext: 1001, wantRight: 1500, wantOK: true},
		{name: "right_overflow", handed: math.MaxUint64 - 5, hasCursor: true, step: 20000, finalized: math.MaxUint64 - 1,
			wantPrev: math.MaxUint64 - 5, wantNext: math.MaxUint64 - 4, wantRight: math.MaxUint64 - 1, wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prev, next, right, ok := handoffWindow(tc.handed, tc.hasCursor, tc.start, tc.step, tc.finalized)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if prev != tc.wantPrev || next != tc.wantNext || right != tc.wantRight {
				t.Fatalf("(prev,next,right) = (%d,%d,%d), want (%d,%d,%d)",
					prev, next, right, tc.wantPrev, tc.wantNext, tc.wantRight)
			}
		})
	}
}

func TestSegments(t *testing.T) {
	t.Parallel()

	t.Run("AC084_40_segments", func(t *testing.T) {
		t.Parallel()
		got := Segments(1001, 21000, 500)
		if len(got) != 40 {
			t.Fatalf("len(Segments) = %d, want 40", len(got))
		}
		if got[0] != [2]uint64{1001, 1500} {
			t.Fatalf("first = %v, want [1001 1500]", got[0])
		}
		if got[39] != [2]uint64{20501, 21000} {
			t.Fatalf("last = %v, want [20501 21000]", got[39])
		}
		for i, seg := range got {
			if seg[0] > seg[1] {
				t.Fatalf("seg[%d] inverted: %v", i, seg)
			}
			if i < len(got)-1 {
				wantLen := uint64(500)
				gotLen := seg[1] - seg[0] + 1
				if gotLen != wantLen {
					t.Fatalf("seg[%d] length = %d, want %d", i, gotLen, wantLen)
				}
				if got[i+1][0] != seg[1]+1 {
					t.Fatalf("seg[%d] does not abut next: %v then %v", i, seg, got[i+1])
				}
			}
		}
	})

	t.Run("AC085_short_last", func(t *testing.T) {
		t.Parallel()
		got := Segments(1001, 1800, 500)
		want := [][2]uint64{{1001, 1500}, {1501, 1800}}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d; got %v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("seg[%d] = %v, want %v", i, got[i], want[i])
			}
		}
	})
}
