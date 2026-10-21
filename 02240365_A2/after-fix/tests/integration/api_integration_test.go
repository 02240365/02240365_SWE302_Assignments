// Package integration holds SWE302 integration tests. They call the real
// HTTP gateway, which calls the ten real gRPC services, so they check that
// the services work together (serialisation, error mapping, stock, history).
//
// Start the system first:   ./run-all.sh test
// Run:                      go test ./tests/integration/ -v
// If the gateway is not running the tests are skipped, so `go test ./...`
// still passes on a machine without the services.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var baseURL = func() string {
	if u := os.Getenv("GATEWAY_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}()

var client = &http.Client{Timeout: 20 * time.Second}

// call sends a request and decodes the JSON answer into a map.
func call(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, baseURL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// setup skips the test when the gateway is down and resets all data, so
// every test starts from the same state (stock, carts, coupons, orders).
func setup(t *testing.T) {
	t.Helper()
	if _, err := client.Get(baseURL + "/api/services"); err != nil {
		t.Skip("gateway not running - start it with ./run-all.sh test")
	}
	if code, _ := call(t, "POST", "/api/test/reset", nil); code != 200 {
		t.Fatalf("reset failed (%d) - start with ./run-all.sh test", code)
	}
}

// stockOf asks the Inventory Service directly over gRPC (the gateway has
// no stock endpoint), so the test checks the real stored value.
func stockOf(t *testing.T, productID string) int32 {
	t.Helper()
	conn, err := grpc.NewClient("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, err := pb.NewInventoryServiceClient(conn).CheckStock(ctx, &pb.StockRequest{ProductId: productID})
	if err != nil {
		t.Fatalf("CheckStock %s: %v", productID, err)
	}
	return r.GetQuantity()
}

// TC17 (FR6, FR8) - normal: an order passes through customer, product,
// discount, shipping, inventory, payment, shipment and history.
func TestTC17_PlaceOrderEndToEndAcrossServices(t *testing.T) {
	setup(t)
	before := stockOf(t, "P003")
	start := time.Now()
	code, o := call(t, "POST", "/api/orders", map[string]any{"customerId": "C001", "productId": "P003", "quantity": 2})
	elapsed := time.Since(start)
	if code != 200 || o["success"] != true {
		t.Fatalf("order failed: %d %v", code, o)
	}
	// 2 x 1800 = 3600, 5% quantity discount = 3420, Thimphu shipping Nu. 100.
	if o["finalTotal"] != 3520.0 || o["shippingZone"] != "WESTERN" || o["discountPercent"] != 5.0 {
		t.Errorf("unexpected amounts: %v", o)
	}
	if after := stockOf(t, "P003"); after != before-2 {
		t.Errorf("stock should drop by 2: before %v after %v", before, after)
	}
	_, hist := call(t, "GET", "/api/orders?customerId=C001", nil)
	if orders := hist["orders"].([]any); len(orders) != 1 {
		t.Errorf("history should hold 1 order, got %d", len(orders))
	}
	code, tr := call(t, "GET", fmt.Sprintf("/api/shipping/track/%v", o["trackingId"]), nil)
	if code != 200 || tr["status"] != "PROCESSING" {
		t.Errorf("tracking failed: %d %v", code, tr)
	}
	t.Logf("order %v placed in %v (includes ~3 s notification stream)", o["orderId"], elapsed.Round(time.Millisecond))
	if elapsed > 8*time.Second { // NFR3
		t.Errorf("order took %v, limit 8s", elapsed)
	}
}

// TC18 (FR6) - invalid/boundary: unknown product and 21 units are refused
// and do not touch stock.
func TestTC18_InvalidOrdersRefused(t *testing.T) {
	setup(t)
	before := stockOf(t, "P001")
	if code, o := call(t, "POST", "/api/orders", map[string]any{"customerId": "C001", "productId": "P999", "quantity": 1}); code != 404 {
		t.Errorf("unknown product: want 404, got %d %v", code, o)
	}
	if code, o := call(t, "POST", "/api/orders", map[string]any{"customerId": "C001", "productId": "P001", "quantity": 21}); code != 400 {
		t.Errorf("21 units: want 400, got %d %v", code, o)
	}
	if code, o := call(t, "POST", "/api/orders", map[string]any{"customerId": "C999", "productId": "P001", "quantity": 1}); code != 404 {
		t.Errorf("unknown customer: want 404, got %d %v", code, o)
	}
	if after := stockOf(t, "P001"); after != before {
		t.Errorf("stock must not change: %v -> %v", before, after)
	}
}

// TC19 (FR4, FR6) - negative: an expired coupon stops the order before
// stock is reserved or payment is taken.
func TestTC19_ExpiredCouponStopsOrder(t *testing.T) {
	setup(t)
	before := stockOf(t, "P002")
	code, o := call(t, "POST", "/api/orders", map[string]any{"customerId": "C001", "productId": "P002", "quantity": 1, "couponCode": "OLD20"})
	if code != 409 || o["step"] != "coupon" {
		t.Fatalf("want 409 at step coupon, got %d %v", code, o)
	}
	if after := stockOf(t, "P002"); after != before {
		t.Errorf("stock must not change: %v -> %v", before, after)
	}
	_, hist := call(t, "GET", "/api/orders?customerId=C001", nil)
	if len(hist["orders"].([]any)) != 0 {
		t.Error("no order should be recorded")
	}
}

// TC20 (FR2) - negative: a huge quantity sent to the cart API must be
// refused and the cart must keep a positive quantity (defect D1).
func TestTC20_CartRejectsOverflowQuantity(t *testing.T) {
	setup(t)
	call(t, "POST", "/api/cart", map[string]any{"customerId": "C001", "productId": "P003", "quantity": 1})
	code, c := call(t, "POST", "/api/cart", map[string]any{"customerId": "C001", "productId": "P003", "quantity": 2147483647})
	if code != 409 {
		t.Errorf("want 409, got %d %v", code, c)
	}
	_, cart := call(t, "GET", "/api/cart?customerId=C001", nil)
	if cart["subtotal"].(float64) <= 0 {
		t.Errorf("cart subtotal must stay positive, got %v", cart["subtotal"])
	}
}

// TC11 at integration level: customer C002 lives in Phuentsholing, a border
// town, so the quote is Nu. 150 with a 2-day ETA.
func TestTC11_ShippingQuoteForBorderCustomer(t *testing.T) {
	setup(t)
	code, q := call(t, "GET", "/api/shipping/quote?customerId=C002&subtotal=4500&itemCount=1", nil)
	if code != 200 || q["zone"] != "BORDER" || q["fee"] != 150.0 || q["etaDays"] != 2.0 {
		t.Fatalf("want BORDER/150/2, got %d %v", code, q)
	}
}
