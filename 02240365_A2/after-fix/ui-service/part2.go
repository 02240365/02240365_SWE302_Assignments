package main

// Part 2 - gateway clients and REST endpoints for the five new services
// (Cart, Review, Shipping, Coupon, Order History), the local discount
// rules, the service-status endpoint and the test-mode reset.
//
// REST surface added in Part 2:
//   GET    /api/cart?customerId=            show the cart
//   POST   /api/cart                        {customerId, productId, quantity}  add
//   DELETE /api/cart/{productId}?customerId=  remove one product
//   DELETE /api/cart?customerId=            empty the cart
//   POST   /api/cart/checkout               {customerId, couponCode}  order every line
//   GET    /api/reviews/{productId}         reviews + average
//   POST   /api/reviews                     {productId, customerId, rating, comment}
//   POST   /api/coupons/validate            {customerId, code, subtotal}
//   GET    /api/shipping/quote?customerId=&subtotal=&itemCount=
//   GET    /api/shipping/track/{trackingId}
//   GET    /api/orders?customerId=          order history
//   GET    /api/orders/{orderId}            one order
//   GET    /api/services                    is each of the 10 services reachable?
//   POST   /api/test/reset                  test mode only (ENABLE_TEST_ENDPOINTS=1)

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	pb "ecommerce-microservices/proto"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

var (
	cartBreaker     = newCircuitBreaker("Cart", 3, 8*time.Second)
	reviewBreaker   = newCircuitBreaker("Review", 3, 8*time.Second)
	shippingBreaker = newCircuitBreaker("Shipping", 3, 8*time.Second)
	couponBreaker   = newCircuitBreaker("Coupon", 3, 8*time.Second)
	historyBreaker  = newCircuitBreaker("History", 3, 8*time.Second)
)

// ---------------------------------------------------------------------
// Clients (all go through callResilient: timeout + retry + breaker)
// ---------------------------------------------------------------------

func (g *gatewayClients) restoreStock(productID string, qty int32) (*pb.StockResponse, error) {
	return callResilient(inventoryBreaker, func(ctx context.Context) (*pb.StockResponse, error) {
		return g.inventory.RestoreStock(ctx, &pb.DecreaseStockRequest{ProductId: productID, Quantity: qty})
	})
}

func (g *gatewayClients) validateCoupon(code, customerID string, subtotal float64) (*pb.CouponResponse, error) {
	return callResilient(couponBreaker, func(ctx context.Context) (*pb.CouponResponse, error) {
		return g.coupon.ValidateCoupon(ctx, &pb.ValidateCouponRequest{Code: code, CustomerId: customerID, Subtotal: subtotal})
	})
}

func (g *gatewayClients) redeemCoupon(code, customerID, orderID string) error {
	_, err := callResilient(couponBreaker, func(ctx context.Context) (*pb.RedeemCouponResponse, error) {
		return g.coupon.RedeemCoupon(ctx, &pb.RedeemCouponRequest{Code: code, CustomerId: customerID, OrderId: orderID})
	})
	return err
}

func (g *gatewayClients) getQuote(address string, itemCount int32, subtotal float64) (*pb.QuoteResponse, error) {
	return callResilient(shippingBreaker, func(ctx context.Context) (*pb.QuoteResponse, error) {
		return g.shipping.GetQuote(ctx, &pb.QuoteRequest{Address: address, ItemCount: itemCount, Subtotal: subtotal})
	})
}

func (g *gatewayClients) createShipment(orderID, customerID, address string) (*pb.ShipmentMsg, error) {
	return callResilient(shippingBreaker, func(ctx context.Context) (*pb.ShipmentMsg, error) {
		return g.shipping.CreateShipment(ctx, &pb.CreateShipmentRequest{OrderId: orderID, CustomerId: customerID, Address: address})
	})
}

func (g *gatewayClients) trackShipment(id string) (*pb.ShipmentMsg, error) {
	return callResilient(shippingBreaker, func(ctx context.Context) (*pb.ShipmentMsg, error) {
		return g.shipping.TrackShipment(ctx, &pb.TrackShipmentRequest{TrackingId: id})
	})
}

func (g *gatewayClients) recordOrder(o *pb.OrderRecordMsg) (*pb.OrderRecordMsg, error) {
	return callResilient(historyBreaker, func(ctx context.Context) (*pb.OrderRecordMsg, error) {
		return g.history.RecordOrder(ctx, o)
	})
}

