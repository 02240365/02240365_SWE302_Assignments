package shop

// SWE302 Assignment 3 - boundary, invalid-input and regression unit tests.
//
// These tests add to shop_test.go (written for WEB303). Each test name
// starts with its test-case ID from the report (TC03 ... TC15) so the
// results can be traced to the test-case table and the traceability matrix.
// Table-driven style: every row is one input with its expected outcome.

import (
	"math"
	"strings"
	"testing"
)

// TC03 (FR2) - boundary: 20 units of one product is the limit.
func TestTC03_CartQuantityBoundary(t *testing.T) {
	cases := []struct {
		name    string
		have    int32 // units already in the cart
		add     int32
		wantErr Kind
	}{
		{"19 + 1 = 20 (on the limit)", 19, 1, 0},
		{"20 + 1 = 21 (just over)", 20, 1, KindConflict},
		{"0 + 20 (limit in one step)", 0, 20, 0},
		{"0 + 21 (over in one step)", 0, 21, KindConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCarts()
			if tc.have > 0 {
				mustAdd(t, c, "C001", "P003", tc.have)
			}
			_, err := c.Add("C001", "P003", "Wireless Mouse", 1800, tc.add)
			if kindOf(err) != tc.wantErr {
				t.Fatalf("want kind %v, got %v (%v)", tc.wantErr, kindOf(err), err)
			}
		})
	}
}

// TC04 (FR2) - boundary: at most 10 different products in a cart.
func TestTC04_CartLineLimit(t *testing.T) {
	c := NewCarts()
	for i := 0; i < MaxLines; i++ {
		if _, err := c.Add("C001", "P"+string(rune('A'+i)), "x", 10, 1); err != nil {
			t.Fatalf("line %d should be accepted: %v", i+1, err)
		}
	}
	if _, err := c.Add("C001", "PZ", "x", 10, 1); kindOf(err) != KindConflict {
		t.Fatalf("11th product should be refused, got %v", err)
	}
}

// TC05 (FR2) - invalid: a huge quantity must be refused, not wrap around
// to a negative number (regression test for defect D1).
func TestTC05_CartQuantityOverflow(t *testing.T) {
	c := NewCarts()
	mustAdd(t, c, "C001", "P001", 1)
	v, err := c.Add("C001", "P001", "Laptop", 75000, math.MaxInt32)
	if kindOf(err) != KindConflict {
		t.Fatalf("want KindConflict, got err=%v cart=%+v", err, v)
	}
	after, _ := c.Get("C001")
	if after.Lines[0].Quantity != 1 || after.Subtotal != 100 {
		t.Fatalf("cart must be unchanged, got %+v", after)
	}
}

// TC06 (FR4) - boundary: WELCOME10 gives 10% but never more than Nu. 5,000.
func TestTC06_CouponPercentCap(t *testing.T) {
	cases := []struct {
		subtotal, want float64
	}{
		{100, 10},
		{49999, 4999.9},
		{50000, 5000}, // exactly on the cap
		{60000, 5000}, // above the cap
	}
	for _, tc := range cases {
		c := NewCoupons()
		r, err := c.Validate("WELCOME10", "C001", tc.subtotal)
		if err != nil || r.DiscountAmount != tc.want {
			t.Errorf("subtotal %v: want %v, got %v (err %v)", tc.subtotal, tc.want, r.DiscountAmount, err)
		}
	}
}

// TC07 (FR4) - boundary: SAVE500 needs a subtotal of at least Nu. 3,000.
func TestTC07_CouponMinimumSubtotal(t *testing.T) {
	c := NewCoupons()
	if _, err := c.Validate("SAVE500", "C001", 2999.99); kindOf(err) != KindConflict {
		t.Errorf("2999.99 should be refused, got %v", err)
	}
	if r, err := c.Validate("save500 ", "C001", 3000); err != nil || r.DiscountAmount != 500 {
		t.Errorf("3000 should give 500 off, got %v %v", r, err)
	}
}

// TC08 (FR4) - invalid: NaN or infinite subtotals must be refused
// (regression test for defect D5).
func TestTC08_CouponRejectsNonFiniteSubtotal(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := NewCoupons()
		if r, err := c.Validate("WELCOME10", "C001", v); kindOf(err) != KindInvalid {
			t.Errorf("subtotal %v: want KindInvalid, got %+v %v", v, r, err)
		}
	}
	s := NewShipping()
	if _, err := s.GetQuote("Thimphu", 1, math.NaN()); kindOf(err) != KindInvalid {
		t.Errorf("shipping quote with NaN subtotal: want KindInvalid, got %v", err)
	}
}

