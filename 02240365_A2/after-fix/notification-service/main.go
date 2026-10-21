package main

import (
	"fmt"
	"log"
	"net"
	"time"

	pb "ecommerce-microservices/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// server implements the generated NotificationServiceServer interface.
type server struct {
	pb.UnimplementedNotificationServiceServer
}

// SubscribeNotifications is a SERVER-STREAMING RPC: the client sends a
// single request and this method sends multiple responses back over the
// same connection, simulating a sequence of order status updates.
func (s *server) SubscribeNotifications(req *pb.SubscribeRequest, stream pb.NotificationService_SubscribeNotificationsServer) error {
	updates := []string{
		fmt.Sprintf("Order %s confirmed for customer %s", req.GetOrderId(), req.GetCustomerId()),
		fmt.Sprintf("Order %s is being processed", req.GetOrderId()),
		fmt.Sprintf("Order %s has shipped", req.GetOrderId()),
	}

	for _, message := range updates {
		notification := &pb.Notification{
			Message:   message,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		// Send() pushes one message down the open stream to the client.
		if err := stream.Send(notification); err != nil {
			return err
		}

		time.Sleep(1 * time.Second) // simulate a delay between status updates
	}

	// Returning nil closes the stream normally (equivalent to EOF on the client).
	return nil
}

func main() {
	lis, err := net.Listen("tcp", ":50055")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterNotificationServiceServer(grpcServer, &server{})
	reflection.Register(grpcServer)

	log.Println("Notification Service listening on :50055")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
