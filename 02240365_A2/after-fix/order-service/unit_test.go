package main

// Lab 4, Part B: Unit tests for the three Lab 3 resilience functions
// (timeoutGetProduct, retryGetProduct, breakerGetProduct/CircuitBreaker).
//
// These use mockProductClient, a hand-written test double for
// pb.ProductServiceClient that never touches the network - each call to
// GetProduct just plays back a scripted response. That is what lets these
// tests run completely independently of the real Product Service and the
// rest of the running system.
//
// Note: timeoutDuration, retryBaseBackoff, and breakerCallTimeout are
// package-level `var`s (see main.go) specifically so tests can shrink
// them here and run in milliseconds instead of the real multi-second
// delays used by the CLI demo.
//
// Run with:  go test ./order-service/...

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockStep describes how the mock client should behave on one call:
// wait `delay` (if set), then return either `err` or `product`.
type mockStep struct {
	delay   time.Duration
	err     error
	product *pb.Product
}

// mockProductClient is a test double for pb.ProductServiceClient. Every
// call to GetProduct plays back the next scripted mockStep (the last
// step repeats if GetProduct is called more times than scripted).
type mockProductClient struct {
	mu    sync.Mutex
	steps []mockStep
	calls int
}

func (m *mockProductClient) GetProduct(ctx context.Context, in *pb.GetProductRequest, opts ...grpc.CallOption) (*pb.Product, error) {
	m.mu.Lock()
	i := m.calls
	m.calls++
	m.mu.Unlock()

	idx := i
	if idx >= len(m.steps) {
		idx = len(m.steps) - 1 // repeat the last scripted step if called more times than scripted
	}
	step := m.steps[idx]

	if step.delay > 0 {
		select {
		case <-time.After(step.delay):
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, "context deadline exceeded")
		}
	}
	if step.err != nil {
		return nil, step.err
	}
	return step.product, nil
}

func (m *mockProductClient) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

var sampleProduct = &pb.Product{ProductId: "P001", Name: "Laptop", Price: 75000}

// ---------------------------------------------------------------------
// Pattern 1: Timeout
// ---------------------------------------------------------------------

func TestTimeoutGetProduct_SucceedsWithinDeadline(t *testing.T) {
	mock := &mockProductClient{steps: []mockStep{{product: sampleProduct}}}

	got, err := timeoutGetProduct(mock, "P001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.GetName() != "Laptop" {
		t.Errorf("got %+v, want Laptop", got)
	}
}

func TestTimeoutGetProduct_TimesOutWhenServerIsSlow(t *testing.T) {
	// Shrink the timeout for this test only, then restore it.
	original := timeoutDuration
	timeoutDuration = 50 * time.Millisecond
	defer func() { timeoutDuration = original }()

	mock := &mockProductClient{steps: []mockStep{{delay: 500 * time.Millisecond, product: sampleProduct}}}

	_, err := timeoutGetProduct(mock, "P001")
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.DeadlineExceeded {
		t.Errorf("got code %s, want DeadlineExceeded", st.Code())
	}
}

// ---------------------------------------------------------------------
// Pattern 2: Retry
// ---------------------------------------------------------------------

func TestRetryGetProduct_SucceedsOnFirstAttempt(t *testing.T) {
	mock := &mockProductClient{steps: []mockStep{{product: sampleProduct}}}

	got, err := retryGetProduct(mock, "P001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.GetName() != "Laptop" {
		t.Errorf("got %+v, want Laptop", got)
	}
	if mock.callCount() != 1 {
		t.Errorf("expected exactly 1 call, got %d", mock.callCount())
	}
}

