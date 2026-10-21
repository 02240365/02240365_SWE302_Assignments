package main

// Part 2 - Coupon Service (port 50059).
// Codes: WELCOME10 (10% off, max Nu. 5,000), SAVE500 (Nu. 500 off orders
// of Nu. 3,000+), OLD20 (expired). One use per customer. Rules are in
// internal/shop/coupon.go.

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
	pb.UnimplementedCouponServiceServer
	coupons *shop.Coupons
}

func (s *server) ValidateCoupon(ctx context.Context, req *pb.ValidateCouponRequest) (*pb.CouponResponse, error) {
	r, err := s.coupons.Validate(req.GetCode(), req.GetCustomerId(), req.GetSubtotal())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return &pb.CouponResponse{Code: r.Code, DiscountAmount: r.DiscountAmount, Description: r.Description}, nil
}

func (s *server) RedeemCoupon(ctx context.Context, req *pb.RedeemCouponRequest) (*pb.RedeemCouponResponse, error) {
	if err := s.coupons.Redeem(req.GetCode(), req.GetCustomerId(), req.GetOrderId()); err != nil {
		return nil, rpcerr.To(err)
	}
	return &pb.RedeemCouponResponse{Status: "REDEEMED"}, nil
}

func (s *server) Reset(ctx context.Context, req *pb.ResetRequest) (*pb.ResetResponse, error) {
	s.coupons.Reset()
	return &pb.ResetResponse{Message: "coupon service reset"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50059")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterCouponServiceServer(grpcServer, &server{coupons: shop.NewCoupons()})
	reflection.Register(grpcServer)

	log.Println("Coupon Service listening on :50059")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
