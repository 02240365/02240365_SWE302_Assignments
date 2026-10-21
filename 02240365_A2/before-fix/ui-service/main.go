package main

// =====================================================================
// Lab 4, Part A - UI Service, extended into an API Gateway
// =====================================================================
//
// This service now does two jobs:
//
//  1. Serves the web UI (static/index.html, style.css, app.js).
//  2. Acts as the system's API Gateway: it is the ONLY thing the browser
//     talks to. Every one of the ten backend gRPC services (Product,
//     Customer, Inventory, Payment, Notification and, from Part 2, Cart,
//     Review, Shipping, Coupon, Order History) is only ever reached
//     through this gateway - the browser never dials gRPC directly, and
//     the gateway never reads any service's data store directly either.
//
// Each downstream dependency gets its OWN circuit breaker, tracked
// independently - the Payment Service being down should not trip the
// breaker guarding the Product Service, for example. All five go through
// the same generic, resilient call helper (callResilient), which applies
// the same Timeout + Retry + Circuit Breaker approach used in Lab 3,
// just written once and reused instead of copy-pasted five times.
//
// REST surface exposed to the browser:
//   GET  /api/products/{id}   - look up a single product
//   POST /api/orders          - place an order (orchestrates all five services)
//
// Run with:  go run ./ui-service            (from the project root)
// Then open: http://localhost:8080
// =====================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	pb "ecommerce-microservices/proto"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Addresses of the ten backend services this gateway fronts (the first
// five come from Labs 2-5, the last five were added in Part 2).
const (
	productServiceAddr      = "localhost:50051"
	inventoryServiceAddr    = "localhost:50052"
	customerServiceAddr     = "localhost:50053"
	paymentServiceAddr      = "localhost:50054"
	notificationServiceAddr = "localhost:50055"
	cartServiceAddr         = "localhost:50056"
	reviewServiceAddr       = "localhost:50057"
	shippingServiceAddr     = "localhost:50058"
	couponServiceAddr       = "localhost:50059"
	historyServiceAddr      = "localhost:50060"
)

// Tunable timings for the gateway's resilient calls. Kept short since a
// person is waiting on the other end of a browser request - unlike the
// order-service CLI demo (Lab 3), which uses long, human-scale delays.
var (
	gwCallTimeout  = 2 * time.Second
	gwMaxRetries   = 2
	gwRetryBackoff = 500 * time.Millisecond
)

// ---------------------------------------------------------------------
// Circuit Breaker (same CLOSED / OPEN / HALF_OPEN design as order-service)
// ---------------------------------------------------------------------

type circuitState string

const (
	stateClosed   circuitState = "CLOSED"
	stateOpen     circuitState = "OPEN"
	stateHalfOpen circuitState = "HALF_OPEN"
)

type circuitBreaker struct {
	mu               sync.Mutex
	name             string
	state            circuitState
	failureCount     int
	failureThreshold int
	openTimeout      time.Duration
	lastFailureTime  time.Time
}

func newCircuitBreaker(name string, threshold int, openTimeout time.Duration) *circuitBreaker {
	return &circuitBreaker{name: name, state: stateClosed, failureThreshold: threshold, openTimeout: openTimeout}
}

func (cb *circuitBreaker) setState(s circuitState) {
	if cb.state != s {
		log.Printf("[CircuitBreaker:%s] %s -> %s", cb.name, cb.state, s)
		cb.state = s
	}
}

func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == stateOpen {
		if time.Since(cb.lastFailureTime) >= cb.openTimeout {
			cb.setState(stateHalfOpen)
			return true
		}
		return false
	}
	return true
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	if cb.state == stateHalfOpen {
		cb.setState(stateClosed)
	}
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.lastFailureTime = time.Now()
	if cb.state == stateHalfOpen {
		cb.setState(stateOpen)
		return
	}
	cb.failureCount++
	if cb.failureCount >= cb.failureThreshold {
		cb.setState(stateOpen)
	}
}

// One breaker per downstream dependency, so a failure in one service
// never affects how the gateway treats another.
var (
	productBreaker      = newCircuitBreaker("Product", 3, 8*time.Second)
	customerBreaker     = newCircuitBreaker("Customer", 3, 8*time.Second)
	inventoryBreaker    = newCircuitBreaker("Inventory", 3, 8*time.Second)
	paymentBreaker      = newCircuitBreaker("Payment", 3, 8*time.Second)
	notificationBreaker = newCircuitBreaker("Notification", 3, 8*time.Second)
)

