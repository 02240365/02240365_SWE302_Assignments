package shop

import (
	"sort"
	"strings"
	"sync"
)

// Cart limits. A real shop would make these configurable.
const (
	MaxQtyPerLine = 20 // most units of one product in a cart
	MaxLines      = 10 // most different products in a cart
)

// CartLine is one product in a customer's cart. The gateway copies the
// name and price from the Product Service when the item is added, so the
// cart can show totals without calling any other service.
type CartLine struct {
	ProductID   string
	ProductName string
	UnitPrice   float64
	Quantity    int32
	LineTotal   float64
}

// CartView is a snapshot of one customer's cart.
type CartView struct {
	CustomerID string
	Lines      []CartLine
	ItemCount  int32
	Subtotal   float64
}

// Carts keeps every customer's cart in memory (like the other services).
type Carts struct {
	mu    sync.Mutex
	carts map[string]map[string]*CartLine
}

func NewCarts() *Carts { return &Carts{carts: map[string]map[string]*CartLine{}} }

func round2(v float64) float64 {
	if v < 0 {
		return -round2(-v)
	}
	return float64(int64(v*100+0.5)) / 100
}

// Add puts quantity units of a product in the cart. Adding a product that
// is already there increases its quantity.
func (c *Carts) Add(customerID, productID, name string, price float64, qty int32) (CartView, error) {
	customerID, productID = strings.TrimSpace(customerID), strings.TrimSpace(productID)
	if customerID == "" || productID == "" {
		return CartView{}, invalid("customer_id and product_id are required")
	}
	if qty <= 0 {
		return CartView{}, invalid("quantity must be at least 1")
	}
	if price <= 0 {
		return CartView{}, invalid("unit price must be greater than zero")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	lines := c.carts[customerID]
	if lines == nil {
		lines = map[string]*CartLine{}
		c.carts[customerID] = lines
	}
	line, ok := lines[productID]
	if !ok && len(lines) >= MaxLines {
		return CartView{}, conflict("the cart is full: it can hold at most %d different products", MaxLines)
	}
	have := int32(0)
	if ok {
		have = line.Quantity
	}
	if have+qty > MaxQtyPerLine {
		return CartView{}, conflict("you can have at most %d units of one product in the cart (you already have %d)", MaxQtyPerLine, have)
	}
	if !ok {
		line = &CartLine{ProductID: productID, ProductName: name}
		lines[productID] = line
	}
	line.UnitPrice = price
	line.Quantity = have + qty
	return c.viewLocked(customerID), nil
}

// Get returns the cart. A customer who has never added anything simply
// has an empty cart; that is not an error.
func (c *Carts) Get(customerID string) (CartView, error) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return CartView{}, invalid("customer_id is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.viewLocked(customerID), nil
}

// Remove deletes one product from the cart.
func (c *Carts) Remove(customerID, productID string) (CartView, error) {
	customerID, productID = strings.TrimSpace(customerID), strings.TrimSpace(productID)
	if customerID == "" || productID == "" {
		return CartView{}, invalid("customer_id and product_id are required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.carts[customerID][productID]; !ok {
		return CartView{}, notFound("product %s is not in the cart", productID)
	}
	delete(c.carts[customerID], productID)
	return c.viewLocked(customerID), nil
}

// Clear empties the cart.
func (c *Carts) Clear(customerID string) (CartView, error) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		return CartView{}, invalid("customer_id is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.carts, customerID)
	return c.viewLocked(customerID), nil
}

// Reset removes every cart (used only by the test-mode reset).
func (c *Carts) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.carts = map[string]map[string]*CartLine{}
}

func (c *Carts) viewLocked(customerID string) CartView {
	v := CartView{CustomerID: customerID, Lines: []CartLine{}}
	for _, l := range c.carts[customerID] {
		line := *l
		line.LineTotal = round2(line.UnitPrice * float64(line.Quantity))
		v.Lines = append(v.Lines, line)
		v.ItemCount += line.Quantity
		v.Subtotal += line.LineTotal
	}
	sort.Slice(v.Lines, func(i, j int) bool { return v.Lines[i].ProductID < v.Lines[j].ProductID })
	v.Subtotal = round2(v.Subtotal)
	return v
}