func TestRetryGetProduct_RecoversAfterTransientFailures(t *testing.T) {
	// Shrink the backoff for this test only, then restore it.
	original := retryBaseBackoff
	retryBaseBackoff = 10 * time.Millisecond
	defer func() { retryBaseBackoff = original }()

	unavailable := status.Error(codes.Unavailable, "product service temporarily unavailable")
	mock := &mockProductClient{steps: []mockStep{
		{err: unavailable},
		{err: unavailable},
		{product: sampleProduct},
	}}

	got, err := retryGetProduct(mock, "P001")
	if err != nil {
		t.Fatalf("expected retry to eventually succeed, got error: %v", err)
	}
	if got.GetName() != "Laptop" {
		t.Errorf("got %+v, want Laptop", got)
	}
	if mock.callCount() != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", mock.callCount())
	}
}

func TestRetryGetProduct_FailsAfterAllAttemptsExhausted(t *testing.T) {
	original := retryBaseBackoff
	retryBaseBackoff = 10 * time.Millisecond
	defer func() { retryBaseBackoff = original }()

	unavailable := status.Error(codes.Unavailable, "product service temporarily unavailable")
	mock := &mockProductClient{steps: []mockStep{{err: unavailable}}} // every call fails

	_, err := retryGetProduct(mock, "P001")
	if err == nil {
		t.Fatal("expected an error after exhausting all retries, got nil")
	}
	if mock.callCount() != maxRetries {
		t.Errorf("expected exactly %d attempts, got %d", maxRetries, mock.callCount())
	}
}

// ---------------------------------------------------------------------
// Pattern 3: Circuit Breaker
// ---------------------------------------------------------------------

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "down")
	mock := &mockProductClient{steps: []mockStep{{err: unavailable}}}
	cb := NewCircuitBreaker(2, 1*time.Second) // trips after 2 failures

	breakerGetProduct(mock, cb, "P001") // failure 1
	breakerGetProduct(mock, cb, "P001") // failure 2 -> should trip

	if cb.State() != StateOpen {
		t.Fatalf("expected breaker to be OPEN after 2 failures, got %s", cb.State())
	}

	callsBefore := mock.callCount()
	if _, err := breakerGetProduct(mock, cb, "P001"); err == nil {
		t.Fatal("expected the breaker to reject the call while OPEN")
	}
	if mock.callCount() != callsBefore {
		t.Error("breaker was OPEN but the mock client was still called")
	}
}

func TestCircuitBreaker_ClosesAfterSuccessfulHalfOpenTrial(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "down")
	mock := &mockProductClient{steps: []mockStep{
		{err: unavailable},
		{err: unavailable},
		{product: sampleProduct}, // the server has recovered by the time the trial runs
	}}
	cb := NewCircuitBreaker(2, 20*time.Millisecond) // short cooldown, for a fast test

	breakerGetProduct(mock, cb, "P001") // failure 1
	breakerGetProduct(mock, cb, "P001") // failure 2 -> OPEN

	time.Sleep(30 * time.Millisecond) // let the cooldown elapse

	got, err := breakerGetProduct(mock, cb, "P001") // HALF_OPEN trial - succeeds
	if err != nil {
		t.Fatalf("expected the half-open trial to succeed, got: %v", err)
	}
	if got.GetName() != "Laptop" {
		t.Errorf("got %+v, want Laptop", got)
	}
	if cb.State() != StateClosed {
		t.Errorf("expected breaker to be CLOSED after a successful trial, got %s", cb.State())
	}
}

func TestCircuitBreaker_ReopensAfterFailedHalfOpenTrial(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "down")
	mock := &mockProductClient{steps: []mockStep{{err: unavailable}}} // every call fails
	cb := NewCircuitBreaker(2, 20*time.Millisecond)

	breakerGetProduct(mock, cb, "P001") // failure 1
	breakerGetProduct(mock, cb, "P001") // failure 2 -> OPEN

	time.Sleep(30 * time.Millisecond)

	if _, err := breakerGetProduct(mock, cb, "P001"); err == nil { // HALF_OPEN trial - still fails
		t.Fatal("expected the half-open trial to fail against a still-down server")
	}
	if cb.State() != StateOpen {
		t.Errorf("expected breaker to be OPEN again after a failed trial, got %s", cb.State())
	}
}
