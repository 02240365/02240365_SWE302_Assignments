# WEB303 E-commerce Microservices (Labs 2-5 + Part 2)

Go + gRPC microservices with an HTTP gateway/website and an AWS Lambda
discount function.

| Service | Port | Notes |
|---|---|---|
| product-service | 50051 | GetProduct (P001-P004) |
| inventory-service | 50052 | CheckStock, DecreaseStock |
| customer-service | 50053 | GetCustomer (C001-C003) |
| payment-service | 50054 | ProcessPayment |
| notification-service | 50055 | server-streaming SubscribeNotifications |
| cart-service | 50056 | Part 2: AddItem, GetCart, RemoveItem, ClearCart |
| review-service | 50057 | Part 2: AddReview, ListReviews (rating 1-5, one review per customer per product) |
| shipping-service | 50058 | Part 2: GetQuote (zones, free from Nu. 5,000), CreateShipment, TrackShipment |
| coupon-service | 50059 | Part 2: ValidateCoupon, RedeemCoupon (WELCOME10, SAVE500, OLD20 expired) |
| history-service | 50060 | Part 2: RecordOrder, ListOrders, GetOrder |
| order-service | - | CLI client/orchestrator (`place`, resilience demos) + unit/integration tests |
| ui-service | 8080 | HTTP API gateway + storefront; per-dependency timeout, retry, circuit breaker; calls Lambda |
| lambda/ | AWS | `calculateOrderDiscount` (5% qty>=2, 10% >=3, 15% >=5) |

## One-time setup
```bash
brew install protobuf          # protoc
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
./setup.sh                     # generates proto/*.pb.go, go mod tidy, go build ./...
```

## Run
```bash
# Lab 5: AWS credentials come from the environment - never put them in code.
export AWS_ACCESS_KEY_ID=...        # IAM user ecommerce-lambda-caller
export AWS_SECRET_ACCESS_KEY=...    # from your downloaded CSV
export AWS_REGION=ap-southeast-2
export LAMBDA_FUNCTION_NAME=calculateOrderDiscount

go run ./product-service       # each in its own terminal
go run ./inventory-service
go run ./customer-service
go run ./payment-service
go run ./notification-service
go run ./ui-service            # then open http://localhost:8080
```
CLI client: `go run ./order-service place C001 P001 2`

## Tests
```bash
go test ./...
```

## Part 2: ten services, Cypress tests

The five services marked "Part 2" were added to reach ten gRPC services. Their business
rules live in `internal/shop/` (standard library only) and are unit-tested there.

```bash
./setup.sh                 # regenerates proto/*.pb.go (now includes the new services)
./run-all.sh test          # builds and starts all 10 services + gateway, test mode on
npm install                # installs Cypress
npm run cy:open            # Test Runner
npm run cy:run             # headless
./stop-all.sh
go test ./internal/shop/   # unit tests of the new services' rules
```

Without `LAMBDA_FUNCTION_NAME` the gateway applies the same discount tiers locally, so no AWS
account is needed for the tests. To use the real Lambda, export the AWS variables (see Run above)
before `./run-all.sh test`.

`POST /api/test/reset` puts all data back to its start so each Cypress test is independent. It
answers 404 unless the gateway is started with `ENABLE_TEST_ENDPOINTS=1` (`./run-all.sh test`).

## The website (pages)

| Page | Address | Feature |
|---|---|---|
| Home | `/` | landing page, product search, featured products, offers |
| Shop | `/shop.html` | catalogue, lookup by ID, place order, add to cart |
| Cart | `/cart.html` | cart, delivery estimate, check-out |
| Reviews | `/reviews.html` | read and write reviews |
| Orders | `/orders.html` | order history and parcel tracking |
| Status | `/status.html` | the ten services and the discount mode (Lambda or local) |

The Customer ID and Coupon boxes in the header are remembered between pages.
Cypress specs: catalog, order, cart, coupon, reviews, shipping, history, navigation, system (106 tests).
