package shop

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Order is one completed order, as kept by the Order History Service.
type Order struct {
	OrderID       string
	CustomerID    string
	ProductID     string
	ProductName   string
	Quantity      int32
	Subtotal      float64 // price x quantity, before any discount
	DiscountTotal float64 // quantity discount + coupon
	ShippingFee   float64
	Total         float64 // what the customer paid
	PaymentID     string
	TrackingID    string
	CouponCode    string
	CreatedAt     string
	seq           int
}

// History stores completed orders.
type History struct {
	mu     sync.Mutex
	orders map[string]Order
	seq    int
	now    func() time.Time
}

func NewHistory() *History { return &History{orders: map[string]Order{}, now: time.Now} }

// Record stores a completed order. Order IDs must be unique.
func (h *History) Record(o Order) (Order, error) {
	o.OrderID, o.CustomerID = strings.TrimSpace(o.OrderID), strings.TrimSpace(o.CustomerID)
	if o.OrderID == "" || o.CustomerID == "" || o.ProductID == "" {
		return Order{}, invalid("order_id, customer_id and product_id are required")
	}
	if o.Quantity <= 0 {
		return Order{}, invalid("quantity must be at least 1")
	}
	if o.Total < 0 {
		return Order{}, invalid("total cannot be negative")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.orders[o.OrderID]; ok {
		return Order{}, exists("order %s is already recorded", o.OrderID)
	}
	h.seq++
	o.seq = h.seq
	o.CreatedAt = h.now().UTC().Format(time.RFC3339)
	h.orders[o.OrderID] = o
	return o, nil
}

// List returns a customer's orders, newest first.
func (h *History) List(customerID string) ([]Order, error) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return nil, invalid("customer_id is required")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []Order{}
	for _, o := range h.orders {
		if o.CustomerID == customerID {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].seq > out[j].seq })
	return out, nil
}

// Get returns one order.
func (h *History) Get(orderID string) (Order, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return Order{}, invalid("order_id is required")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	o, ok := h.orders[orderID]
	if !ok {
		return Order{}, notFound("order %s was not found", orderID)
	}
	return o, nil
}

func (h *History) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.orders = map[string]Order{}
	h.seq = 0
}
