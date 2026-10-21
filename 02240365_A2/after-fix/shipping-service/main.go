package main

// Part 2 - Shipping Service (port 50058).
// Quotes delivery fees by zone and books/tracks shipments. Rules (zones,
// free shipping from Nu. 5,000, tracking IDs) are in internal/shop/shipping.go.

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
	pb.UnimplementedShippingServiceServer
	shipping *shop.Shipping
}

func toMsg(s shop.Shipment) *pb.ShipmentMsg {
	m := &pb.ShipmentMsg{
		TrackingId: s.TrackingID, OrderId: s.OrderID, CustomerId: s.CustomerID, Address: s.Address,
		Status: s.Status, EtaDays: s.EtaDays,
	}
	for _, e := range s.Events {
		m.Events = append(m.Events, &pb.TrackEventMsg{Time: e.Time, Message: e.Message})
	}
	return m
}

func (s *server) GetQuote(ctx context.Context, req *pb.QuoteRequest) (*pb.QuoteResponse, error) {
	q, err := s.shipping.GetQuote(req.GetAddress(), req.GetItemCount(), req.GetSubtotal())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return &pb.QuoteResponse{Zone: q.Zone, Method: q.Method, Fee: q.Fee, EtaDays: q.EtaDays, FreeShipping: q.FreeShipping}, nil
}

func (s *server) CreateShipment(ctx context.Context, req *pb.CreateShipmentRequest) (*pb.ShipmentMsg, error) {
	sh, err := s.shipping.Create(req.GetOrderId(), req.GetCustomerId(), req.GetAddress())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return toMsg(sh), nil
}

func (s *server) TrackShipment(ctx context.Context, req *pb.TrackShipmentRequest) (*pb.ShipmentMsg, error) {
	sh, err := s.shipping.Track(req.GetTrackingId())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return toMsg(sh), nil
}

func (s *server) Reset(ctx context.Context, req *pb.ResetRequest) (*pb.ResetResponse, error) {
	s.shipping.Reset()
	return &pb.ResetResponse{Message: "shipping service reset"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50058")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterShippingServiceServer(grpcServer, &server{shipping: shop.NewShipping()})
	reflection.Register(grpcServer)

	log.Println("Shipping Service listening on :50058")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