// ---------------------------------------------------------------------
// Shared resilient call helper (Timeout + Retry + Circuit Breaker)
// ---------------------------------------------------------------------

// callResilient wraps a single downstream gRPC call with all three Lab 3
// patterns: the breaker decides whether to attempt the call at all; each
// attempt is bounded by gwCallTimeout; failed attempts are retried with a
// short backoff. NotFound / InvalidArgument / FailedPrecondition are
// treated as the dependency working correctly and just reporting a
// business-level outcome, so they are returned immediately without being
// retried or counted as a breaker failure.
func callResilient[T any](breaker *circuitBreaker, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T

	if !breaker.allow() {
		return zero, status.Error(codes.Unavailable, "circuit breaker is open - dependency assumed unavailable")
	}

	var lastErr error
	for attempt := 1; attempt <= gwMaxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), gwCallTimeout)
		result, err := fn(ctx)
		cancel()

		if err == nil {
			breaker.recordSuccess()
			return result, nil
		}

		st, _ := status.FromError(err)
		switch st.Code() {
		case codes.NotFound, codes.InvalidArgument, codes.FailedPrecondition, codes.AlreadyExists:
			return zero, err
		}

		lastErr = err
		log.Printf("[Retry:%s] attempt %d/%d failed: %s", breaker.name, attempt, gwMaxRetries, st.Message())
		if attempt < gwMaxRetries {
			time.Sleep(gwRetryBackoff)
		}
	}

	breaker.recordFailure()
	return zero, lastErr
}

// ---------------------------------------------------------------------
// Per-service gateway clients, backed by callResilient
// ---------------------------------------------------------------------

type gatewayClients struct {
	product      pb.ProductServiceClient
	customer     pb.CustomerServiceClient
	inventory    pb.InventoryServiceClient
	payment      pb.PaymentServiceClient
	notification pb.NotificationServiceClient

	// Part 2
	cart     pb.CartServiceClient
	review   pb.ReviewServiceClient
	shipping pb.ShippingServiceClient
	coupon   pb.CouponServiceClient
	history  pb.OrderHistoryServiceClient
}

func (g *gatewayClients) getProduct(id string) (*pb.Product, error) {
	return callResilient(productBreaker, func(ctx context.Context) (*pb.Product, error) {
		return g.product.GetProduct(ctx, &pb.GetProductRequest{ProductId: id})
	})
}

func (g *gatewayClients) getCustomer(id string) (*pb.Customer, error) {
	return callResilient(customerBreaker, func(ctx context.Context) (*pb.Customer, error) {
		return g.customer.GetCustomer(ctx, &pb.GetCustomerRequest{CustomerId: id})
	})
}

func (g *gatewayClients) decreaseStock(productID string, qty int32) (*pb.StockResponse, error) {
	return callResilient(inventoryBreaker, func(ctx context.Context) (*pb.StockResponse, error) {
		return g.inventory.DecreaseStock(ctx, &pb.DecreaseStockRequest{ProductId: productID, Quantity: qty})
	})
}

func (g *gatewayClients) processPayment(orderID, customerID string, amount float64) (*pb.PaymentResponse, error) {
	return callResilient(paymentBreaker, func(ctx context.Context) (*pb.PaymentResponse, error) {
		return g.payment.ProcessPayment(ctx, &pb.PaymentRequest{OrderId: orderID, CustomerId: customerID, Amount: amount})
	})
}

// collectNotifications reads every message off the server-streaming
// SubscribeNotifications call and returns them as a plain slice, since a
// simple JSON REST response can't stream the way gRPC can. A single
// timeout bounds the whole subscription (no retry/breaker here - a
// notification feed failing outright doesn't block the order, which has
// already succeeded by the time this runs).
func (g *gatewayClients) collectNotifications(customerID, orderID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := g.notification.SubscribeNotifications(ctx, &pb.SubscribeRequest{CustomerId: customerID, OrderId: orderID})
	if err != nil {
		notificationBreaker.recordFailure()
		return nil, err
	}

	var messages []string
	for {
		notification, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			notificationBreaker.recordFailure()
			return messages, err
		}
		messages = append(messages, fmt.Sprintf("[%s] %s", notification.GetTimestamp(), notification.GetMessage()))
	}
	notificationBreaker.recordSuccess()
	return messages, nil
}

// ---------------------------------------------------------------------
// Lab 5: AWS Lambda - order discount calculation (6th dependency)
// ---------------------------------------------------------------------

