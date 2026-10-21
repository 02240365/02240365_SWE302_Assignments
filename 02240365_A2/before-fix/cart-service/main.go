package main

// Part 2 - Cart Service (port 50056).
// Keeps each customer's shopping cart in memory. All business rules
// (quantity limits, merging lines, totals) live in internal/shop/cart.go,
// where they are unit-tested; this file only translates gRPC <-> shop.

import (
	"context"
	"log"
	"net"

	"ecommerce-microservices/internal/rpcerr"
	"ecommerce-microservices/internal/shop"
	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type server struct {
	pb.UnimplementedCartServiceServer
	carts *shop.Carts
}

func toResponse(v shop.CartView) *pb.CartResponse {
	resp := &pb.CartResponse{CustomerId: v.CustomerID, ItemCount: v.ItemCount, Subtotal: v.Subtotal}
	for _, l := range v.Lines {
		resp.Lines = append(resp.Lines, &pb.CartLineMsg{
			ProductId: l.ProductID, ProductName: l.ProductName, UnitPrice: l.UnitPrice,
			Quantity: l.Quantity, LineTotal: l.LineTotal,
		})
	}
	return resp
}

func (s *server) AddItem(ctx context.Context, req *pb.AddCartItemRequest) (*pb.CartResponse, error) {
	v, err := s.carts.Add(req.GetCustomerId(), req.GetProductId(), req.GetProductName(), req.GetUnitPrice(), req.GetQuantity())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return toResponse(v), nil
}

func (s *server) GetCart(ctx context.Context, req *pb.CartCustomerRequest) (*pb.CartResponse, error) {
	v, err := s.carts.Get(req.GetCustomerId())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return toResponse(v), nil
}

func (s *server) RemoveItem(ctx context.Context, req *pb.RemoveCartItemRequest) (*pb.CartResponse, error) {
	v, err := s.carts.Remove(req.GetCustomerId(), req.GetProductId())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return toResponse(v), nil
}

func (s *server) ClearCart(ctx context.Context, req *pb.CartCustomerRequest) (*pb.CartResponse, error) {
	v, err := s.carts.Clear(req.GetCustomerId())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return toResponse(v), nil
}

func (s *server) Reset(ctx context.Context, req *pb.ResetRequest) (*pb.ResetResponse, error) {
	s.carts.Reset()
	return &pb.ResetResponse{Message: "cart service reset"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50056")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterCartServiceServer(grpcServer, &server{carts: shop.NewCarts()})
	reflection.Register(grpcServer)

	log.Println("Cart Service listening on :50056")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