// TC09 (FR4) - negative: a customer cannot use the same coupon twice,
// but another customer still can.
func TestTC09_CouponSingleUse(t *testing.T) {
	c := NewCoupons()
	if err := c.Redeem("WELCOME10", "C001", "ORD-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Validate("WELCOME10", "C001", 1000); kindOf(err) != KindConflict {
		t.Errorf("second use by C001 should be refused, got %v", err)
	}
	if err := c.Redeem("WELCOME10", "C001", "ORD-2"); kindOf(err) != KindExists {
		t.Errorf("second redeem by C001 should be KindExists, got %v", err)
	}
	if _, err := c.Validate("WELCOME10", "C002", 1000); err != nil {
		t.Errorf("C002 should still be able to use it, got %v", err)
	}
}

// TC10 (FR5) - boundary: free shipping starts at exactly Nu. 5,000.
func TestTC10_FreeShippingThreshold(t *testing.T) {
	s := NewShipping()
	below, _ := s.GetQuote("Thimphu, Bhutan", 1, 4999.99)
	at, _ := s.GetQuote("Thimphu, Bhutan", 1, 5000)
	if below.Fee != 100 || below.FreeShipping {
		t.Errorf("4999.99: want fee 100, got %+v", below)
	}
	if at.Fee != 0 || !at.FreeShipping || at.EtaDays != 1 {
		t.Errorf("5000: want free with 1-day ETA, got %+v", at)
	}
}

// TC11 / TC12 (FR5) - zone detection. TC11: the official spelling
// "Phuntsholing" must be a border town (defect D2). TC12: a word that only
// contains "paro" must not be treated as Paro (defect D3).
func TestTC11_TC12_ShippingZones(t *testing.T) {
	cases := []struct {
		address, zone string
		fee           float64
	}{
		{"Thimphu, Bhutan", "WESTERN", 100},
		{"PARO", "WESTERN", 100},
		{"Phuentsholing, Bhutan", "BORDER", 150},
		{"Phuntsholing, Chukha", "BORDER", 150}, // TC11
		{"Gelephu", "BORDER", 150},
		{"Sparo Lane, Mongar", "REMOTE", 250}, // TC12
		{"Samdrup Jongkhar", "REMOTE", 250},
	}
	s := NewShipping()
	for _, tc := range cases {
		q, err := s.GetQuote(tc.address, 1, 1000)
		if err != nil || q.Zone != tc.zone || q.Fee != tc.fee {
			t.Errorf("%q: want %s/%v, got %s/%v (err %v)", tc.address, tc.zone, tc.fee, q.Zone, q.Fee, err)
		}
	}
}

// TC13 (FR7) - boundary: rating 1..5 and comment 1..200 characters.
func TestTC13_ReviewBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		rating  int32
		comment string
		wantErr Kind
	}{
		{"rating 0", 0, "ok", KindInvalid},
		{"rating 1", 1, "ok", 0},
		{"rating 5", 5, "ok", 0},
		{"rating 6", 6, "ok", KindInvalid},
		{"comment only spaces", 3, "   ", KindInvalid},
		{"comment 200 chars", 3, strings.Repeat("a", 200), 0},
		{"comment 201 chars", 3, strings.Repeat("a", 201), KindInvalid},
	}
	for i, tc := range cases {
		r := NewReviews()
		_, err := r.Add("P001", "C00"+string(rune('1'+i)), "Name", tc.rating, tc.comment)
		if kindOf(err) != tc.wantErr {
			t.Errorf("%s: want %v, got %v (%v)", tc.name, tc.wantErr, kindOf(err), err)
		}
	}
}

// TC14 (FR7) - negative: one review per customer per product.
func TestTC14_DuplicateReview(t *testing.T) {
	r := NewReviews()
	if _, err := r.Add("P001", "C001", "Sonam", 4, "Good"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add("P001", " C001 ", "Sonam", 5, "Again"); kindOf(err) != KindExists {
		t.Fatalf("second review should be KindExists, got %v", err)
	}
}

// TC15 - boundary: money must round half up to two decimals
// (regression test for defect D4).
func TestTC15_MoneyRounding(t *testing.T) {
	cases := map[float64]float64{1.005: 1.01, 0.285: 0.29, 2.675: 2.68, 1234.565: 1234.57, -1.005: -1.01, 75000: 75000}
	for in, want := range cases {
		if got := round2(in); got != want {
			t.Errorf("round2(%v) = %v, want %v", in, got, want)
		}
	}
}

func mustAdd(t *testing.T, c *Carts, customer, product string, qty int32) {
	t.Helper()
	if _, err := c.Add(customer, product, "x", 100, qty); err != nil {
		t.Fatalf("setup add failed: %v", err)
	}
}
