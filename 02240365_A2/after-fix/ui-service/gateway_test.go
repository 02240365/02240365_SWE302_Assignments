package main

// SWE302 Assignment 3 - unit tests for the API gateway (ui-service).
// The gateway had no tests before. These cover the local discount rules
// (TC01, TC02) and the request checks that run before any microservice is
// called (TC16), using net/http/httptest - no network or services needed.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TC01 (FR3) - boundary: 0% for 1 unit, 5% from 2, 10% from 3, 15% from 5.
func TestTC01_QuantityDiscountTiers(t *testing.T) {
	cases := []struct {
		qty     int32
		percent float64
		final   float64
	}{
		{1, 0, 4500}, {2, 5, 8550}, {3, 10, 12150}, {4, 10, 16200}, {5, 15, 19125}, {20, 15, 76500},
	}
	for _, tc := range cases {
		d := localDiscount("P002", 4500, tc.qty)
		if !d.Success || d.DiscountPercent != tc.percent || d.FinalTotal != tc.final {
			t.Errorf("qty %d: want %v%% / %v, got %+v", tc.qty, tc.percent, tc.final, d)
		}
	}
}

// TC02 (FR3) - invalid: missing product, zero price or zero quantity.
func TestTC02_QuantityDiscountInvalidInput(t *testing.T) {
	for name, d := range map[string]*discountResult{
		"empty product": localDiscount("", 4500, 1),
		"zero price":    localDiscount("P002", 0, 1),
		"zero quantity": localDiscount("P002", 4500, 0),
		"negative qty":  localDiscount("P002", 4500, -3),
	} {
		if d.Success || d.ErrorCode != "invalid_input" {
			t.Errorf("%s: want invalid_input, got %+v", name, d)
		}
	}
}

// TC16 (NFR2) - invalid requests are answered with a clear JSON error
// before any downstream service is contacted.
func TestTC16_GatewayRejectsBadRequests(t *testing.T) {
	g := &gatewayClients{} // no connections: the handlers must not need them here
	cases := []struct {
		name       string
		handler    http.HandlerFunc
		method     string
		body       string
		wantStatus int
	}{
		{"malformed JSON to cart", cartHandler(g), "POST", "{oops", 400},
		{"malformed JSON to reviews", reviewsAddHandler(g), "POST", "{oops", 400},
		{"cart quantity 0", cartHandler(g), "POST", `{"customerId":"C001","productId":"P001","quantity":0}`, 400},
		{"cart without customer", cartHandler(g), "POST", `{"productId":"P001","quantity":1}`, 400},
		{"quantity is text", cartHandler(g), "POST", `{"customerId":"C001","productId":"P001","quantity":"two"}`, 400},
		{"wrong method on orders", ordersHandler(g, nil), "PUT", "", 405},
		{"wrong method on checkout", checkoutHandler(g, nil), "GET", "", 405},
		{"body over 64 KB", reviewsAddHandler(g), "POST", `{"comment":"` + strings.Repeat("x", 70000) + `"}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("want %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["success"] != false || body["message"] == "" {
				t.Fatalf("want a JSON error with a message, got %s", rec.Body)
			}
		})
	}
}

// Order quantity above the per-order limit is refused (part of TC18).
func TestTC18_OrderQuantityLimit(t *testing.T) {
	code, resp := placeOrder(&gatewayClients{}, nil, orderRequest{CustomerID: "C001", ProductID: "P001", Quantity: maxOrderQty + 1}, false)
	if code != 400 || resp.ErrorCode != "invalid_input" {
		t.Fatalf("want 400 invalid_input, got %d %+v", code, resp)
	}
}

// Configuration: addresses come from the environment, with safe defaults.
func TestEnvOr(t *testing.T) {
	t.Setenv("PRODUCT_ADDR", "product:50051")
	t.Setenv("BLANK_ADDR", "   ")
	if got := envOr("PRODUCT_ADDR", "localhost:50051"); got != "product:50051" {
		t.Errorf("want env value, got %s", got)
	}
	if got := envOr("BLANK_ADDR", "localhost:1"); got != "localhost:1" {
		t.Errorf("blank should use default, got %s", got)
	}
	if got := envOr("UNSET_ADDR_XYZ", ":8080"); got != ":8080" {
		t.Errorf("unset should use default, got %s", got)
	}
}
