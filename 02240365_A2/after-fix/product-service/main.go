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

// ProductServiceServer implements the ProductService
// defined in product.proto.
type ProductServiceServer struct {
	pb.UnimplementedProductServiceServer

	products map[string]*pb.Product
}

// GetProduct receives a product ID and returns the
// corresponding product information.
func (s *ProductServiceServer) GetProduct(
	ctx context.Context,
	req *pb.GetProductRequest,
) (*pb.Product, error) {

	// Check whether the product ID was provided.
	if req.GetProductId() == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"product ID is required",
		)
	}

	// Search for the product.
	product, exists := s.products[req.GetProductId()]

	// If the product does not exist, return NOT_FOUND.
	if !exists {
		return nil, status.Errorf(
			codes.NotFound,
			"product with ID %s not found",
			req.GetProductId(),
		)
	}

	return product, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Sample products held in memory for the demo.
	server := &ProductServiceServer{
		products: map[string]*pb.Product{
			"P001": {ProductId: "P001", Name: "Laptop", Price: 75000},
			"P002": {ProductId: "P002", Name: "Mechanical Keyboard", Price: 4500},
			"P003": {ProductId: "P003", Name: "Wireless Mouse", Price: 1800},
			"P004": {ProductId: "P004", Name: "Monitor", Price: 25000},
		},
	}

	grpcServer := grpc.NewServer()
	pb.RegisterProductServiceServer(grpcServer, server)
	reflection.Register(grpcServer) // lets grpcurl list/describe the service

	log.Println("Product Service listening on :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
