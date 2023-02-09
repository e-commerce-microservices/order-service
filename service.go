package main

import (
	"context"
	"errors"
	"log"
	"strconv"

	"github.com/e-commerce-microservices/order-service/pb"
	"github.com/e-commerce-microservices/order-service/repository"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/metadata"
)

type orderService struct {
	authClient    pb.AuthServiceClient
	productClient pb.ProductServiceClient
	orderRepo     repository.Queries
	pb.UnimplementedOrderServiceServer
}

var _empty = &empty.Empty{}

func (srv orderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	var err error
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// auth
	claims, err := srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	log.Println("Get inventory")
	// check product inventory + other order (waiting status)
	// get inventory
	inventory, err := srv.productClient.GetListProductInventory(ctx, &pb.GetInventoryRequest{
		ProductId: req.GetProductId(),
	})
	if err != nil {
		return nil, err
	}

	if inventory.GetCount() < int64(req.GetOrderQuantity()) {
		return &pb.CreateOrderResponse{
			Message: "Sản phẩm trong kho không đủ",
		}, errors.New("invalid request")
	}

	log.Println("create order")
	customerID, _ := strconv.ParseInt(claims.GetId(), 10, 64)
	err = srv.orderRepo.CreateOrder(ctx, repository.CreateOrderParams{
		CustomerID: customerID,
		SupplierID: req.GetSupplierId(),
		ProductID:  req.GetProductId(),
		Quantity:   req.GetOrderQuantity(),
	})
	if err != nil {
		return nil, err
	}

	log.Println("desc order")
	// update inventory
	_, err = srv.productClient.DescInventory(ctx, &pb.DescInventoryRequest{
		ProductId: req.GetProductId(),
		Count:     req.GetOrderQuantity(),
	})
	if err != nil {
		return nil, err
	}

	return &pb.CreateOrderResponse{
		Message: "Tạo đơn hàng thành công",
	}, nil
}

func (srv orderService) DeleteOrder(ctx context.Context, req *pb.DeleteOrderRequest) (*pb.DeleteOrderResponse, error) {
	// authorization for supplier_id
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// auth
	claims, err := srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		return nil, err
	}

	customerID, err := strconv.ParseInt(claims.GetId(), 10, 64)
	// get supplier_id from order_id
	order, err := srv.orderRepo.GetOrderByID(ctx, req.GetOrderId())
	if err != nil {
		log.Println(err)
		return nil, errors.New("Hủy đơn hàng không thành công")
	}

	if !(customerID == order.SupplierID || customerID == order.CustomerID) {
		return nil, errors.New("Hủy đơn hàng không thành công, unauthorization")
	}

	// update product count
	_, err = srv.productClient.IncInventory(ctx, &pb.IncInventoryRequest{
		ProductId: order.ProductID,
		Count:     order.Quantity,
	})
	if err != nil {
		log.Println(err)
		return nil, errors.New("Hủy đơn hàng không thành công")
	}

	// delete order
	err = srv.orderRepo.DeleteOrder(ctx, req.GetOrderId())
	if err != nil {
		log.Println(err)
		return nil, errors.New("Hủy đơn hàng không thành công")
	}

	return &pb.DeleteOrderResponse{
		Message: "Hủy đơn hàng thành công",
	}, nil
}
func (srv orderService) CheckOrderIsHandled(ctx context.Context, req *pb.CheckOrderIsHandledRequest) (*pb.CheckOrderIsHandledResponse, error) {
	// auth
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// auth
	claims, err := srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		return nil, err
	}

	customerID, err := strconv.ParseInt(claims.GetId(), 10, 64)
	n, err := srv.orderRepo.CheckOrderIsHandled(ctx, repository.CheckOrderIsHandledParams{
		ProductID:  req.GetProductId(),
		CustomerID: customerID,
	})
	log.Println("number order: ", n, req.GetProductId(), customerID)
	if err != nil {
		log.Println(err)
		return &pb.CheckOrderIsHandledResponse{
			IsBought: false,
		}, nil
	}
	if n == 0 {
		return &pb.CheckOrderIsHandledResponse{
			IsBought: false,
		}, nil
	}
	return &pb.CheckOrderIsHandledResponse{
		IsBought: true,
	}, nil
}

