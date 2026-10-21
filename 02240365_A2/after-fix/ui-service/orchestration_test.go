package main

// SWE302 Assignment 3 - component-integration tests for the order flow.
//
// placeOrder and checkoutHandler call ten services through generated gRPC
// client interfaces. Here each interface is replaced by a small in-memory
// fake; the coupon and shipping fakes use the REAL business rules from
// internal/shop, so the gateway and those rules are tested together.
// Embedding the interface means a fake only implements the calls it needs.

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"ecommerce-microservices/internal/rpcerr"
	"ecommerce-microservices/internal/shop"
	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeProduct struct{ pb.ProductServiceClient }

func (fakeProduct) GetProduct(_ context.Context, in *pb.GetProductRequest, _ ...grpc.CallOption) (*pb.Product, error) {
	p, ok := map[string]*pb.Product{
		"P002": {ProductId: "P002", Name: "Mechanical Keyboard", Price: 4500},
		"P003": {ProductId: "P003", Name: "Wireless Mouse", Price: 1800},
	}[in.GetProductId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "no product")
	}
	return p, nil
}

type fakeCustomer struct{ pb.CustomerServiceClient }

func (fakeCustomer) GetCustomer(_ context.Context, in *pb.GetCustomerRequest, _ ...grpc.CallOption) (*pb.Customer, error) {
	return &pb.Customer{CustomerId: in.GetCustomerId(), Name: "Sonam Choki", Address: "Phuntsholing, Bhutan"}, nil
}

type fakeInventory struct {
	pb.InventoryServiceClient
	stock map[string]int32
}

func (f *fakeInventory) DecreaseStock(_ context.Context, in *pb.DecreaseStockRequest, _ ...grpc.CallOption) (*pb.StockResponse, error) {
	if f.stock[in.GetProductId()] < in.GetQuantity() {
		return nil, status.Error(codes.FailedPrecondition, "not enough stock")
	}
	f.stock[in.GetProductId()] -= in.GetQuantity()
	return &pb.StockResponse{ProductId: in.GetProductId(), Quantity: f.stock[in.GetProductId()]}, nil
}

func (f *fakeInventory) RestoreStock(_ context.Context, in *pb.DecreaseStockRequest, _ ...grpc.CallOption) (*pb.StockResponse, error) {
	f.stock[in.GetProductId()] += in.GetQuantity()
	return &pb.StockResponse{ProductId: in.GetProductId(), Quantity: f.stock[in.GetProductId()]}, nil
}

type fakePayment struct {
	pb.PaymentServiceClient
	decline bool
	charged []float64
}

func (f *fakePayment) ProcessPayment(_ context.Context, in *pb.PaymentRequest, _ ...grpc.CallOption) (*pb.PaymentResponse, error) {
	if f.decline {
		return nil, status.Error(codes.FailedPrecondition, "card declined")
	}
	f.charged = append(f.charged, in.GetAmount())
	return &pb.PaymentResponse{PaymentId: "PAY-1", Status: "SUCCESS"}, nil
}

type fakeCoupon struct {
	pb.CouponServiceClient
	rules *shop.Coupons
}

func (f fakeCoupon) ValidateCoupon(_ context.Context, in *pb.ValidateCouponRequest, _ ...grpc.CallOption) (*pb.CouponResponse, error) {
	r, err := f.rules.Validate(in.GetCode(), in.GetCustomerId(), in.GetSubtotal())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return &pb.CouponResponse{Code: r.Code, DiscountAmount: r.DiscountAmount, Description: r.Description}, nil
}

func (f fakeCoupon) RedeemCoupon(_ context.Context, in *pb.RedeemCouponRequest, _ ...grpc.CallOption) (*pb.RedeemCouponResponse, error) {
	return &pb.RedeemCouponResponse{Status: "OK"}, rpcerr.To(f.rules.Redeem(in.GetCode(), in.GetCustomerId(), in.GetOrderId()))
}

type fakeShipping struct {
	pb.ShippingServiceClient
	rules *shop.Shipping
}

func (f fakeShipping) GetQuote(_ context.Context, in *pb.QuoteRequest, _ ...grpc.CallOption) (*pb.QuoteResponse, error) {
	q, err := f.rules.GetQuote(in.GetAddress(), in.GetItemCount(), in.GetSubtotal())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return &pb.QuoteResponse{Zone: q.Zone, Method: q.Method, Fee: q.Fee, EtaDays: q.EtaDays, FreeShipping: q.FreeShipping}, nil
}

func (f fakeShipping) CreateShipment(_ context.Context, in *pb.CreateShipmentRequest, _ ...grpc.CallOption) (*pb.ShipmentMsg, error) {
	s, err := f.rules.Create(in.GetOrderId(), in.GetCustomerId(), in.GetAddress())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return &pb.ShipmentMsg{TrackingId: s.TrackingID, OrderId: s.OrderID, Status: s.Status}, nil
}

type fakeHistory struct {
	pb.OrderHistoryServiceClient
	saved []*pb.OrderRecordMsg
}

func (f *fakeHistory) RecordOrder(_ context.Context, in *pb.OrderRecordMsg, _ ...grpc.CallOption) (*pb.OrderRecordMsg, error) {
	f.saved = append(f.saved, in)
	return in, nil
}

type fakeCart struct {
	pb.CartServiceClient
	lines []*pb.CartLineMsg
}

