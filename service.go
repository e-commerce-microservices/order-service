package main

import (
	"context"
	"errors"
	"strconv"

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
	// check product inventory + other order (waiting status)

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
func (srv orderService) UpdateOrder(ctx context.Context, req *pb.UpdateOrderStatusRequest) (*pb.UpdateOrderStatusResponse, error) {
	var err error

	err = srv.orderRepo.UpdateOrderStatus(ctx, repository.UpdateOrderStatusParams{
		Status: req.GetStatus(),
		ID:     req.GetOrderId(),
	})
	if err != nil {
		return nil, err
	}

	return &pb.UpdateOrderStatusResponse{
		Message: "Cập nhật đơn hàng thành công",
	}, nil
}

func (srv orderService) HandleOrder(ctx context.Context, req *pb.HandleOrderRequest) (*pb.HandleOrderResponse, error) {
	var err error

	// update product quantity

	err = srv.orderRepo.UpdateOrderStatus(ctx, repository.UpdateOrderStatusParams{
		Status: pb.OrderStatus_handled,
		ID:     req.GetOrderId(),
	})
	if err != nil {
		return nil, err
	}

	return &pb.HandleOrderResponse{
		Message: "Xử lý đơn hàng thành công",
	}, nil
}
func (srv orderService) GetWaitingOrderBySupplier(ctx context.Context, req *pb.GetWaitingOrderBySupplierRequest) (*pb.GetWaitingOrderBySupplierResponse, error) {
	var err error
	// auth
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	claims, err := srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		return nil, err
	}

	if claims.GetId() != strconv.FormatInt(req.GetSupplierId(), 10) {
		return nil, errors.New("Unauthorization")
	}

	listOrder, err := srv.orderRepo.GetWaitingOrderBySupplier(ctx, req.GetSupplierId())
	if err != nil {
		return nil, err
	}

	result := make([]*pb.Order, 0, len(listOrder))
	for _, order := range listOrder {
		result = append(result, &pb.Order{
			ProductId:     order.ProductID,
			OrderQuantity: order.Quantity,
			CustomerId:    order.CustomerID,
			SupplierId:    order.SupplierID,
		})
	}

	return &pb.GetWaitingOrderBySupplierResponse{
		ListOrder: result,
	}, nil
}
func (srv orderService) GetWaitingOrderByCustomer(ctx context.Context, req *pb.GetWaitingOrderByCustomerRequest) (*pb.GetWaitingOrderByCustomerResponse, error) {
	var err error
	// auth
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	claims, err := srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		return nil, err
	}

	if claims.GetId() != strconv.FormatInt(req.GetCustomerId(), 10) {
		return nil, errors.New("Unauthorization")
	}

	listOrder, err := srv.orderRepo.GetWaitingOrderByCustomer(ctx, req.GetCustomerId())
	if err != nil {
		return nil, err
	}

	result := make([]*pb.Order, 0, len(listOrder))
	for _, order := range listOrder {
		result = append(result, &pb.Order{
			ProductId:     order.ProductID,
			OrderQuantity: order.Quantity,
			CustomerId:    order.CustomerID,
			SupplierId:    order.SupplierID,
		})
	}

	return &pb.GetWaitingOrderByCustomerResponse{
		ListOrder: result,
	}, nil
}

func (srv orderService) Ping(context.Context, *empty.Empty) (*pb.Pong, error) {
	return &pb.Pong{
		Message: "pong",
	}, nil
}
