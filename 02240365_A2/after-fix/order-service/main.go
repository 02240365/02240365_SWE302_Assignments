package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// =====================================================================
// Lab 3 - Resilience Patterns
// =====================================================================
//
// Three resilience patterns are implemented below, each one completely
// independent of the other two, with its own function and its own CLI
// command:
//
//   1. Timeout          -> timeoutGetProduct()          -> "timeout" command
//   2. Retry              -> retryGetProduct()           -> "retry" command
//   3. Circuit Breaker    -> CircuitBreaker + breakerGetProduct() -> "circuitbreaker" command
//
// Unlike a typical demo that fakes failures inside the server, these
// patterns are exercised against the REAL Lab 2 Product Service and
// REAL products (e.g. P001). Failures are produced by actually
// stopping, pausing, or restarting the product-service process while
// the order-service is calling it - see the Lab 3 report / README for
// the exact terminal actions to take, and when. The backoff and
// cooldown durations below are deliberately generous (several seconds)
// to give enough real time to stop/restart the server by hand between
// attempts. The original Part A and Part B commands from Lab 2 are
// unchanged.
//
// Lab 4 change: the timing constants below were changed from `const` to
// `var`. Nothing about their default values or the CLI demo's behaviour
// changed - this only makes them overridable from a _test.go file in the
// same package, so unit tests can shrink a 3s backoff down to a few
// milliseconds and run in a fraction of a second instead of waiting
// through real, human-scale delays. See order-service/unit_test.go.
// =====================================================================

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "place":
		// Part B: go run ./order-service place <customer_id> <product_id> <quantity>
		if len(os.Args) != 5 {
			printUsage()
			os.Exit(1)
		}
		placeOrder(os.Args[2], os.Args[3], os.Args[4])
		return

	case "timeout":
		// Lab 3, Pattern 1: go run ./order-service timeout <product_id>
		if len(os.Args) != 3 {
			printUsage()
			os.Exit(1)
		}
		runTimeoutDemo(os.Args[2])
		return

	case "retry":
		// Lab 3, Pattern 2: go run ./order-service retry <product_id>
		if len(os.Args) != 3 {
			printUsage()
			os.Exit(1)
		}
		runRetryDemo(os.Args[2])
		return

	case "circuitbreaker":
		// Lab 3, Pattern 3: go run ./order-service circuitbreaker <product_id> <num_calls> <delay_seconds>
		if len(os.Args) != 5 {
			printUsage()
			os.Exit(1)
		}
		numCalls, err1 := strconv.Atoi(os.Args[3])
		delaySeconds, err2 := strconv.Atoi(os.Args[4])
		if err1 != nil || err2 != nil || numCalls <= 0 || delaySeconds < 0 {
			printUsage()
			os.Exit(1)
		}
		runCircuitBreakerDemo(os.Args[2], numCalls, delaySeconds)
		return
	}

	// Part A (kept working): go run ./order-service <product_id>
	getProduct(os.Args[1])
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  go run ./order-service <product_id>")
	fmt.Println("      Part A - fetch a single product from the Product Service")
	fmt.Println("  go run ./order-service place <customer_id> <product_id> <quantity>")
	fmt.Println("      Part B - place a full order across all microservices")
	fmt.Println("  go run ./order-service timeout <product_id>")
	fmt.Println("      Lab 3, Pattern 1 - single call bounded by a context timeout")
	fmt.Println("  go run ./order-service retry <product_id>")
	fmt.Println("      Lab 3, Pattern 2 - single call retried on failure with backoff")
	fmt.Println("  go run ./order-service circuitbreaker <product_id> <num_calls> <delay_seconds>")
	fmt.Println("      Lab 3, Pattern 3 - repeated calls through a circuit breaker")
	fmt.Println("      Use a real product ID, e.g. P001, and stop/restart the")
	fmt.Println("      Product Service at the right moment to trigger real failures.")
}

// connect opens an insecure (no TLS) gRPC connection to the given address.
// Insecure credentials are fine here because everything runs on localhost.
func connect(address string) *grpc.ClientConn {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to %s: %v", address, err)
	}
	return conn
}

// getProduct reproduces the original Part A behaviour: look up a single
// product from the Product Service and print its details.
func getProduct(productID string) {
	conn := connect("localhost:50051")
	defer conn.Close()

	client := pb.NewProductServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	product, err := client.GetProduct(ctx, &pb.GetProductRequest{ProductId: productID})
	if err != nil {
		st, _ := status.FromError(err)
		fmt.Printf("Error fetching product: %s\n", st.Message())
		return
	}

	fmt.Println("Product ID:", product.GetProductId())
	fmt.Println("Name:", product.GetName())
	fmt.Println("Price:", product.GetPrice())
}

