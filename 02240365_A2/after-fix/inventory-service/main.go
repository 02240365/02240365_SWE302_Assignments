package main

import (
	"context"
	"log"
	"net"
	"sync"

	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// server implements the generated InventoryServiceServer interface.
// A mutex guards the in-memory stock map since gRPC handles requests
// concurrently on separate goroutines.
type server struct {
	pb.UnimplementedInventoryServiceServer
	mu    sync.Mutex
	stock map[string]int32
}

// CheckStock returns how many units of a product are currently available.
func (s *server) CheckStock(ctx context.Context, req *pb.StockRequest) (*pb.StockResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	qty, ok := s.stock[req.GetProductId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no inventory record for product %s", req.GetProductId())
	}

	return &pb.StockResponse{ProductId: req.GetProductId(), Quantity: qty}, nil
}

// DecreaseStock reserves units of a product for an order.
//   - Returns codes.NotFound if the product has no inventory record.
//   - Returns codes.InvalidArgument if the requested quantity is not positive.
//   - Returns codes.FailedPrecondition if there is not enough stock left.
func (s *server) DecreaseStock(ctx context.Context, req *pb.DecreaseStockRequest) (*pb.StockResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	qty, ok := s.stock[req.GetProductId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no inventory record for product %s", req.GetProductId())
	}
	if req.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be greater than zero")
	}
	if qty < req.GetQuantity() {
		return nil, status.Errorf(codes.FailedPrecondition,
			"insufficient stock for product %s: requested %d, available %d",
			req.GetProductId(), req.GetQuantity(), qty)
	}

	s.stock[req.GetProductId()] = qty - req.GetQuantity()
	return &pb.StockResponse{ProductId: req.GetProductId(), Quantity: s.stock[req.GetProductId()]}, nil
}

// RestoreStock gives units back to a product. The gateway calls it when an
// order fails AFTER stock was reserved (for example the payment was
// declined), so a failed order never makes stock disappear.
//   - Returns codes.NotFound if the product has no inventory record.
//   - Returns codes.InvalidArgument if the quantity is not positive.
func (s *server) RestoreStock(ctx context.Context, req *pb.DecreaseStockRequest) (*pb.StockResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	qty, ok := s.stock[req.GetProductId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no inventory record for product %s", req.GetProductId())
	}
	if req.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be greater than zero")
	}
	s.stock[req.GetProductId()] = qty + req.GetQuantity()
	return &pb.StockResponse{ProductId: req.GetProductId(), Quantity: s.stock[req.GetProductId()]}, nil
}

// seedStock returns the starting stock levels (also used by Reset).
func seedStock() map[string]int32 {
	return map[string]int32{"P001": 10, "P002": 50, "P003": 100, "P004": 20}
}

// Reset puts the seed stock back. Test mode only: the gateway exposes it
// through POST /api/test/reset, which is off unless ENABLE_TEST_ENDPOINTS=1.
func (s *server) Reset(ctx context.Context, req *pb.ResetRequest) (*pb.ResetResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stock = seedStock()
	return &pb.ResetResponse{Message: "inventory service reset"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Seed stock levels held in memory for the demo.
	// NOTE: P001 starts at 10 (Lab 2 screenshot: ordering 2 left 8).
	// The other starting values are reconstructed - adjust to taste.
	srv := &server{stock: seedStock()}

	grpcServer := grpc.NewServer()
	pb.RegisterInventoryServiceServer(grpcServer, srv)
	reflection.Register(grpcServer)

	log.Println("Inventory Service listening on :50052")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
