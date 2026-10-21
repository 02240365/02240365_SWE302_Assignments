package main

// Part 2 - the order flow, rewritten as one reusable function.
//
// placeOrder is used by POST /api/orders (one product) and by
// POST /api/cart/checkout (every line of the cart). The order of the steps
// matters, and was changed in Part 2 after testing found that the first
// version could lose stock:
//
//   1. customer   - who is ordering, and the delivery address
//   2. product    - name and price
//   3. discount   - quantity discount (AWS Lambda, or the local rules)
//   4. coupon     - optional coupon, checked but not yet used up
//   5. shipping   - delivery fee for the address
//   6. inventory  - reserve the stock (only now, right before paying)
//   7. payment    - charge goods + shipping; if this fails the stock is
//                   given back (RestoreStock) - the "compensating action"
//   8. after payment, best-effort: redeem coupon, book shipment, save to
//      order history, collect notifications. A failure here does not undo
//      the payment; it is reported in "warnings".

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	pb "ecommerce-microservices/proto"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

// maxOrderQty is the most units of one product allowed in a single order
// (the same limit as one cart line).
const maxOrderQty = 20

var orderSeq int64

// newOrderID returns an ID that is unique even when the same customer
// buys the same product twice. (The first version used "ORD-<customer>-
// <product>", so a second order produced the same ID as the first.)
func newOrderID(customerID, productID string) string {
	n := atomic.AddInt64(&orderSeq, 1)
	return fmt.Sprintf("ORD-%s-%s-%d%03d", customerID, productID, time.Now().UnixMilli(), n%1000)
}

type orderRequest struct {
	CustomerID string `json:"customerId"`
	ProductID  string `json:"productId"`
	Quantity   int32  `json:"quantity"`
	CouponCode string `json:"couponCode"`
}

type orderResponse struct {
	Success         bool     `json:"success"`
	Step            string   `json:"step,omitempty"` // which step failed, if any
	ErrorCode       string   `json:"errorCode,omitempty"`
	Message         string   `json:"message,omitempty"`
	OrderID         string   `json:"orderId,omitempty"`
	CustomerName    string   `json:"customerName,omitempty"`
	ProductName     string   `json:"productName,omitempty"`
	UnitPrice       float64  `json:"unitPrice,omitempty"`
	Quantity        int32    `json:"quantity,omitempty"`
	TotalAmount     float64  `json:"totalAmount,omitempty"` // price x quantity, before discounts
	RemainingStock  int32    `json:"remainingStock"`
	PaymentID       string   `json:"paymentId,omitempty"`
	Notifications   []string `json:"notifications,omitempty"`
	DiscountPercent float64  `json:"discountPercent,omitempty"` // quantity discount
	DiscountAmount  float64  `json:"discountAmount,omitempty"`
	CouponCode      string   `json:"couponCode,omitempty"`
	CouponDiscount  float64  `json:"couponDiscount,omitempty"`
	ShippingFee     float64  `json:"shippingFee"`
	ShippingZone    string   `json:"shippingZone,omitempty"`
	EtaDays         int32    `json:"etaDays,omitempty"`
	TrackingID      string   `json:"trackingId,omitempty"`
	FinalTotal      float64  `json:"finalTotal,omitempty"` // what was actually charged
	Warnings        []string `json:"warnings,omitempty"`
}

func fail(step string, err error, notFoundMsg string) (int, orderResponse) {
	httpStatus, code, msg := dependencyErrorCode(err)
	if code == "not_found" && notFoundMsg != "" {
		msg = notFoundMsg
	}
	return httpStatus, orderResponse{Success: false, Step: step, ErrorCode: code, Message: msg}
}

func invalidOrder(msg string) (int, orderResponse) {
	return http.StatusBadRequest, orderResponse{Success: false, ErrorCode: "invalid_input", Message: msg}
}

func round2(v float64) float64 {
	if v < 0 {
		return -round2(-v)
	}
	return float64(int64(v*100+0.5)) / 100
}