// =====================================================================
// PATTERN 1: TIMEOUT (standalone, no retry, no circuit breaker)
// =====================================================================

// Lab 4: changed from `const` to `var` so tests can override it (see
// header comment above). Default value is unchanged from Lab 3.
var timeoutDuration = 2 * time.Second

// timeoutGetProduct makes exactly ONE call to the Product Service,
// bounded by a 2 second context timeout. If the server does not
// respond in time, the call is cancelled and a DeadlineExceeded error
// is returned - nothing is retried here, this function only
// demonstrates the Timeout pattern in isolation.
//
// To trigger a REAL timeout: freeze the Product Service process with
// SIGSTOP just before running this command (see README), so its TCP
// connection stays open but it never replies. Resume it afterwards
// with SIGCONT.
func timeoutGetProduct(client pb.ProductServiceClient, productID string) (*pb.Product, error) {
	fmt.Printf("Calling GetProduct(%s) with a %s timeout...\n", productID, timeoutDuration)

	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	start := time.Now()
	product, err := client.GetProduct(ctx, &pb.GetProductRequest{ProductId: productID})
	elapsed := time.Since(start)

	if err != nil {
		st, _ := status.FromError(err)
		if st.Code() == codes.DeadlineExceeded {
			fmt.Printf("Request TIMED OUT after %s (client gave up waiting)\n", elapsed.Round(time.Millisecond))
		} else {
			fmt.Printf("Request failed after %s: %s\n", elapsed.Round(time.Millisecond), st.Message())
		}
		return nil, err
	}

	fmt.Printf("Request succeeded after %s\n", elapsed.Round(time.Millisecond))
	return product, nil
}

func runTimeoutDemo(productID string) {
	conn := connect("localhost:50051")
	defer conn.Close()
	client := pb.NewProductServiceClient(conn)

	fmt.Println("===== PATTERN 1: TIMEOUT =====")
	product, err := timeoutGetProduct(client, productID)
	if err != nil {
		fmt.Println("Result: FAILED -", err)
		return
	}
	fmt.Printf("Result: SUCCESS - %s | Nu. %.2f\n", product.GetName(), product.GetPrice())
}

// =====================================================================
// PATTERN 2: RETRY (standalone, no timeout pattern, no circuit breaker)
// =====================================================================

// Lab 4: all three changed from `const` to `var` so tests can override
// them (see header comment above). Default values are unchanged from Lab 3.
var (
	maxRetries       = 3
	retryBaseBackoff = 3 * time.Second // deliberately generous - gives you real time to restart the server between attempts
	retryCallTimeout = 5 * time.Second // generous per-attempt timeout, so this call is not exercising the Timeout pattern
)

// retryGetProduct calls the Product Service up to maxRetries times,
// waiting an increasing backoff between attempts, until it either
// succeeds or runs out of attempts. This function has no circuit
// breaker - it always tries the server directly.
//
// To trigger a REAL retry-then-recover: stop the Product Service
// before running this command, then start it again (go run
// ./product-service) during one of the "Retrying in Ns..." pauses so
// the next attempt reaches a live server and succeeds.
func retryGetProduct(client pb.ProductServiceClient, productID string) (*pb.Product, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), retryCallTimeout)
		product, err := client.GetProduct(ctx, &pb.GetProductRequest{ProductId: productID})
		cancel()

		if err == nil {
			fmt.Printf("Attempt %d/%d SUCCEEDED\n", attempt, maxRetries)
			return product, nil
		}

		lastErr = err
		st, _ := status.FromError(err)
		fmt.Printf("Attempt %d/%d FAILED: %s\n", attempt, maxRetries, st.Message())

		if attempt < maxRetries {
			backoff := time.Duration(attempt) * retryBaseBackoff
			fmt.Printf("Retrying in %s...\n", backoff)
			time.Sleep(backoff)
		}
	}

	return nil, lastErr
}

func runRetryDemo(productID string) {
	conn := connect("localhost:50051")
	defer conn.Close()
	client := pb.NewProductServiceClient(conn)

	fmt.Println("===== PATTERN 2: RETRY =====")
	product, err := retryGetProduct(client, productID)
	if err != nil {
		fmt.Println("Result: FAILED after all retries -", err)
		return
	}
	fmt.Printf("Result: SUCCESS - %s | Nu. %.2f\n", product.GetName(), product.GetPrice())
}

// =====================================================================
// PATTERN 3: CIRCUIT BREAKER (standalone, single attempt per call,
// no retry loop, so only breaker behaviour is being exercised)
// =====================================================================

type CircuitState string

const (
	StateClosed   CircuitState = "CLOSED"
	StateOpen     CircuitState = "OPEN"
	StateHalfOpen CircuitState = "HALF_OPEN"
)

