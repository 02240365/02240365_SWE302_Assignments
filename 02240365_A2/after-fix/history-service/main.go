package main

// Part 2 - Order History Service (port 50060).
// Remembers every completed order so a customer can look at "My orders".
// Rules (unique order IDs, newest first) are in internal/shop/history.go.

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
	pb.UnimplementedOrderHistoryServiceServer
	history *shop.History
}

func toMsg(o shop.Order) *pb.OrderRecordMsg {
	return &pb.OrderRecordMsg{
		OrderId: o.OrderID, CustomerId: o.CustomerID, ProductId: o.ProductID, ProductName: o.ProductName,
		Quantity: o.Quantity, Subtotal: o.Subtotal, DiscountTotal: o.DiscountTotal, ShippingFee: o.ShippingFee,
		Total: o.Total, PaymentId: o.PaymentID, TrackingId: o.TrackingID, CouponCode: o.CouponCode, CreatedAt: o.CreatedAt,
	}
}

func (s *server) RecordOrder(ctx context.Context, req *pb.OrderRecordMsg) (*pb.OrderRecordMsg, error) {
	o, err := s.history.Record(shop.Order{
		OrderID: req.GetOrderId(), CustomerID: req.GetCustomerId(), ProductID: req.GetProductId(),
		ProductName: req.GetProductName(), Quantity: req.GetQuantity(), Subtotal: req.GetSubtotal(),
		DiscountTotal: req.GetDiscountTotal(), ShippingFee: req.GetShippingFee(), Total: req.GetTotal(),
		PaymentID: req.GetPaymentId(), TrackingID: req.GetTrackingId(), CouponCode: req.GetCouponCode(),
	})
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return toMsg(o), nil
}

func (s *server) ListOrders(ctx context.Context, req *pb.ListOrdersRequest) (*pb.ListOrdersResponse, error) {
	orders, err := s.history.List(req.GetCustomerId())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	resp := &pb.ListOrdersResponse{}
	for _, o := range orders {
		resp.Orders = append(resp.Orders, toMsg(o))
	}
	return resp, nil
}

func (s *server) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.OrderRecordMsg, error) {
	o, err := s.history.Get(req.GetOrderId())
	if err != nil {
		return nil, rpcerr.To(err)
	}
	return toMsg(o), nil
}

func (s *server) Reset(ctx context.Context, req *pb.ResetRequest) (*pb.ResetResponse, error) {
	s.history.Reset()
	return &pb.ResetResponse{Message: "order history service reset"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50060")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterOrderHistoryServiceServer(grpcServer, &server{history: shop.NewHistory()})
	reflection.Register(grpcServer)

	log.Println("Order History Service listening on :50060")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
