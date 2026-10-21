package main

import (
	"context"
	"log"
	"net"

	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// server implements the generated CustomerServiceServer interface.
type server struct {
	pb.UnimplementedCustomerServiceServer
	customers map[string]*pb.Customer // in-memory "database" of customers
}

// GetCustomer looks up a customer by ID.
//   - Returns codes.InvalidArgument if no customer_id was supplied.
//   - Returns codes.NotFound if the customer does not exist.
func (s *server) GetCustomer(ctx context.Context, req *pb.GetCustomerRequest) (*pb.Customer, error) {
	if req.GetCustomerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "customer_id must not be empty")
	}

	customer, ok := s.customers[req.GetCustomerId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "customer %s not found", req.GetCustomerId())
	}

	return customer, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Seed a few sample customers held in memory for the demo.
	customers := map[string]*pb.Customer{
		"C001": {CustomerId: "C001", Name: "Sonam Zangmo", Email: "sonam.zangmo@example.com", Address: "Thimphu, Bhutan"},
		"C002": {CustomerId: "C002", Name: "Sonam Choki", Email: "sonam.choki@example.com", Address: "Phuentsholing, Bhutan"},
		"C003": {CustomerId: "C003", Name: "Wangchu Gyeltshen", Email: "wangchu.gyeltshen@example.com", Address: "Paro, Bhutan"},
	}

	grpcServer := grpc.NewServer()
	pb.RegisterCustomerServiceServer(grpcServer, &server{customers: customers})
	reflection.Register(grpcServer)

	log.Println("Customer Service listening on :50053")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