var lambdaBreaker = newCircuitBreaker("Lambda", 3, 8*time.Second)

type discountRequest struct {
	ProductID string  `json:"productId"`
	UnitPrice float64 `json:"unitPrice"`
	Quantity  int32   `json:"quantity"`
}

type discountResult struct {
	Success         bool    `json:"success"`
	ErrorCode       string  `json:"errorCode,omitempty"`
	Message         string  `json:"message,omitempty"`
	ProductID       string  `json:"productId"`
	Quantity        int32   `json:"quantity"`
	UnitPrice       float64 `json:"unitPrice"`
	Subtotal        float64 `json:"subtotal"`
	DiscountPercent float64 `json:"discountPercent"`
	DiscountAmount  float64 `json:"discountAmount"`
	FinalTotal      float64 `json:"finalTotal"`
}

// calculateDiscount invokes the AWS Lambda function with the same
// Timeout + Retry + Circuit Breaker approach already used for the other
// five dependencies.
func calculateDiscount(lambdaClient *lambda.Client, productID string, unitPrice float64, quantity int32) (*discountResult, error) {
	// Part 2: when no Lambda function is configured (for example on a
	// machine without AWS credentials) the same tier rules are applied
	// locally, so the storefront and the Cypress tests still run.
	if os.Getenv("LAMBDA_FUNCTION_NAME") == "" {
		return localDiscount(productID, unitPrice, quantity), nil
	}
	return callResilient(lambdaBreaker, func(ctx context.Context) (*discountResult, error) {
		payload, err := json.Marshal(discountRequest{ProductID: productID, UnitPrice: unitPrice, Quantity: quantity})
		if err != nil {
			return nil, err
		}

		functionName := os.Getenv("LAMBDA_FUNCTION_NAME")
		out, err := lambdaClient.Invoke(ctx, &lambda.InvokeInput{
			FunctionName: aws.String(functionName),
			Payload:      payload,
		})
		if err != nil {
			if strings.Contains(err.Error(), "AccessDenied") || strings.Contains(err.Error(), "not authorized") {
				log.Printf("[Lambda] permission error - check the IAM policy attached to the caller: %v", err)
			}
			return nil, err
		}
		if out.FunctionError != nil {
			return nil, fmt.Errorf("lambda function error: %s", string(out.Payload))
		}

		var result discountResult
		if err := json.Unmarshal(out.Payload, &result); err != nil {
			return nil, err
		}
		return &result, nil
	})
}

// ---------------------------------------------------------------------
// HTTP layer: /api/products/{id}
// ---------------------------------------------------------------------

type productResponse struct {
	Found     bool    `json:"found"`
	ProductID string  `json:"productId,omitempty"`
	Name      string  `json:"name,omitempty"`
	Price     float64 `json:"price,omitempty"`
	ErrorCode string  `json:"errorCode,omitempty"`
	Message   string  `json:"message,omitempty"`
}

// catalogProductIDs is the set of product IDs the storefront grid
// displays. ProductService only exposes a single-item GetProduct lookup
// (no ListProducts RPC), so the gateway aggregates the catalog itself by
// calling GetProduct once per known ID. If your Product Service's sample
// data uses different IDs, update this list to match.
var catalogProductIDs = []string{"P001", "P002", "P003", "P004"}

type catalogResponse struct {
	Products []productResponse `json:"products"`
}

// productsListHandler backs GET /api/products (no id): it looks up every
// ID in catalogProductIDs through the same resilient client used for a
// single lookup, and returns whichever ones succeeded. A product that
// fails (e.g. a transient dependency error) is simply left out of the
// response rather than failing the whole page - the storefront shows a
// partial catalog instead of an empty one.
func productsListHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var results []productResponse
		for _, id := range catalogProductIDs {
			product, err := g.getProduct(id)
			if err != nil {
				log.Printf("catalog: skipping %s: %v", id, err)
				continue
			}
			results = append(results, productResponse{
				Found: true, ProductID: product.GetProductId(), Name: product.GetName(), Price: product.GetPrice(),
			})
		}
		writeJSON(w, http.StatusOK, catalogResponse{Products: results})
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(body)
}