func (srv orderService) UpdateOrder(ctx context.Context, req *pb.UpdateOrderStatusRequest) (*pb.UpdateOrderStatusResponse, error) {
	var err error

	err = srv.orderRepo.UpdateOrderStatus(ctx, repository.UpdateOrderStatusParams{
		Status: repository.NullOrderStatusEnum{
			OrderStatusEnum: repository.OrderStatusEnum(req.GetStatus().String()),
			Valid:           false,
		},
		ID: req.GetOrderId(),
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

	// authorization for supplier_id
	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// auth
	claims, err := srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		return nil, err
	}

	customerID, err := strconv.ParseInt(claims.GetId(), 10, 64)
	// get supplier_id from order_id
	order, err := srv.orderRepo.GetOrderByID(ctx, req.GetOrderId())
	if err != nil {
		log.Println(err)
		return nil, errors.New("Xử lý đơn hàng không thành công")
	}

	if customerID != order.SupplierID {
		return nil, errors.New("Xử lý đơn hàng không thành công, unauthorization")
	}

	err = srv.orderRepo.HandleOrder(ctx, req.GetOrderId())
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
	if claims.GetUserRole() == pb.UserRole_customer {
		return nil, errors.New("Unauthorization")
	}
	supplierID, _ := strconv.ParseInt(claims.GetId(), 10, 64)

	listOrder, err := srv.orderRepo.GetWaitingOrderBySupplier(ctx, supplierID)
	if err != nil {
		return nil, err
	}

	listID := make([]int64, 0, len(listOrder))
	for _, order := range listOrder {
		listID = append(listID, order.ProductID)
	}
	resp, err := srv.productClient.GetListProductByIDs(ctx, &pb.GetListProductByIDsRequest{
		ListId: listID,
	})
	if len(resp.ListProduct) == 0 || err != nil {
		return &pb.GetWaitingOrderBySupplierResponse{
			ListOrder: []*pb.Order{},
		}, nil
	}
	m := make(map[int64]*pb.Product)
	for _, v := range resp.ListProduct {
		m[v.ProductId] = v
	}

	result := make([]*pb.Order, 0, len(listOrder))
	for _, order := range listOrder {
		product, ok := m[order.ProductID]
		if !ok {
			continue
		}
		// get product info
		result = append(result, &pb.Order{
			ProductPrice:  product.Price,
			ProductName:   product.Name,
			ProductImage:  product.Thumbnail,
			OrderId:       order.ID,
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
	// auth
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errors.New("invalid request")
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	log.Println("get user claims")
	claims, err := srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		return nil, err
	}

	customerID, _ := strconv.ParseInt(claims.GetId(), 10, 64)

	log.Println("get list order")
	listOrder, err := srv.orderRepo.GetWaitingOrderByCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}

	listID := make([]int64, 0, len(listOrder))
	for _, order := range listOrder {
		listID = append(listID, order.ProductID)
	}
	log.Println("get list product")
	resp, err := srv.productClient.GetListProductByIDs(ctx, &pb.GetListProductByIDsRequest{
		ListId: listID,
	})
	if len(resp.ListProduct) == 0 || err != nil {
		return &pb.GetWaitingOrderByCustomerResponse{
			ListOrder: []*pb.Order{},
		}, nil
	}

	m := make(map[int64]*pb.Product)
	for _, v := range resp.ListProduct {
		m[v.ProductId] = v
	}
	if err != nil {
		return nil, err
	}

	log.Println("create response")
	result := make([]*pb.Order, 0, len(listOrder))
	for _, order := range listOrder {
		product, ok := m[order.ProductID]
		if !ok {
			continue
		}

		result = append(result, &pb.Order{
			ProductPrice:  product.Price,
			ProductName:   product.Name,
			ProductImage:  product.Thumbnail,
			OrderId:       order.ID,
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
func (srv orderService) GetHandledOrderByCustomer(ctx context.Context, req *pb.GetHandledOrderByCustomerRequest) (*pb.GetHandledOrderByCustomerResponse, error) {
	// auth
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, errors.New("invalid request")
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	log.Println("get user claims")
	claims, err := srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		return nil, err
	}

	customerID, _ := strconv.ParseInt(claims.GetId(), 10, 64)

	log.Println("get list order")
	listOrder, err := srv.orderRepo.GetHandledOrderByCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}

	listID := make([]int64, 0, len(listOrder))
	for _, order := range listOrder {
		listID = append(listID, order.ProductID)
	}
	log.Println("get list product")
	resp, err := srv.productClient.GetListProductByIDs(ctx, &pb.GetListProductByIDsRequest{
		ListId: listID,
	})

	if len(resp.ListProduct) == 0 || err != nil {
		return &pb.GetHandledOrderByCustomerResponse{
			ListOrder: []*pb.Order{},
		}, nil
	}

	m := make(map[int64]*pb.Product)
	for _, v := range resp.ListProduct {
		m[v.ProductId] = v
	}
	if err != nil {
		return nil, err
	}

	log.Println("create response")
	result := make([]*pb.Order, 0, len(listOrder))
	for _, order := range listOrder {
		product, ok := m[order.ProductID]
		if !ok {
			continue
		}

		result = append(result, &pb.Order{
			ProductPrice:  product.Price,
			ProductName:   product.Name,
			ProductImage:  product.Thumbnail,
			OrderId:       order.ID,
			ProductId:     order.ProductID,
			OrderQuantity: order.Quantity,
			CustomerId:    order.CustomerID,
			SupplierId:    order.SupplierID,
		})
	}

	return &pb.GetHandledOrderByCustomerResponse{
		ListOrder: result,
	}, nil
}

func (srv orderService) Ping(context.Context, *empty.Empty) (*pb.Pong, error) {
	return &pb.Pong{
		Message: "pong",
	}, nil
}