func (f *fakeCart) GetCart(_ context.Context, in *pb.CartCustomerRequest, _ ...grpc.CallOption) (*pb.CartResponse, error) {
	return &pb.CartResponse{CustomerId: in.GetCustomerId(), Lines: f.lines}, nil
}

func (f *fakeCart) RemoveItem(_ context.Context, in *pb.RemoveCartItemRequest, _ ...grpc.CallOption) (*pb.CartResponse, error) {
	kept := []*pb.CartLineMsg{}
	for _, l := range f.lines {
		if l.GetProductId() != in.GetProductId() {
			kept = append(kept, l)
		}
	}
	f.lines = kept
	return &pb.CartResponse{Lines: kept}, nil
}

// newFakeGateway wires the fakes together, with 10 of each product in stock.
func newFakeGateway(t *testing.T) (*gatewayClients, *fakeInventory, *fakePayment, *fakeHistory, *fakeCart) {
	t.Setenv("LAMBDA_FUNCTION_NAME", "") // use the local discount rules
	inv := &fakeInventory{stock: map[string]int32{"P002": 10, "P003": 10}}
	pay, hist, cart := &fakePayment{}, &fakeHistory{}, &fakeCart{}
	return &gatewayClients{
		product: fakeProduct{}, customer: fakeCustomer{}, inventory: inv, payment: pay,
		coupon: fakeCoupon{rules: shop.NewCoupons()}, shipping: fakeShipping{rules: shop.NewShipping()},
		history: hist, cart: cart,
	}, inv, pay, hist, cart
}

// TC23 (FR3-FR6, FR8) - normal: all eight steps run in order and the
// amounts combine correctly: 2 x 4,500 = 9,000 -5% = 8,550, WELCOME10 -855
// = 7,695, free shipping (>= 5,000).
func TestTC23_PlaceOrderComputesTotalsAndRecords(t *testing.T) {
	g, inv, pay, hist, _ := newFakeGateway(t)
	code, r := placeOrder(g, nil, orderRequest{CustomerID: "C002", ProductID: "P002", Quantity: 2, CouponCode: "welcome10"}, false)
	if code != 200 || !r.Success {
		t.Fatalf("want success, got %d %+v", code, r)
	}
	if r.DiscountPercent != 5 || r.CouponDiscount != 855 || r.ShippingFee != 0 || r.FinalTotal != 7695 {
		t.Errorf("wrong amounts: %+v", r)
	}
	if r.ShippingZone != "BORDER" || r.TrackingID == "" || len(r.Warnings) != 0 {
		t.Errorf("shipping/tracking wrong: %+v", r)
	}
	if inv.stock["P002"] != 8 || len(pay.charged) != 1 || pay.charged[0] != 7695 || len(hist.saved) != 1 {
		t.Errorf("side effects wrong: stock=%d charged=%v saved=%d", inv.stock["P002"], pay.charged, len(hist.saved))
	}
}

// TC24 (FR6) - negative: a declined payment gives the reserved stock back
// (compensating action), does not use up the coupon and records nothing.
func TestTC24_PaymentFailureRestoresStock(t *testing.T) {
	g, inv, pay, hist, _ := newFakeGateway(t)
	pay.decline = true
	code, r := placeOrder(g, nil, orderRequest{CustomerID: "C002", ProductID: "P003", Quantity: 3, CouponCode: "WELCOME10"}, false)
	if code != 409 || r.Step != "payment" {
		t.Fatalf("want 409 at payment, got %d %+v", code, r)
	}
	if inv.stock["P003"] != 10 {
		t.Errorf("stock must be restored to 10, got %d", inv.stock["P003"])
	}
	if len(hist.saved) != 0 {
		t.Error("a failed order must not be recorded")
	}
	pay.decline = false // the coupon must still be usable afterwards
	if _, r2 := placeOrder(g, nil, orderRequest{CustomerID: "C002", ProductID: "P003", Quantity: 1, CouponCode: "WELCOME10"}, false); !r2.Success || r2.CouponCode != "WELCOME10" {
		t.Errorf("coupon should still work after the failed payment: %+v", r2)
	}
}

// TC25 (FR2, FR4) - checkout orders every cart line, applies the coupon to
// the first line only, and empties the cart.
func TestTC25_CheckoutAppliesCouponOnceAndEmptiesCart(t *testing.T) {
	g, _, pay, _, cart := newFakeGateway(t)
	cart.lines = []*pb.CartLineMsg{
		{ProductId: "P002", ProductName: "Mechanical Keyboard", UnitPrice: 4500, Quantity: 1},
		{ProductId: "P003", ProductName: "Wireless Mouse", UnitPrice: 1800, Quantity: 1},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/cart/checkout", strings.NewReader(`{"customerId":"C002","couponCode":"WELCOME10"}`))
	checkoutHandler(g, nil)(rec, req)
	var out checkoutResponse
	json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || !out.Success || len(out.Orders) != 2 {
		t.Fatalf("want 2 orders, got %d %s", rec.Code, rec.Body)
	}
	if out.Orders[0].CouponCode != "WELCOME10" || out.Orders[1].CouponCode != "" {
		t.Errorf("coupon must apply to the first line only: %+v", out.Orders)
	}
	// Keyboard 4,500 -450 +150 shipping = 4,200; mouse 1,800 +150 = 1,950.
	if out.Total != 6150 || len(pay.charged) != 2 || len(cart.lines) != 0 {
		t.Errorf("total %v, charges %v, lines left %d", out.Total, pay.charged, len(cart.lines))
	}
}