// dependencyErrorCode maps a gRPC status code to the small set of error
// codes the front-end understands, so every endpoint reports failures
// the same way.
func dependencyErrorCode(err error) (httpStatus int, code, fallbackMessage string) {
	st, _ := status.FromError(err)
	switch st.Code() {
	case codes.NotFound:
		return http.StatusNotFound, "not_found", "The requested record does not exist."
	case codes.InvalidArgument:
		return http.StatusBadRequest, "invalid_input", "Please check your input and try again."
	case codes.FailedPrecondition:
		return http.StatusConflict, "failed_precondition", st.Message()
	case codes.AlreadyExists:
		return http.StatusConflict, "already_exists", st.Message()
	default:
		return http.StatusServiceUnavailable, "unavailable", "A dependent service is currently unavailable. Please try again shortly."
	}
}

func productHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/products/")
		if id == "" {
			id = r.URL.Query().Get("id")
		}
		if id == "" {
			writeJSON(w, http.StatusBadRequest, productResponse{Found: false, ErrorCode: "invalid_input", Message: "Please enter a product ID."})
			return
		}

		product, err := g.getProduct(id)
		if err != nil {
			httpStatus, code, msg := dependencyErrorCode(err)
			if code == "not_found" {
				msg = fmt.Sprintf("Product '%s' does not exist.", id)
			}
			writeJSON(w, httpStatus, productResponse{Found: false, ErrorCode: code, Message: msg})
			return
		}

		writeJSON(w, http.StatusOK, productResponse{
			Found: true, ProductID: product.GetProductId(), Name: product.GetName(), Price: product.GetPrice(),
		})
	}
}

func mustDial(address string) *grpc.ClientConn {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to %s: %v", address, err)
	}
	return conn
}

func main() {
	productConn := mustDial(productServiceAddr)
	defer productConn.Close()
	customerConn := mustDial(customerServiceAddr)
	defer customerConn.Close()
	inventoryConn := mustDial(inventoryServiceAddr)
	defer inventoryConn.Close()
	paymentConn := mustDial(paymentServiceAddr)
	defer paymentConn.Close()
	notificationConn := mustDial(notificationServiceAddr)
	defer notificationConn.Close()

	cartConn := mustDial(cartServiceAddr)
	defer cartConn.Close()
	reviewConn := mustDial(reviewServiceAddr)
	defer reviewConn.Close()
	shippingConn := mustDial(shippingServiceAddr)
	defer shippingConn.Close()
	couponConn := mustDial(couponServiceAddr)
	defer couponConn.Close()
	historyConn := mustDial(historyServiceAddr)
	defer historyConn.Close()

	// --- Lab 5: AWS Lambda client ---
	// Credentials and region come from the environment
	// (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION) - never
	// hardcoded here.
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	lambdaClient := lambda.NewFromConfig(awsCfg)

	gateway := &gatewayClients{
		product:      pb.NewProductServiceClient(productConn),
		customer:     pb.NewCustomerServiceClient(customerConn),
		inventory:    pb.NewInventoryServiceClient(inventoryConn),
		payment:      pb.NewPaymentServiceClient(paymentConn),
		notification: pb.NewNotificationServiceClient(notificationConn),
		cart:         pb.NewCartServiceClient(cartConn),
		review:       pb.NewReviewServiceClient(reviewConn),
		shipping:     pb.NewShippingServiceClient(shippingConn),
		coupon:       pb.NewCouponServiceClient(couponConn),
		history:      pb.NewOrderHistoryServiceClient(historyConn),
	}
	conns := map[string]*grpc.ClientConn{
		"product": productConn, "inventory": inventoryConn, "customer": customerConn, "payment": paymentConn,
		"notification": notificationConn, "cart": cartConn, "review": reviewConn, "shipping": shippingConn,
		"coupon": couponConn, "history": historyConn,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/products", productsListHandler(gateway)) // exact match: GET /api/products (catalog)
	mux.HandleFunc("/api/products/", productHandler(gateway))     // subtree: GET /api/products/{id}
	mux.HandleFunc("/api/orders", ordersHandler(gateway, lambdaClient))
	mux.HandleFunc("/api/orders/", orderByIDHandler(gateway))
	registerPart2Routes(mux, gateway, lambdaClient, conns)
	mux.Handle("/", http.FileServer(http.Dir("ui-service/static")))

	const addr = ":8080"
	log.Printf("Gateway / UI Service running at http://localhost%s", addr)
	log.Printf("Fronting: Product(%s) Customer(%s) Inventory(%s) Payment(%s) Notification(%s) Lambda(%s)",
		productServiceAddr, customerServiceAddr, inventoryServiceAddr, paymentServiceAddr, notificationServiceAddr,
		os.Getenv("LAMBDA_FUNCTION_NAME"))
	log.Fatal(http.ListenAndServe(addr, mux))
}
