package shop

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// kindOf returns the Kind of a business error, or 0 for nil/other errors.
func kindOf(err error) Kind {
	var se *Error
	if errors.As(err, &se) {
		return se.Kind
	}
	return 0
}

// ---------------- Cart ----------------

func TestCart_AddMergesSameProductAndTotals(t *testing.T) {
	c := NewCarts()
	c.Add("C001", "P003", "Wireless Mouse", 1800, 1)
	v, err := c.Add("C001", "P003", "Wireless Mouse", 1800, 2)
	if err != nil || len(v.Lines) != 1 || v.Lines[0].Quantity != 3 {
		t.Fatalf("want one line of 3, got %+v err=%v", v, err)
	}
	if v.ItemCount != 3 || v.Subtotal != 5400 {
		t.Fatalf("want 3 items / 5400, got %d / %v", v.ItemCount, v.Subtotal)
	}
}

func TestCart_InvalidInput(t *testing.T) {
	c := NewCarts()
	for name, fn := range map[string]func() error{
		"empty customer": func() error { _, e := c.Add("", "P001", "x", 1, 1); return e },
		"empty product":  func() error { _, e := c.Add("C001", "", "x", 1, 1); return e },
		"zero quantity":  func() error { _, e := c.Add("C001", "P001", "x", 1, 0); return e },
		"negative qty":   func() error { _, e := c.Add("C001", "P001", "x", 1, -2); return e },
		"zero price":     func() error { _, e := c.Add("C001", "P001", "x", 0, 1); return e },
	} {
		if kindOf(fn()) != KindInvalid {
			t.Errorf("%s: want KindInvalid", name)
		}
	}
}

func TestCart_LimitsPerLineAndLines(t *testing.T) {
	c := NewCarts()
	if _, err := c.Add("C001", "P001", "x", 10, MaxQtyPerLine); err != nil {
		t.Fatal(err)
	}
	if kindOf(addErr(c, "C001", "P001", 1)) != KindConflict {
		t.Error("21st unit of one product should be refused")
	}
	for i := 0; i < MaxLines-1; i++ {
		c.Add("C002", string(rune('A'+i)), "x", 10, 1)
	}
	if _, err := c.Add("C002", "Z", "x", 10, 1); err != nil {
		t.Fatalf("10th line should be allowed: %v", err)
	}
	if kindOf(addErr(c, "C002", "ZZ", 1)) != KindConflict {
		t.Error("11th different product should be refused")
	}
}

func addErr(c *Carts, cust, prod string, q int32) error {
	_, e := c.Add(cust, prod, "x", 10, q)
	return e
}

func TestCart_RemoveClearAndIsolation(t *testing.T) {
	c := NewCarts()
	c.Add("C001", "P001", "Laptop", 75000, 1)
	c.Add("C002", "P002", "Keyboard", 4500, 1)
	if _, err := c.Remove("C001", "P999"); kindOf(err) != KindNotFound {
		t.Error("removing an absent product should be NotFound")
	}
	if v, _ := c.Remove("C001", "P001"); len(v.Lines) != 0 {
		t.Error("cart should be empty after remove")
	}
	if v, _ := c.Get("C002"); len(v.Lines) != 1 {
		t.Error("another customer's cart must not change")
	}
	if v, _ := c.Clear("C002"); len(v.Lines) != 0 {
		t.Error("clear should empty the cart")
	}
	if v, err := c.Get("C003"); err != nil || len(v.Lines) != 0 {
		t.Error("unknown customer has an empty cart, not an error")
	}
}

// ---------------- Review ----------------

func TestReview_AddAndAverage(t *testing.T) {
	r := NewReviews()
	r.Add("P001", "C001", "Sonam", 5, "Great")
	r.Add("P001", "C002", "Choki", 4, "Good")
	l, _ := r.List("P001")
	if l.ReviewCount != 2 || l.AverageRating != 4.5 {
		t.Fatalf("want 2 reviews avg 4.5, got %+v", l)
	}
	if l.Reviews[0].CustomerID != "C002" {
		t.Error("newest review should come first")
	}
	if l2, _ := r.List("P002"); l2.ReviewCount != 0 || l2.AverageRating != 0 {
		t.Error("product with no reviews should show zero")
	}
}

