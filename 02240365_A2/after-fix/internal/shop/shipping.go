package shop

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"ecommerce-microservices/internal/money"
)

// FreeShippingThreshold: orders at or above this subtotal ship free.
const FreeShippingThreshold = 5000.0

// Quote is the shipping price and delivery time for an address.
type Quote struct {
	Zone         string
	Method       string
	Fee          float64
	EtaDays      int32
	FreeShipping bool
}

// TrackEvent is one line in a shipment's history.
type TrackEvent struct {
	Time    string
	Message string
}

// Shipment is a parcel created for an order.
type Shipment struct {
	TrackingID string
	OrderID    string
	CustomerID string
	Address    string
	Status     string
	EtaDays    int32
	Events     []TrackEvent
}

// Shipping creates quotes and shipments.
type Shipping struct {
	mu        sync.Mutex
	counter   int
	shipments map[string]*Shipment
	now       func() time.Time
}

func NewShipping() *Shipping {
	return &Shipping{shipments: map[string]*Shipment{}, now: time.Now}
}

// deliveryZone is one shipping zone and the place names that belong to it.
// Adding a town or a spelling is now a data change, not new code.
type deliveryZone struct {
	Name, Method string
	Fee          float64
	EtaDays      int32
	Places       []string // lower-case place names, matched as whole words
}

var deliveryZones = []deliveryZone{
	{"WESTERN", "Standard (Western Bhutan)", 100, 1, []string{"thimphu", "paro"}},
	// "Phuntsholing" is the official spelling; "Phuentsholing" is also common (D2).
	{"BORDER", "Standard (Border towns)", 150, 2, []string{"phuentsholing", "phuntsholing", "gelephu"}},
}

var remoteZone = deliveryZone{"REMOTE", "Standard (Other districts)", 250, 4, nil}

// zoneFor chooses a delivery zone from the address. The address is split
// into words, so "Sparo Lane" no longer matches "paro" (defect D3).
func zoneFor(address string) deliveryZone {
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(address), func(r rune) bool {
		return !unicode.IsLetter(r)
	}) {
		words[w] = true
	}
	for _, z := range deliveryZones {
		for _, place := range z.Places {
			if words[place] {
				return z
			}
		}
	}
	return remoteZone
}

// GetQuote prices shipping. Orders of Nu. 5,000 or more ship free but the
// delivery time still depends on the zone.
func (s *Shipping) GetQuote(address string, itemCount int32, subtotal float64) (Quote, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return Quote{}, invalid("a delivery address is required")
	}
	if itemCount <= 0 {
		return Quote{}, invalid("item_count must be at least 1")
	}
	if !money.IsValidAmount(subtotal) { // also refuses NaN and infinity (D5)
		return Quote{}, invalid("subtotal must be greater than zero")
	}
	z := zoneFor(address)
	q := Quote{Zone: z.Name, Method: z.Method, Fee: z.Fee, EtaDays: z.EtaDays}
	if subtotal >= FreeShippingThreshold {
		q.Fee, q.FreeShipping = 0, true
	}
	return q, nil
}

// Create books a shipment and returns its tracking ID.
func (s *Shipping) Create(orderID, customerID, address string) (Shipment, error) {
	orderID, customerID, address = strings.TrimSpace(orderID), strings.TrimSpace(customerID), strings.TrimSpace(address)
	if orderID == "" || customerID == "" || address == "" {
		return Shipment{}, invalid("order_id, customer_id and address are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.shipments {
		if existing.OrderID == orderID {
			return Shipment{}, exists("a shipment already exists for order %s", orderID)
		}
	}
	s.counter++
	eta := zoneFor(address).EtaDays
	sh := &Shipment{
		TrackingID: fmt.Sprintf("TRK-%05d", s.counter), OrderID: orderID, CustomerID: customerID,
		Address: address, Status: "PROCESSING", EtaDays: eta,
		Events: []TrackEvent{{Time: s.now().UTC().Format(time.RFC3339), Message: "Shipment created; the warehouse is preparing your parcel"}},
	}
	s.shipments[sh.TrackingID] = sh
	return *sh, nil
}

// Track returns a shipment and its history.
func (s *Shipping) Track(trackingID string) (Shipment, error) {
	trackingID = strings.ToUpper(strings.TrimSpace(trackingID))
	if trackingID == "" {
		return Shipment{}, invalid("a tracking ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shipments[trackingID]
	if !ok {
		return Shipment{}, notFound("no shipment found with tracking ID %s", trackingID)
	}
	return *sh, nil
}

func (s *Shipping) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shipments = map[string]*Shipment{}
	s.counter = 0
}