// ---------------------------------------------------------------------
// Local copy of the Lambda discount rules (used when no Lambda function
// name is configured). Must stay identical to lambda/index.mjs.
// ---------------------------------------------------------------------

func localDiscount(productID string, unitPrice float64, quantity int32) *discountResult {
	if productID == "" || unitPrice <= 0 || quantity <= 0 {
		return &discountResult{Success: false, ErrorCode: "invalid_input",
			Message: "productId, a positive unitPrice and a positive integer quantity are required."}
	}
	percent := 0.0
	switch {
	case quantity >= 5:
		percent = 15
	case quantity >= 3:
		percent = 10
	case quantity >= 2:
		percent = 5
	}
	subtotal := round2(unitPrice * float64(quantity))
	amount := round2(subtotal * percent / 100)
	return &discountResult{
		Success: true, ProductID: productID, Quantity: quantity, UnitPrice: unitPrice, Subtotal: subtotal,
		DiscountPercent: percent, DiscountAmount: amount, FinalTotal: round2(subtotal - amount),
	}
}

// ---------------------------------------------------------------------
// Shared HTTP helpers
// ---------------------------------------------------------------------

type apiError struct {
	Success   bool   `json:"success"`
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}

func writeErr(w http.ResponseWriter, err error) {
	httpStatus, code, msg := dependencyErrorCode(err)
	// The new services write messages meant for people ("coupon OLD20 has
	// expired"), so pass them through. Only "unavailable" keeps the
	// generic text, because its raw message describes the network fault.
	if code != "unavailable" {
		if st, ok := status.FromError(err); ok && st.Message() != "" {
			msg = st.Message()
		}
	}
	writeJSON(w, httpStatus, apiError{Success: false, ErrorCode: code, Message: msg})
}

func writeBad(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, apiError{Success: false, ErrorCode: "invalid_input", Message: msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		writeBad(w, "Malformed request body.")
		return false
	}
	return true
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeJSON(w, http.StatusMethodNotAllowed, apiError{Success: false, ErrorCode: "invalid_input", Message: "Use " + allowed + " for this endpoint."})
}

// ---------------------------------------------------------------------
// Cart
// ---------------------------------------------------------------------

type cartLineJSON struct {
	ProductID   string  `json:"productId"`
	ProductName string  `json:"productName"`
	UnitPrice   float64 `json:"unitPrice"`
	Quantity    int32   `json:"quantity"`
	LineTotal   float64 `json:"lineTotal"`
}

type cartJSON struct {
	Success    bool           `json:"success"`
	CustomerID string         `json:"customerId"`
	Lines      []cartLineJSON `json:"lines"`
	ItemCount  int32          `json:"itemCount"`
	Subtotal   float64        `json:"subtotal"`
}

func toCartJSON(c *pb.CartResponse) cartJSON {
	out := cartJSON{Success: true, CustomerID: c.GetCustomerId(), Lines: []cartLineJSON{}, ItemCount: c.GetItemCount(), Subtotal: c.GetSubtotal()}
	for _, l := range c.GetLines() {
		out.Lines = append(out.Lines, cartLineJSON{l.GetProductId(), l.GetProductName(), l.GetUnitPrice(), l.GetQuantity(), l.GetLineTotal()})
	}
	return out
}

func cartHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			customerID := strings.TrimSpace(r.URL.Query().Get("customerId"))
			if customerID == "" {
				writeBad(w, "Please enter your Customer ID.")
				return
			}
			if _, err := g.getCustomer(customerID); err != nil {
				writeCustomerErr(w, err, customerID)
				return
			}
			c, err := callResilient(cartBreaker, func(ctx context.Context) (*pb.CartResponse, error) {
				return g.cart.GetCart(ctx, &pb.CartCustomerRequest{CustomerId: customerID})
			})
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, toCartJSON(c))
		case http.MethodPost:
			var req struct {
				CustomerID string `json:"customerId"`
				ProductID  string `json:"productId"`
				Quantity   int32  `json:"quantity"`
			}
			if !decodeBody(w, r, &req) {
				return
			}
			req.CustomerID, req.ProductID = strings.TrimSpace(req.CustomerID), strings.TrimSpace(req.ProductID)
			if req.CustomerID == "" || req.ProductID == "" || req.Quantity <= 0 {
				writeBad(w, "Please provide a customer ID, a product ID, and a quantity greater than zero.")
				return
			}
			if _, err := g.getCustomer(req.CustomerID); err != nil {
				writeCustomerErr(w, err, req.CustomerID)
				return
			}
			product, err := g.getProduct(req.ProductID)
			if err != nil {
				writeNotFoundOr(w, err, fmt.Sprintf("Product '%s' does not exist.", req.ProductID))
				return
			}
			c, err := callResilient(cartBreaker, func(ctx context.Context) (*pb.CartResponse, error) {
				return g.cart.AddItem(ctx, &pb.AddCartItemRequest{
					CustomerId: req.CustomerID, ProductId: req.ProductID, ProductName: product.GetName(),
					UnitPrice: product.GetPrice(), Quantity: req.Quantity,
				})
			})
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, toCartJSON(c))
		case http.MethodDelete:
			customerID := strings.TrimSpace(r.URL.Query().Get("customerId"))
			if customerID == "" {
				writeBad(w, "Please enter your Customer ID.")
				return
			}
			c, err := callResilient(cartBreaker, func(ctx context.Context) (*pb.CartResponse, error) {
				return g.cart.ClearCart(ctx, &pb.CartCustomerRequest{CustomerId: customerID})
			})
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, toCartJSON(c))
		default:
			methodNotAllowed(w, "GET, POST or DELETE")
		}
	}
}

func cartItemHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			methodNotAllowed(w, "DELETE")
			return
		}
		productID := strings.TrimPrefix(r.URL.Path, "/api/cart/")
		customerID := strings.TrimSpace(r.URL.Query().Get("customerId"))
		if customerID == "" || productID == "" {
			writeBad(w, "Please provide the customer ID and the product ID.")
			return
		}
		c, err := callResilient(cartBreaker, func(ctx context.Context) (*pb.CartResponse, error) {
			return g.cart.RemoveItem(ctx, &pb.RemoveCartItemRequest{CustomerId: customerID, ProductId: productID})
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toCartJSON(c))
	}
}

type checkoutResponse struct {
	Success   bool            `json:"success"`
	ErrorCode string          `json:"errorCode,omitempty"`
	Message   string          `json:"message,omitempty"`
	Step      string          `json:"step,omitempty"`
	Orders    []orderResponse `json:"orders"`
	Total     float64         `json:"total"`
}