func TestReview_Validation(t *testing.T) {
	r := NewReviews()
	cases := []struct {
		name   string
		rating int32
		text   string
		kind   Kind
	}{
		{"rating 0", 0, "ok", KindInvalid},
		{"rating 6", 6, "ok", KindInvalid},
		{"empty comment", 3, "", KindInvalid},
		{"spaces only", 3, "   ", KindInvalid},
		{"too long", 3, strings.Repeat("a", MaxCommentLength+1), KindInvalid},
	}
	for _, tc := range cases {
		_, err := r.Add("P001", "C001", "S", tc.rating, tc.text)
		if kindOf(err) != tc.kind {
			t.Errorf("%s: got %v", tc.name, err)
		}
	}
	if _, err := r.Add("P001", "C001", "S", 3, strings.Repeat("a", MaxCommentLength)); err != nil {
		t.Errorf("200 characters is allowed: %v", err)
	}
}

func TestReview_OneReviewPerCustomerPerProduct(t *testing.T) {
	r := NewReviews()
	r.Add("P001", "C001", "S", 5, "first")
	if _, err := r.Add("P001", "C001", "S", 1, "second"); kindOf(err) != KindExists {
		t.Errorf("second review by the same customer should be AlreadyExists, got %v", err)
	}
	if _, err := r.Add("P002", "C001", "S", 4, "other product"); err != nil {
		t.Errorf("same customer may review another product: %v", err)
	}
}

func TestReview_CommentCountsCharactersNotBytes(t *testing.T) {
	r := NewReviews()
	if _, err := r.Add("P001", "C001", "S", 5, strings.Repeat("ཀ", MaxCommentLength)); err != nil {
		t.Errorf("200 Dzongkha characters should be accepted: %v", err)
	}
}

// ---------------- Shipping ----------------

func TestShipping_QuoteZonesAndFree(t *testing.T) {
	s := NewShipping()
	cases := []struct {
		addr string
		sub  float64
		fee  float64
		eta  int32
		free bool
		zone string
	}{
		{"Thimphu, Bhutan", 1000, 100, 1, false, "WESTERN"},
		{"paro, bhutan", 1000, 100, 1, false, "WESTERN"},
		{"Phuentsholing, Bhutan", 1000, 150, 2, false, "BORDER"},
		{"Trashigang", 1000, 250, 4, false, "REMOTE"},
		{"Thimphu", 4999.99, 100, 1, false, "WESTERN"},
		{"Thimphu", 5000, 0, 1, true, "WESTERN"},
		{"Trashigang", 75000, 0, 4, true, "REMOTE"},
	}
	for _, c := range cases {
		q, err := s.GetQuote(c.addr, 1, c.sub)
		if err != nil || q.Fee != c.fee || q.EtaDays != c.eta || q.FreeShipping != c.free || q.Zone != c.zone {
			t.Errorf("%s/%.2f: got %+v err=%v", c.addr, c.sub, q, err)
		}
	}
}

func TestShipping_QuoteInvalid(t *testing.T) {
	s := NewShipping()
	for _, f := range []func() error{
		func() error { _, e := s.GetQuote("", 1, 100); return e },
		func() error { _, e := s.GetQuote("   ", 1, 100); return e },
		func() error { _, e := s.GetQuote("Thimphu", 0, 100); return e },
		func() error { _, e := s.GetQuote("Thimphu", 1, 0); return e },
	} {
		if kindOf(f()) != KindInvalid {
			t.Error("expected KindInvalid")
		}
	}
}

func TestShipping_CreateAndTrack(t *testing.T) {
	s := NewShipping()
	sh, err := s.Create("ORD-1", "C001", "Thimphu, Bhutan")
	if err != nil || sh.TrackingID != "TRK-00001" || sh.Status != "PROCESSING" {
		t.Fatalf("unexpected %+v %v", sh, err)
	}
	got, err := s.Track("trk-00001 ")
	if err != nil || got.OrderID != "ORD-1" || len(got.Events) != 1 {
		t.Fatalf("track failed: %+v %v", got, err)
	}
	if _, err := s.Track("TRK-99999"); kindOf(err) != KindNotFound {
		t.Error("unknown tracking ID should be NotFound")
	}
	if _, err := s.Track(""); kindOf(err) != KindInvalid {
		t.Error("empty tracking ID should be Invalid")
	}
	if _, err := s.Create("ORD-1", "C001", "Thimphu"); kindOf(err) != KindExists {
		t.Error("second shipment for the same order should be AlreadyExists")
	}
	if _, err := s.Create("ORD-2", "C001", ""); kindOf(err) != KindInvalid {
		t.Error("missing address should be Invalid")
	}
}

// ---------------- Coupon ----------------

