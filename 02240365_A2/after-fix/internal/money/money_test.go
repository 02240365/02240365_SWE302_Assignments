package money

import (
	"math"
	"testing"
)

func TestRound2(t *testing.T) {
	cases := map[float64]float64{
		0: 0, 1.005: 1.01, 0.285: 0.29, 2.675: 2.68, 1234.565: 1234.57,
		-1.005: -1.01, 8550.004: 8550, 75000: 75000, 4999.995: 5000,
	}
	for in, want := range cases {
		if got := Round2(in); got != want {
			t.Errorf("Round2(%v) = %v, want %v", in, got, want)
		}
	}
	if !math.IsNaN(Round2(math.NaN())) {
		t.Error("NaN should pass through")
	}
}

func TestIsValidAmount(t *testing.T) {
	for v, want := range map[float64]bool{1: true, 0.01: true, 0: false, -5: false, math.Inf(1): false} {
		if IsValidAmount(v) != want {
			t.Errorf("IsValidAmount(%v) want %v", v, want)
		}
	}
	if IsValidAmount(math.NaN()) {
		t.Error("NaN is not a valid amount")
	}
}
