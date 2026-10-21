package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// server implements the generated PaymentServiceServer interface.
type server struct {
	pb.UnimplementedPaymentServiceServer
}

// ProcessPayment simulates charging a customer for an order and returns
// a payment confirmation with a generated payment ID.
//   - Returns codes.InvalidArgument if the amount is not positive, or if
//     order_id / customer_id are missing.
func (s *server) ProcessPayment(ctx context.Context, req *pb.PaymentRequest) (*pb.PaymentResponse, error) {
	if req.GetOrderId() == "" || req.GetCustomerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id and customer_id are required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "payment amount must be greater than zero")
	}

	// In a real system this would call out to a payment gateway.
	// Here we simulate a successful charge and generate a payment ID.
	paymentID := fmt.Sprintf("PAY-%06d", rand.Intn(1000000))

	return &pb.PaymentResponse{PaymentId: paymentID, Status: "SUCCESS"}, nil
}

func main() {
	rand.Seed(time.Now().UnixNano())

	lis, err := net.Listen("tcp", ":50054")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterPaymentServiceServer(grpcServer, &server{})
	reflection.Register(grpcServer)

	log.Println("Payment Service listening on :50054")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