func TestCoupon_PercentFlatCapAndCase(t *testing.T) {
	c := NewCoupons()
	r, err := c.Validate(" welcome10 ", "C001", 2000)
	if err != nil || r.Code != "WELCOME10" || r.DiscountAmount != 200 {
		t.Fatalf("10%% of 2000 should be 200, got %+v %v", r, err)
	}
	if r, _ := c.Validate("WELCOME10", "C001", 100000); r.DiscountAmount != 5000 {
		t.Errorf("percentage coupon is capped at 5000, got %v", r.DiscountAmount)
	}
	if r, err := c.Validate("SAVE500", "C001", 3000); err != nil || r.DiscountAmount != 500 {
		t.Errorf("SAVE500 on 3000: %+v %v", r, err)
	}
}

func TestCoupon_Refusals(t *testing.T) {
	c := NewCoupons()
	if _, err := c.Validate("NOPE", "C001", 1000); kindOf(err) != KindNotFound {
		t.Error("unknown code -> NotFound")
	}
	if _, err := c.Validate("OLD20", "C001", 1000); kindOf(err) != KindConflict || !strings.Contains(err.Error(), "expired") {
		t.Errorf("expired code -> Conflict mentioning expired, got %v", err)
	}
	if _, err := c.Validate("SAVE500", "C001", 2999.99); kindOf(err) != KindConflict {
		t.Error("below the minimum -> Conflict")
	}
	if _, err := c.Validate("", "C001", 1000); kindOf(err) != KindInvalid {
		t.Error("empty code -> Invalid")
	}
	if _, err := c.Validate("WELCOME10", "C001", 0); kindOf(err) != KindInvalid {
		t.Error("zero subtotal -> Invalid")
	}
}

func TestCoupon_OneUsePerCustomer(t *testing.T) {
	c := NewCoupons()
	if err := c.Redeem("WELCOME10", "C001", "ORD-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Validate("WELCOME10", "C001", 1000); kindOf(err) != KindConflict {
		t.Error("used coupon should not validate again for the same customer")
	}
	if _, err := c.Validate("WELCOME10", "C002", 1000); err != nil {
		t.Errorf("another customer may still use it: %v", err)
	}
	if err := c.Redeem("WELCOME10", "C001", "ORD-2"); kindOf(err) != KindExists {
		t.Error("double redeem -> AlreadyExists")
	}
	c.Reset()
	if _, err := c.Validate("WELCOME10", "C001", 1000); err != nil {
		t.Error("reset should clear redemptions")
	}
}

func TestCoupon_ExpiryUsesClock(t *testing.T) {
	c := NewCoupons()
	c.now = func() time.Time { return time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC) }
	if _, err := c.Validate("OLD20", "C001", 1000); err != nil {
		t.Errorf("OLD20 is valid before its expiry date: %v", err)
	}
}

// ---------------- History ----------------

func TestHistory_RecordListGet(t *testing.T) {
	h := NewHistory()
	h.Record(Order{OrderID: "O1", CustomerID: "C001", ProductID: "P1", Quantity: 1, Total: 10})
	h.Record(Order{OrderID: "O2", CustomerID: "C001", ProductID: "P2", Quantity: 2, Total: 20})
	h.Record(Order{OrderID: "O3", CustomerID: "C002", ProductID: "P1", Quantity: 1, Total: 10})
	l, _ := h.List("C001")
	if len(l) != 2 || l[0].OrderID != "O2" {
		t.Fatalf("want newest-first [O2 O1], got %+v", l)
	}
	if none, _ := h.List("C003"); len(none) != 0 {
		t.Error("customer with no orders gets an empty list")
	}
	if o, err := h.Get("O3"); err != nil || o.CustomerID != "C002" {
		t.Error("get O3 failed")
	}
	if _, err := h.Get("O9"); kindOf(err) != KindNotFound {
		t.Error("unknown order -> NotFound")
	}
}

func TestHistory_Validation(t *testing.T) {
	h := NewHistory()
	if _, err := h.Record(Order{OrderID: "", CustomerID: "C1", ProductID: "P"}); kindOf(err) != KindInvalid {
		t.Error("missing order id -> Invalid")
	}
	if _, err := h.Record(Order{OrderID: "O1", CustomerID: "C1", ProductID: "P", Quantity: 0}); kindOf(err) != KindInvalid {
		t.Error("zero quantity -> Invalid")
	}
	h.Record(Order{OrderID: "O1", CustomerID: "C1", ProductID: "P", Quantity: 1})
	if _, err := h.Record(Order{OrderID: "O1", CustomerID: "C1", ProductID: "P", Quantity: 1}); kindOf(err) != KindExists {
		t.Error("duplicate order id -> AlreadyExists")
	}
}