// CircuitBreaker is a simple, dependency-free circuit breaker.
//
//   - CLOSED:     requests pass through normally. Consecutive failures
//                 are counted; reaching failureThreshold trips the
//                 breaker to OPEN.
//   - OPEN:       requests are rejected immediately without calling the
//                 downstream service. After openTimeout has elapsed,
//                 the next request is allowed through as a trial and
//                 the breaker moves to HALF_OPEN.
//   - HALF_OPEN:  a single trial request is allowed. If it succeeds,
//                 the breaker closes (service has recovered). If it
//                 fails, the breaker re-opens and the cooldown restarts.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failureCount     int
	failureThreshold int
	openTimeout      time.Duration
	lastFailureTime  time.Time
}

func NewCircuitBreaker(failureThreshold int, openTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		openTimeout:      openTimeout,
	}
}

// setState changes state and prints the transition, satisfying the
// requirement to clearly show CLOSED / OPEN / HALF_OPEN in the terminal.
func (cb *CircuitBreaker) setState(newState CircuitState) {
	if cb.state != newState {
		fmt.Printf(">>> Circuit Breaker state change: %s -> %s\n", cb.state, newState)
		cb.state = newState
	}
}

// Allow reports whether a request may proceed, and performs the
// OPEN -> HALF_OPEN transition once the cooldown has elapsed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen {
		if time.Since(cb.lastFailureTime) >= cb.openTimeout {
			cb.setState(StateHalfOpen)
			return true
		}
		return false
	}
	return true
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	if cb.state == StateHalfOpen {
		cb.setState(StateClosed)
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	if cb.state == StateHalfOpen {
		// The trial request failed - the dependency has not recovered.
		cb.setState(StateOpen)
		return
	}

	cb.failureCount++
	if cb.failureCount >= cb.failureThreshold {
		cb.setState(StateOpen)
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Lab 4: changed from `const` to `var` so tests can override it.
// Default value is unchanged from Lab 3.
var breakerCallTimeout = 3 * time.Second // generous, this demo is not exercising the Timeout pattern

// breakerGetProduct makes exactly ONE attempt (no retry loop) to call
// the Product Service, gated by the circuit breaker. This isolates
// Circuit Breaker behaviour from the Retry pattern.
func breakerGetProduct(client pb.ProductServiceClient, cb *CircuitBreaker, productID string) (*pb.Product, error) {
	if !cb.Allow() {
		fmt.Printf("Circuit Breaker is OPEN - request for %s rejected without calling Product Service\n", productID)
		return nil, fmt.Errorf("circuit breaker is open")
	}

	ctx, cancel := context.WithTimeout(context.Background(), breakerCallTimeout)
	defer cancel()

	product, err := client.GetProduct(ctx, &pb.GetProductRequest{ProductId: productID})
	if err != nil {
		st, _ := status.FromError(err)
		fmt.Printf("Call failed: %s\n", st.Message())
		cb.RecordFailure()
		return nil, err
	}

	cb.RecordSuccess()
	return product, nil
}

// runCircuitBreakerDemo repeatedly calls the Product Service through
// breakerGetProduct, sleeping delaySeconds between each call, so the
// breaker's CLOSED / OPEN / HALF_OPEN transitions can be observed live
// over several calls in a single run.
//
// To trigger a REAL open-then-recover cycle: stop the Product Service
// before running this command against a real product (e.g. P001).
// Once the terminal shows the breaker has gone OPEN, restart the
// Product Service (go run ./product-service) - it should be back up
// well before the HALF_OPEN trial fires, letting the breaker close
// again on real, live data. To demonstrate the breaker staying OPEN
// instead, simply leave the server stopped for the whole run.
func runCircuitBreakerDemo(productID string, numCalls int, delaySeconds int) {
	conn := connect("localhost:50051")
	defer conn.Close()
	client := pb.NewProductServiceClient(conn)

	fmt.Println("===== PATTERN 3: CIRCUIT BREAKER =====")

	// Trips after 3 consecutive failures, stays OPEN for 8 seconds
	// (deliberately generous, to give real time to restart the server)
	// before allowing a HALF_OPEN trial request.
	cb := NewCircuitBreaker(3, 8*time.Second)

	for i := 1; i <= numCalls; i++ {
		fmt.Printf("\n----- Call %d/%d | product=%s | breaker=%s -----\n", i, numCalls, productID, cb.State())

		product, err := breakerGetProduct(client, cb, productID)
		if err != nil {
			fmt.Printf("Call %d/%d FAILED: %v\n", i, numCalls, err)
		} else {
			fmt.Printf("Call %d/%d SUCCESS: %s | Nu. %.2f\n", i, numCalls, product.GetName(), product.GetPrice())
		}

		if i < numCalls {
			time.Sleep(time.Duration(delaySeconds) * time.Second)
		}
	}

	fmt.Printf("\nDemo finished. Final circuit breaker state: %s\n", cb.State())
}

// placeOrder is the Part B workflow. The Order Service acts as an
// orchestrator, calling all five microservices in sequence to simulate
// a real order being placed:
//
//  1. Customer Service     - verify the customer exists          (unary RPC)
//  2. Product Service      - fetch product details and price     (unary RPC)
//  3. Inventory Service    - check and reserve stock              (unary RPC)
//  4. Payment Service      - charge the customer                 (unary RPC)
//  5. Notification Service - stream order status updates back    (server-streaming RPC)
//
// If any step fails, the gRPC status code and message returned by that
// service are surfaced to the user and the order stops there.
func placeOrder(customerID, productID, quantityStr string) {
	var quantity int32
	if _, err := fmt.Sscanf(quantityStr, "%d", &quantity); err != nil || quantity <= 0 {
		fmt.Println("Error: quantity must be a positive whole number")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- Step 1: Customer Service ---
	custConn := connect("localhost:50053")
	defer custConn.Close()
	custClient := pb.NewCustomerServiceClient(custConn)

	customer, err := custClient.GetCustomer(ctx, &pb.GetCustomerRequest{CustomerId: customerID})
	if err != nil {
		st, _ := status.FromError(err)
		fmt.Printf("Customer check failed: %s\n", st.Message())
		return
	}
	fmt.Printf("Step 1/5 - Customer verified: %s (%s)\n", customer.GetName(), customer.GetEmail())

	// --- Step 2: Product Service ---
	prodConn := connect("localhost:50051")
	defer prodConn.Close()
	prodClient := pb.NewProductServiceClient(prodConn)

	product, err := prodClient.GetProduct(ctx, &pb.GetProductRequest{ProductId: productID})
	if err != nil {
		st, _ := status.FromError(err)
		fmt.Printf("Product lookup failed: %s\n", st.Message())
		return
	}
	fmt.Printf("Step 2/5 - Product found: %s (Nu. %.2f each)\n", product.GetName(), product.GetPrice())

	// --- Step 3: Inventory Service ---
	invConn := connect("localhost:50052")
	defer invConn.Close()
	invClient := pb.NewInventoryServiceClient(invConn)

	stock, err := invClient.DecreaseStock(ctx, &pb.DecreaseStockRequest{ProductId: productID, Quantity: quantity})
	if err != nil {
		st, _ := status.FromError(err)
		fmt.Printf("Inventory update failed: %s\n", st.Message())
		return
	}
	fmt.Printf("Step 3/5 - Stock reserved. Remaining stock for %s: %d\n", productID, stock.GetQuantity())

	// --- Step 4: Payment Service ---
	payConn := connect("localhost:50054")
	defer payConn.Close()
	payClient := pb.NewPaymentServiceClient(payConn)

	totalAmount := product.GetPrice() * float64(quantity)
	orderID := fmt.Sprintf("ORD-%s-%s", customerID, productID)

	payment, err := payClient.ProcessPayment(ctx, &pb.PaymentRequest{
		OrderId:    orderID,
		CustomerId: customerID,
		Amount:     totalAmount,
	})
	if err != nil {
		st, _ := status.FromError(err)
		fmt.Printf("Payment failed: %s\n", st.Message())
		return
	}
	fmt.Printf("Step 4/5 - Payment successful. Payment ID: %s | Amount charged: Nu. %.2f\n", payment.GetPaymentId(), totalAmount)

	// --- Step 5: Notification Service (server-streaming RPC) ---
	notifConn := connect("localhost:50055")
	defer notifConn.Close()
	notifClient := pb.NewNotificationServiceClient(notifConn)

	// Use a fresh, longer-lived context for the stream since it takes a
	// few seconds to receive all the notification messages.
	streamCtx, streamCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer streamCancel()

	stream, err := notifClient.SubscribeNotifications(streamCtx, &pb.SubscribeRequest{
		CustomerId: customerID,
		OrderId:    orderID,
	})
	if err != nil {
		st, _ := status.FromError(err)
		fmt.Printf("Could not subscribe to notifications: %s\n", st.Message())
		return
	}

	fmt.Println("Step 5/5 - Order status updates:")
	for {
		notification, err := stream.Recv()
		if err == io.EOF {
			break // server closed the stream normally
		}
		if err != nil {
			st, _ := status.FromError(err)
			fmt.Printf("Notification stream error: %s\n", st.Message())
			break
		}
		fmt.Printf("  [%s] %s\n", notification.GetTimestamp(), notification.GetMessage())
	}

	fmt.Printf("\nOrder %s completed successfully.\n", orderID)
}
