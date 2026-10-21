package main

// Lab 4, Part C: Integration tests.
//
// Unlike unit_test.go (which uses an in-memory mock and never touches the
// network), these tests start a REAL gRPC server - fakeProductServer - on
// an OS-assigned loopback port, and have the resilience functions dial it
// for real. That is what makes this an integration test: it exercises
// actual dialling, serialisation, and context deadlines over the network,
// not just Go function calls.
//
// fakeProductServer is a test double, not the real Product Service - its
// behaviour (delay, success, error) is scripted per test, which is what
// keeps these tests deterministic and independent of manually starting,
// stopping, or freezing a separate process the way the Lab 3 report does.
//
// This file covers all four situations required by Lab 4 Part C:
//   - the dependency is available                  -> TestIntegration_DependencyAvailable
//   - the dependency is unavailable                 -> TestIntegration_DependencyUnavailable
//   - the dependency returns an error                -> TestIntegration_DependencyReturnsError
//   - the dependency exceeds the configured timeout  -> TestIntegration_DependencyExceedsTimeout
//
// Run with:  go test ./order-service/...
// (these run alongside the unit tests in the same package; use
//  go test -short ./order-service/... to skip them and only run the fast,
//  fully in-memory unit tests from unit_test.go)

import (
	"context"
	"net"
	"testing"
	"time"

	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// fakeProductServer is a real gRPC server standing in for the Product
// Service, with scriptable behaviour so tests stay repeatable.
type fakeProductServer struct {
	pb.UnimplementedProductServiceServer
	delay    time.Duration
	failWith error
	product  *pb.Product
}

func (f *fakeProductServer) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.Product, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, "context deadline exceeded")
		}
	}
	if f.failWith != nil {
		return nil, f.failWith
	}
	return f.product, nil
}

// startFakeProductService starts fake on an OS-assigned free port and
// returns a connected gRPC client plus a cleanup function to call with
// defer.
func startFakeProductService(t *testing.T, fake *fakeProductServer) (pb.ProductServiceClient, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterProductServiceServer(grpcServer, fake)
	go grpcServer.Serve(listener)

	conn, err := grpc.Dial(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial fake server: %v", err)
	}

	cleanup := func() {
		conn.Close()
		grpcServer.Stop()
	}
	return pb.NewProductServiceClient(conn), cleanup
}

// TestIntegration_DependencyAvailable: the dependency is available.
func TestIntegration_DependencyAvailable(t *testing.T) {
	client, cleanup := startFakeProductService(t, &fakeProductServer{product: sampleProduct})
	defer cleanup()

	got, err := timeoutGetProduct(client, "P001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.GetName() != "Laptop" {
		t.Errorf("got %+v, want Laptop", got)
	}
}

// TestIntegration_DependencyUnavailable: the dependency is unavailable.
// A port is reserved and then immediately closed, so the address is real
// but nothing is listening on it - a genuine "connection refused" over
// the network, not a scripted error.
func TestIntegration_DependencyUnavailable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer conn.Close()
	client := pb.NewProductServiceClient(conn)

	original := retryBaseBackoff
	retryBaseBackoff = 10 * time.Millisecond
	defer func() { retryBaseBackoff = original }()

	_, err = retryGetProduct(client, "P001")
	if err == nil {
		t.Fatal("expected an error when the dependency is unavailable")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %s, want Unavailable", st.Code())
	}
}

// TestIntegration_DependencyReturnsError: the dependency returns an error.
func TestIntegration_DependencyReturnsError(t *testing.T) {
	client, cleanup := startFakeProductService(t, &fakeProductServer{
		failWith: status.Error(codes.NotFound, "product with ID P999 not found"),
	})
	defer cleanup()

	_, err := timeoutGetProduct(client, "P999")
	if err == nil {
		t.Fatal("expected a NotFound error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("got code %s, want NotFound", st.Code())
	}
}

// TestIntegration_DependencyExceedsTimeout: the dependency takes longer
// to respond than the configured timeout.
func TestIntegration_DependencyExceedsTimeout(t *testing.T) {
	original := timeoutDuration
	timeoutDuration = 100 * time.Millisecond
	defer func() { timeoutDuration = original }()

	client, cleanup := startFakeProductService(t, &fakeProductServer{
		delay:   500 * time.Millisecond, // slower than the 100ms client timeout
		product: sampleProduct,
	})
	defer cleanup()

	_, err := timeoutGetProduct(client, "P001")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.DeadlineExceeded {
		t.Errorf("got code %s, want DeadlineExceeded", st.Code())
	}
}
