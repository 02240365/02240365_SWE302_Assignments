package shop

import (
	"strings"
	"sync"
	"time"
)

type couponRule struct {
	Code        string
	Percent     float64 // percentage off, or 0
	Flat        float64 // flat amount off, or 0
	MaxDiscount float64 // cap for percentage coupons, 0 = no cap
	MinSubtotal float64
	Expires     time.Time // zero = never
	Description string
}

// CouponResult is what a valid coupon is worth on a given subtotal.
type CouponResult struct {
	Code           string
	DiscountAmount float64
	Description    string
}

// Coupons validates and redeems coupon codes. Each customer can use each
// coupon once.
type Coupons struct {
	mu       sync.Mutex
	rules    map[string]couponRule
	redeemed map[string]string // "CODE|customer" -> order ID
	now      func() time.Time
}

func NewCoupons() *Coupons {
	rules := []couponRule{
		{Code: "WELCOME10", Percent: 10, MaxDiscount: 5000, Description: "10% off (up to Nu. 5,000)"},
		{Code: "SAVE500", Flat: 500, MinSubtotal: 3000, Description: "Nu. 500 off orders of Nu. 3,000 or more"},
		{Code: "OLD20", Percent: 20, Expires: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), Description: "20% off (expired)"},
	}
	m := map[string]couponRule{}
	for _, r := range rules {
		m[r.Code] = r
	}
	return &Coupons{rules: m, redeemed: map[string]string{}, now: time.Now}
}

func normalise(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// Validate checks a coupon without using it up.
func (c *Coupons) Validate(code, customerID string, subtotal float64) (CouponResult, error) {
	code, customerID = normalise(code), strings.TrimSpace(customerID)
	if code == "" {
		return CouponResult{}, invalid("please enter a coupon code")
	}
	if customerID == "" {
		return CouponResult{}, invalid("customer_id is required")
	}
	if subtotal <= 0 {
		return CouponResult{}, invalid("subtotal must be greater than zero")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	rule, ok := c.rules[code]
	if !ok {
		return CouponResult{}, notFound("coupon %s does not exist", code)
	}
	if !rule.Expires.IsZero() && c.now().After(rule.Expires) {
		return CouponResult{}, conflict("coupon %s has expired", code)
	}
	if _, used := c.redeemed[code+"|"+customerID]; used {
		return CouponResult{}, conflict("coupon %s has already been used by customer %s", code, customerID)
	}
	if subtotal < rule.MinSubtotal {
		return CouponResult{}, conflict("coupon %s needs an order of at least Nu. %.0f", code, rule.MinSubtotal)
	}
	amount := rule.Flat
	if rule.Percent > 0 {
		amount = round2(subtotal * rule.Percent / 100)
		if rule.MaxDiscount > 0 && amount > rule.MaxDiscount {
			amount = rule.MaxDiscount
		}
	}
	if amount > subtotal {
		amount = subtotal
	}
	return CouponResult{Code: code, DiscountAmount: round2(amount), Description: rule.Description}, nil
}

// Redeem marks a coupon as used by a customer for an order. It runs only
// after the payment succeeded.
func (c *Coupons) Redeem(code, customerID, orderID string) error {
	code, customerID, orderID = normalise(code), strings.TrimSpace(customerID), strings.TrimSpace(orderID)
	if code == "" || customerID == "" || orderID == "" {
		return invalid("code, customer_id and order_id are required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.rules[code]; !ok {
		return notFound("coupon %s does not exist", code)
	}
	key := code + "|" + customerID
	if _, used := c.redeemed[key]; used {
		return exists("coupon %s has already been used by customer %s", code, customerID)
	}
	c.redeemed[key] = orderID
	return nil
}

func (c *Coupons) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.redeemed = map[string]string{}
}
