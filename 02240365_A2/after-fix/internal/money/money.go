// Package money holds the one rounding rule used everywhere prices are
// calculated (cart, coupons, discounts, checkout). Before SWE302 the same
// round2 function was copied into internal/shop and ui-service, and both
// copies rounded 1.005 down to 1.00 because of binary floating point.
package money

import (
	"math"
	"math/big"
	"strconv"
)

// Round2 rounds v to two decimal places, half away from zero, using the
// decimal value a person sees (1.005 -> 1.01, 0.285 -> 0.29, -1.005 -> -1.01).
// NaN and infinities are returned unchanged so callers can reject them.
func Round2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	// The shortest decimal text for v ("1.005") is exact as a rational number.
	r, _ := new(big.Rat).SetString(strconv.FormatFloat(v, 'f', -1, 64))
	r.Mul(r, big.NewRat(100, 1))
	q, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if twice := new(big.Int).Mul(new(big.Int).Abs(rem), big.NewInt(2)); twice.Cmp(r.Denom()) >= 0 {
		q.Add(q, big.NewInt(int64(r.Num().Sign()))) // round the half away from zero
	}
	f, _ := new(big.Rat).SetFrac(q, big.NewInt(100)).Float64()
	return f
}

// IsValidAmount reports whether v is a usable money amount: a finite
// number greater than zero.
func IsValidAmount(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0
}
