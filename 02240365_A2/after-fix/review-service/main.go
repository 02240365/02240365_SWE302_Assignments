package main

// Part 2 - Review Service (port 50057).
// Stores product reviews in memory. Rules (rating 1-5, comment length,
// one review per customer per product, average rating) are in
// internal/shop/review.go and unit-tested there.

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
	pb.UnimplementedReviewServiceServer
	reviews *shop.Reviews
}

func toMsg(r shop.Review) *pb.ReviewMsg {
	return &pb.ReviewMsg{
		ReviewId: r.ReviewID, ProductId: r.ProductID, CustomerId: r.CustomerID, CustomerName: r.CustomerName,
		Rating: r.Rating, Comment: r.Comment, CreatedAt: r.CreatedAt,
	}
}

func (s *server) AddReview(ctx context.Context, req *pb.AddReviewRequest) (*pb.ReviewMsg, error) {
	r, err := s.reviews.Add(req.GetProductId(), req.GetCustomerId(), req.GetCustomerName(), req.GetRating(), req.GetComment())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return toMsg(r), nil
}

func (s *server) ListReviews(ctx context.Context, req *pb.ListReviewsRequest) (*pb.ListReviewsResponse, error) {
	l, err := s.reviews.List(req.GetProductId())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	resp := &pb.ListReviewsResponse{ProductId: l.ProductID, AverageRating: l.AverageRating, ReviewCount: l.ReviewCount}
	for _, r := range l.Reviews {
		resp.Reviews = append(resp.Reviews, toMsg(r))
	}
	return resp, nil
}

func (s *server) Reset(ctx context.Context, req *pb.ResetRequest) (*pb.ResetResponse, error) {
	s.reviews.Reset()
	return &pb.ResetResponse{Message: "review service reset"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50057")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterReviewServiceServer(grpcServer, &server{reviews: shop.NewReviews()})
	reflection.Register(grpcServer)

	log.Println("Review Service listening on :50057")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