// checkoutHandler orders every line of the cart, one after another. If a
// line fails, the lines already ordered are removed from the cart, the
// remaining lines stay, and the response says which step failed. The
// coupon (if any) is applied to the first line only, because a coupon can
// be used once.
func checkoutHandler(g *gatewayClients, lambdaClient *lambda.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		var req struct {
			CustomerID string `json:"customerId"`
			CouponCode string `json:"couponCode"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		req.CustomerID = strings.TrimSpace(req.CustomerID)
		if req.CustomerID == "" {
			writeBad(w, "Please enter your Customer ID.")
			return
		}
		cart, err := callResilient(cartBreaker, func(ctx context.Context) (*pb.CartResponse, error) {
			return g.cart.GetCart(ctx, &pb.CartCustomerRequest{CustomerId: req.CustomerID})
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		if len(cart.GetLines()) == 0 {
			writeJSON(w, http.StatusConflict, checkoutResponse{ErrorCode: "failed_precondition", Message: "Your cart is empty.", Orders: []orderResponse{}})
			return
		}
		out := checkoutResponse{Orders: []orderResponse{}}
		coupon := req.CouponCode
		for _, line := range cart.GetLines() {
			code, resp := placeOrder(g, lambdaClient, orderRequest{
				CustomerID: req.CustomerID, ProductID: line.GetProductId(), Quantity: line.GetQuantity(), CouponCode: coupon,
			}, false)
			if !resp.Success {
				out.ErrorCode, out.Message, out.Step = resp.ErrorCode, resp.Message, resp.Step
				if resp.ProductName == "" {
					out.Message = fmt.Sprintf("%s: %s", line.GetProductName(), resp.Message)
				}
				writeJSON(w, code, out)
				return
			}
			coupon = ""
			out.Orders = append(out.Orders, resp)
			out.Total = round2(out.Total + resp.FinalTotal)
			if _, err := callResilient(cartBreaker, func(ctx context.Context) (*pb.CartResponse, error) {
				return g.cart.RemoveItem(ctx, &pb.RemoveCartItemRequest{CustomerId: req.CustomerID, ProductId: line.GetProductId()})
			}); err != nil {
				log.Printf("checkout: could not remove %s from the cart: %v", line.GetProductId(), err)
			}
		}
		out.Success = true
		writeJSON(w, http.StatusOK, out)
	}
}

func writeCustomerErr(w http.ResponseWriter, err error, id string) {
	writeNotFoundOr(w, err, fmt.Sprintf("Customer '%s' does not exist.", id))
}

func writeNotFoundOr(w http.ResponseWriter, err error, notFoundMsg string) {
	httpStatus, code, msg := dependencyErrorCode(err)
	if code == "not_found" {
		msg = notFoundMsg
	}
	writeJSON(w, httpStatus, apiError{Success: false, ErrorCode: code, Message: msg})
}

// ---------------------------------------------------------------------
// Reviews
// ---------------------------------------------------------------------

type reviewJSON struct {
	ReviewID     string `json:"reviewId"`
	ProductID    string `json:"productId"`
	CustomerID   string `json:"customerId"`
	CustomerName string `json:"customerName"`
	Rating       int32  `json:"rating"`
	Comment      string `json:"comment"`
	CreatedAt    string `json:"createdAt"`
}

type reviewListJSON struct {
	Success       bool         `json:"success"`
	ProductID     string       `json:"productId"`
	Reviews       []reviewJSON `json:"reviews"`
	AverageRating float64      `json:"averageRating"`
	ReviewCount   int32        `json:"reviewCount"`
}

func toReviewJSON(r *pb.ReviewMsg) reviewJSON {
	return reviewJSON{r.GetReviewId(), r.GetProductId(), r.GetCustomerId(), r.GetCustomerName(), r.GetRating(), r.GetComment(), r.GetCreatedAt()}
}

func reviewsListHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		productID := strings.TrimPrefix(r.URL.Path, "/api/reviews/")
		if productID == "" {
			writeBad(w, "Please choose a product.")
			return
		}
		if _, err := g.getProduct(productID); err != nil {
			writeNotFoundOr(w, err, fmt.Sprintf("Product '%s' does not exist.", productID))
			return
		}
		l, err := callResilient(reviewBreaker, func(ctx context.Context) (*pb.ListReviewsResponse, error) {
			return g.review.ListReviews(ctx, &pb.ListReviewsRequest{ProductId: productID})
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		out := reviewListJSON{Success: true, ProductID: productID, Reviews: []reviewJSON{}, AverageRating: l.GetAverageRating(), ReviewCount: l.GetReviewCount()}
		for _, rv := range l.GetReviews() {
			out.Reviews = append(out.Reviews, toReviewJSON(rv))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func reviewsAddHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		var req struct {
			ProductID  string `json:"productId"`
			CustomerID string `json:"customerId"`
			Rating     int32  `json:"rating"`
			Comment    string `json:"comment"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		req.ProductID, req.CustomerID = strings.TrimSpace(req.ProductID), strings.TrimSpace(req.CustomerID)
		if req.CustomerID == "" {
			writeBad(w, "Please enter your Customer ID to write a review.")
			return
		}
		customer, err := g.getCustomer(req.CustomerID)
		if err != nil {
			writeCustomerErr(w, err, req.CustomerID)
			return
		}
		if _, err := g.getProduct(req.ProductID); err != nil {
			writeNotFoundOr(w, err, fmt.Sprintf("Product '%s' does not exist.", req.ProductID))
			return
		}
		rv, err := callResilient(reviewBreaker, func(ctx context.Context) (*pb.ReviewMsg, error) {
			return g.review.AddReview(ctx, &pb.AddReviewRequest{
				ProductId: req.ProductID, CustomerId: req.CustomerID, CustomerName: customer.GetName(),
				Rating: req.Rating, Comment: req.Comment,
			})
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, struct {
			Success bool       `json:"success"`
			Review  reviewJSON `json:"review"`
		}{true, toReviewJSON(rv)})
	}
}

// ---------------------------------------------------------------------
// Coupons, shipping, tracking, history
// ---------------------------------------------------------------------

func couponValidateHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		var req struct {
			CustomerID string  `json:"customerId"`
			Code       string  `json:"code"`
			Subtotal   float64 `json:"subtotal"`
		}
		if !decodeBody(w, r, &req) {
			return
		}
		c, err := g.validateCoupon(req.Code, strings.TrimSpace(req.CustomerID), req.Subtotal)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Success        bool    `json:"success"`
			Code           string  `json:"code"`
			DiscountAmount float64 `json:"discountAmount"`
			Description    string  `json:"description"`
		}{true, c.GetCode(), c.GetDiscountAmount(), c.GetDescription()})
	}
}

func shippingQuoteHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		customerID := strings.TrimSpace(r.URL.Query().Get("customerId"))
		subtotal, err1 := strconv.ParseFloat(r.URL.Query().Get("subtotal"), 64)
		count, err2 := strconv.Atoi(r.URL.Query().Get("itemCount"))
		if customerID == "" || err1 != nil || err2 != nil {
			writeBad(w, "Please provide customerId, subtotal and itemCount.")
			return
		}
		customer, err := g.getCustomer(customerID)
		if err != nil {
			writeCustomerErr(w, err, customerID)
			return
		}
		q, err := g.getQuote(customer.GetAddress(), int32(count), subtotal)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Success      bool    `json:"success"`
			Address      string  `json:"address"`
			Zone         string  `json:"zone"`
			Method       string  `json:"method"`
			Fee          float64 `json:"fee"`
			EtaDays      int32   `json:"etaDays"`
			FreeShipping bool    `json:"freeShipping"`
		}{true, customer.GetAddress(), q.GetZone(), q.GetMethod(), q.GetFee(), q.GetEtaDays(), q.GetFreeShipping()})
	}
}

func trackHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/shipping/track/")
		sh, err := g.trackShipment(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		type ev struct {
			Time    string `json:"time"`
			Message string `json:"message"`
		}
		events := []ev{}
		for _, e := range sh.GetEvents() {
			events = append(events, ev{e.GetTime(), e.GetMessage()})
		}
		writeJSON(w, http.StatusOK, struct {
			Success    bool   `json:"success"`
			TrackingID string `json:"trackingId"`
			OrderID    string `json:"orderId"`
			Status     string `json:"status"`
			Address    string `json:"address"`
			EtaDays    int32  `json:"etaDays"`
			Events     []ev   `json:"events"`
		}{true, sh.GetTrackingId(), sh.GetOrderId(), sh.GetStatus(), sh.GetAddress(), sh.GetEtaDays(), events})
	}
}

type orderRecordJSON struct {
	OrderID       string  `json:"orderId"`
	CustomerID    string  `json:"customerId"`
	ProductID     string  `json:"productId"`
	ProductName   string  `json:"productName"`
	Quantity      int32   `json:"quantity"`
	Subtotal      float64 `json:"subtotal"`
	DiscountTotal float64 `json:"discountTotal"`
	ShippingFee   float64 `json:"shippingFee"`
	Total         float64 `json:"total"`
	PaymentID     string  `json:"paymentId"`
	TrackingID    string  `json:"trackingId"`
	CouponCode    string  `json:"couponCode"`
	CreatedAt     string  `json:"createdAt"`
}

func toOrderJSON(o *pb.OrderRecordMsg) orderRecordJSON {
	return orderRecordJSON{o.GetOrderId(), o.GetCustomerId(), o.GetProductId(), o.GetProductName(), o.GetQuantity(),
		o.GetSubtotal(), o.GetDiscountTotal(), o.GetShippingFee(), o.GetTotal(), o.GetPaymentId(), o.GetTrackingId(),
		o.GetCouponCode(), o.GetCreatedAt()}
}

func listOrdersHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID := strings.TrimSpace(r.URL.Query().Get("customerId"))
		if customerID == "" {
			writeBad(w, "Please enter your Customer ID.")
			return
		}
		if _, err := g.getCustomer(customerID); err != nil {
			writeCustomerErr(w, err, customerID)
			return
		}
		l, err := callResilient(historyBreaker, func(ctx context.Context) (*pb.ListOrdersResponse, error) {
			return g.history.ListOrders(ctx, &pb.ListOrdersRequest{CustomerId: customerID})
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		orders := []orderRecordJSON{}
		for _, o := range l.GetOrders() {
			orders = append(orders, toOrderJSON(o))
		}
		writeJSON(w, http.StatusOK, struct {
			Success bool              `json:"success"`
			Orders  []orderRecordJSON `json:"orders"`
		}{true, orders})
	}
}

func orderByIDHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/orders/")
		o, err := callResilient(historyBreaker, func(ctx context.Context) (*pb.OrderRecordMsg, error) {
			return g.history.GetOrder(ctx, &pb.GetOrderRequest{OrderId: id})
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Success bool            `json:"success"`
			Order   orderRecordJSON `json:"order"`
		}{true, toOrderJSON(o)})
	}
}

// ---------------------------------------------------------------------
// Service status and test-mode reset
// ---------------------------------------------------------------------

// serviceUp reports whether a gRPC connection reaches a running server.
func serviceUp(conn *grpc.ClientConn) bool {
	conn.Connect()
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return true
		}
		if !conn.WaitForStateChange(ctx, state) {
			return conn.GetState() == connectivity.Ready
		}
	}
}

func servicesHandler(conns map[string]*grpc.ClientConn) http.HandlerFunc {
	order := []string{"product", "inventory", "customer", "payment", "notification", "cart", "review", "shipping", "coupon", "history"}
	ports := map[string]int{"product": 50051, "inventory": 50052, "customer": 50053, "payment": 50054, "notification": 50055,
		"cart": 50056, "review": 50057, "shipping": 50058, "coupon": 50059, "history": 50060}
	type svc struct {
		Name string `json:"name"`
		Port int    `json:"port"`
		Up   bool   `json:"up"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		list := make([]svc, len(order))
		done := make(chan struct{}, len(order))
		for i, name := range order {
			go func(i int, name string) {
				list[i] = svc{name, ports[name], serviceUp(conns[name])}
				done <- struct{}{}
			}(i, name)
		}
		for range order {
			<-done
		}
		discount := "AWS Lambda (" + os.Getenv("LAMBDA_FUNCTION_NAME") + ")"
		if os.Getenv("LAMBDA_FUNCTION_NAME") == "" {
			discount = "local rules (no Lambda configured)"
		}
		writeJSON(w, http.StatusOK, struct {
			Success  bool   `json:"success"`
			Services []svc  `json:"services"`
			Discount string `json:"discount"`
		}{true, list, discount})
	}
}

// testResetHandler puts every service back to its starting data so each
// automated test run starts clean. It is OFF unless the gateway is started
// with ENABLE_TEST_ENDPOINTS=1; otherwise it answers 404 as if it did not
// exist, so it can never be used against a real deployment by accident.
func testResetHandler(g *gatewayClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("ENABLE_TEST_ENDPOINTS") != "1" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req := &pb.ResetRequest{}
		var failed []string
		check := func(name string, err error) {
			if err != nil {
				failed = append(failed, name)
			}
		}
		_, err := g.inventory.Reset(ctx, req)
		check("inventory", err)
		_, err = g.cart.Reset(ctx, req)
		check("cart", err)
		_, err = g.review.Reset(ctx, req)
		check("review", err)
		_, err = g.shipping.Reset(ctx, req)
		check("shipping", err)
		_, err = g.coupon.Reset(ctx, req)
		check("coupon", err)
		_, err = g.history.Reset(ctx, req)
		check("history", err)
		if len(failed) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, apiError{false, "unavailable", "Could not reset: " + strings.Join(failed, ", ")})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
	}
}

func registerPart2Routes(mux *http.ServeMux, g *gatewayClients, lambdaClient *lambda.Client, conns map[string]*grpc.ClientConn) {
	mux.HandleFunc("/api/cart", cartHandler(g))
	mux.HandleFunc("/api/cart/checkout", checkoutHandler(g, lambdaClient))
	mux.HandleFunc("/api/cart/", cartItemHandler(g))
	mux.HandleFunc("/api/reviews", reviewsAddHandler(g))
	mux.HandleFunc("/api/reviews/", reviewsListHandler(g))
	mux.HandleFunc("/api/coupons/validate", couponValidateHandler(g))
	mux.HandleFunc("/api/shipping/quote", shippingQuoteHandler(g))
	mux.HandleFunc("/api/shipping/track/", trackHandler(g))
	mux.HandleFunc("/api/services", servicesHandler(conns))
	mux.HandleFunc("/api/test/reset", testResetHandler(g))
}
