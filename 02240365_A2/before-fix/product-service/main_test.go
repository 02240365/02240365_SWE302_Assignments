package main

// Lab 4, Part B: Unit tests for the Product Service.
//
// These tests call ProductServiceServer.GetProduct directly as a plain Go
// function - no gRPC server is started, no network is used, and nothing
// else in the system needs to be running. That is what makes this a unit
// test: it exercises ONE component (the Product Service's own lookup
// logic) in complete isolation.
//
// Run with:  go test ./product-service/...

import (
	"context"
	"testing"

	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestServer builds a ProductServiceServer with the same sample data
// used in main(), so these tests don't depend on main() ever running.
func newTestServer() *ProductServiceServer {
	return &ProductServiceServer{
		products: map[string]*pb.Product{
			"P001": {ProductId: "P001", Name: "Laptop", Price: 75000},
			"P002": {ProductId: "P002", Name: "Mechanical Keyboard", Price: 4500},
			"P003": {ProductId: "P003", Name: "Wireless Mouse", Price: 1800},
			"P004": {ProductId: "P004", Name: "Monitor", Price: 25000},
		},
	}
}

// TestGetProduct_Success covers "successful retrieval of an existing
// record" and checks the returned data matches what was expected.
func TestGetProduct_Success(t *testing.T) {
	server := newTestServer()

	got, err := server.GetProduct(context.Background(), &pb.GetProductRequest{ProductId: "P001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.GetProductId() != "P001" || got.GetName() != "Laptop" || got.GetPrice() != 75000 {
		t.Errorf("got %+v, want ProductId=P001 Name=Laptop Price=75000", got)
	}
}

// TestGetProduct_DifferentIDs covers "retrieval of different records
// using different identifiers", verifying each one independently.
func TestGetProduct_DifferentIDs(t *testing.T) {
	server := newTestServer()

	cases := []struct {
		id        string
		wantName  string
		wantPrice float64
	}{
		{"P001", "Laptop", 75000},
		{"P002", "Mechanical Keyboard", 4500},
		{"P003", "Wireless Mouse", 1800},
		{"P004", "Monitor", 25000},
	}

	for _, c := range cases {
		got, err := server.GetProduct(context.Background(), &pb.GetProductRequest{ProductId: c.id})
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.id, err)
			continue
		}
		if got.GetName() != c.wantName || got.GetPrice() != c.wantPrice {
			t.Errorf("%s: got %+v, want Name=%s Price=%.2f", c.id, got, c.wantName, c.wantPrice)
		}
	}
}

// TestGetProduct_NotFound covers "a request for a record that does not
// exist" and checks the correct gRPC status code is returned.
func TestGetProduct_NotFound(t *testing.T) {
	server := newTestServer()

	_, err := server.GetProduct(context.Background(), &pb.GetProductRequest{ProductId: "P999"})
	if err == nil {
		t.Fatal("expected an error for a non-existent product, got nil")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("got code %s, want NotFound", st.Code())
	}
	if st.Message() == "" {
		t.Error("expected a human-readable error message, got an empty string")
	}
}

// TestGetProduct_EmptyID covers "an empty or invalid identifier".
func TestGetProduct_EmptyID(t *testing.T) {
	server := newTestServer()

	_, err := server.GetProduct(context.Background(), &pb.GetProductRequest{ProductId: ""})
	if err == nil {
		t.Fatal("expected an error for an empty product ID, got nil")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %s, want InvalidArgument", st.Code())
	}
}

// TestGetProduct_ErrorHandling covers "appropriate error handling" more
// generally: every error path must return a proper gRPC status (not a
// plain Go error), since that is what every calling service depends on
// to tell failures apart (see order-service and ui-service).
func TestGetProduct_ErrorHandling(t *testing.T) {
	server := newTestServer()

	unknownID, err := server.GetProduct(context.Background(), &pb.GetProductRequest{ProductId: "DOES-NOT-EXIST"})
	if unknownID != nil {
		t.Error("expected a nil product alongside the error")
	}
	if _, ok := status.FromError(err); !ok {
		t.Error("expected a proper gRPC status error, not a plain error")
	}

	emptyID, err := server.GetProduct(context.Background(), &pb.GetProductRequest{ProductId: ""})
	if emptyID != nil {
		t.Error("expected a nil product alongside the error")
	}
	if _, ok := status.FromError(err); !ok {
		t.Error("expected a proper gRPC status error, not a plain error")
	}
}