// placeOrder runs the whole flow for one product. withNotifications adds
// about three seconds (the notification stream), so checkout turns it off.
func placeOrder(g *gatewayClients, lambdaClient *lambda.Client, req orderRequest, withNotifications bool) (int, orderResponse) {
	req.CustomerID = strings.TrimSpace(req.CustomerID)
	req.ProductID = strings.TrimSpace(req.ProductID)
	req.CouponCode = strings.TrimSpace(req.CouponCode)
	if req.CustomerID == "" || req.ProductID == "" || req.Quantity <= 0 {
		return invalidOrder("Please provide a customer ID, a product ID, and a quantity greater than zero.")
	}
	if req.Quantity > maxOrderQty {
		return invalidOrder(fmt.Sprintf("You can order at most %d units of one product at a time.", maxOrderQty))
	}

	// 1. customer
	customer, err := g.getCustomer(req.CustomerID)
	if err != nil {
		return fail("customer", err, fmt.Sprintf("Customer '%s' does not exist.", req.CustomerID))
	}
	// 2. product
	product, err := g.getProduct(req.ProductID)
	if err != nil {
		return fail("product", err, fmt.Sprintf("Product '%s' does not exist.", req.ProductID))
	}
	orderID := newOrderID(req.CustomerID, req.ProductID)
	subtotal := round2(product.GetPrice() * float64(req.Quantity))

	// 3. quantity discount (Lambda)
	discount, err := calculateDiscount(lambdaClient, req.ProductID, product.GetPrice(), req.Quantity)
	if err != nil {
		return fail("discount", err, "")
	}
	if !discount.Success {
		return http.StatusBadRequest, orderResponse{Success: false, Step: "discount", ErrorCode: "invalid_input", Message: discount.Message}
	}
	goods := discount.FinalTotal

	// 4. coupon (optional) - validated now, redeemed only after payment
	couponDiscount := 0.0
	couponCode := ""
	if req.CouponCode != "" {
		c, err := g.validateCoupon(req.CouponCode, req.CustomerID, goods)
		if err != nil {
			return fail("coupon", err, "")
		}
		couponDiscount, couponCode = c.GetDiscountAmount(), c.GetCode()
		goods = round2(goods - couponDiscount)
	}

	// 5. shipping quote
	quote, err := g.getQuote(customer.GetAddress(), req.Quantity, goods)
	if err != nil {
		return fail("shipping", err, "")
	}
	charge := round2(goods + quote.GetFee())

	// 6. reserve stock
	stock, err := g.decreaseStock(req.ProductID, req.Quantity)
	if err != nil {
		return fail("inventory", err, "")
	}

	// 7. payment - on failure give the stock back
	payment, err := g.processPayment(orderID, req.CustomerID, charge)
	if err != nil {
		if _, rerr := g.restoreStock(req.ProductID, req.Quantity); rerr != nil {
			log.Printf("ALERT: payment failed for %s and the stock could NOT be restored (%d x %s): %v", orderID, req.Quantity, req.ProductID, rerr)
		}
		return fail("payment", err, "")
	}

	resp := orderResponse{
		Success: true, OrderID: orderID, CustomerName: customer.GetName(), ProductName: product.GetName(),
		UnitPrice: product.GetPrice(), Quantity: req.Quantity, TotalAmount: subtotal, RemainingStock: stock.GetQuantity(),
		PaymentID: payment.GetPaymentId(), DiscountPercent: discount.DiscountPercent, DiscountAmount: discount.DiscountAmount,
		CouponCode: couponCode, CouponDiscount: couponDiscount, ShippingFee: quote.GetFee(),
		ShippingZone: quote.GetZone(), EtaDays: quote.GetEtaDays(), FinalTotal: charge,
	}

	// 8. best-effort steps after the money has been taken
	if couponCode != "" {
		if err := g.redeemCoupon(couponCode, req.CustomerID, orderID); err != nil {
			log.Printf("coupon %s could not be marked as used for %s: %v", couponCode, orderID, err)
			resp.Warnings = append(resp.Warnings, "The coupon could not be marked as used.")
		}
	}
	shipment, err := g.createShipment(orderID, req.CustomerID, customer.GetAddress())
	if err != nil {
		log.Printf("shipment for %s failed: %v", orderID, err)
		resp.Warnings = append(resp.Warnings, "Your order is paid, but the shipment could not be created yet. Support will contact you.")
	} else {
		resp.TrackingID = shipment.GetTrackingId()
	}
	_, err = g.recordOrder(&pb.OrderRecordMsg{
		OrderId: orderID, CustomerId: req.CustomerID, ProductId: req.ProductID, ProductName: product.GetName(),
		Quantity: req.Quantity, Subtotal: subtotal, DiscountTotal: round2(discount.DiscountAmount + couponDiscount),
		ShippingFee: quote.GetFee(), Total: charge, PaymentId: payment.GetPaymentId(),
		TrackingId: resp.TrackingID, CouponCode: couponCode,
	})
	if err != nil {
		log.Printf("order %s could not be saved to history: %v", orderID, err)
		resp.Warnings = append(resp.Warnings, "The order could not be added to your order history.")
	}
	if withNotifications {
		notifications, nerr := g.collectNotifications(req.CustomerID, orderID)
		if nerr != nil {
			log.Printf("notification stream ended early: %v", nerr)
		}
		resp.Notifications = notifications
	}
	return http.StatusOK, resp
}

// ordersHandler: POST /api/orders places an order; GET /api/orders?customerId=
// lists a customer's order history.
func ordersHandler(g *gatewayClients, lambdaClient *lambda.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listOrdersHandler(g)(w, r)
		case http.MethodPost:
			var req orderRequest
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, orderResponse{Success: false, ErrorCode: "invalid_input", Message: "Malformed request body."})
				return
			}
			code, resp := placeOrder(g, lambdaClient, req, true)
			writeJSON(w, code, resp)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, orderResponse{Success: false, ErrorCode: "invalid_input", Message: "Use POST to place an order or GET to list orders."})
		}
	}
}
