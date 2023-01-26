package main

import (
	"context"

	"github.com/e-commerce-microservices/order-service/pb"
	"github.com/e-commerce-microservices/order-service/repository"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/metadata"
)

type orderService struct {
	authClient pb.AuthServiceClient
	orderRepo  repository.Queries
	pb.UnimplementedOrderServiceServer
}

var _empty = &empty.Empty{}

func (srv orderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	var err error
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// auth
	_, err = srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		return nil, err
	}

	err = srv.orderRepo.CreateOrder(ctx, repository.CreateOrderParams{
		CustomerID: req.GetCustomerId(),
		SupplierID: req.GetSupplierId(),
		ProductID:  req.GetProductId(),
		Quantity:   req.GetOrderQuantity(),
	})
	if err != nil {
		return nil, err
	}

	return &pb.CreateOrderResponse{
		Message: "Tạo thành công",
	}, nil
}

func (srv orderService) Ping(context.Context, *empty.Empty) (*pb.Pong, error) {
	return &pb.Pong{
		Message: "pong",
	}, nil
}
