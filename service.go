package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/e-commerce-microservices/order-service/pb"
	"github.com/e-commerce-microservices/order-service/repository"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/itimofeev/go-saga"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc/metadata"
)

type orderService struct {
	authClient    pb.AuthServiceClient
	productClient pb.ProductServiceClient
	cartClient    pb.CartServiceClient
	orderRepo     repository.Queries
	pb.UnimplementedOrderServiceServer
}

var _empty = &empty.Empty{}

var tracer = otel.Tracer("order-service")

func (srv orderService) GetAddressOrder(ctx context.Context, req *pb.GetAddressOrderRequest) (*pb.GetAddressOrderResponse, error) {
	address, err := srv.orderRepo.GetAddressById(ctx, req.GetAddressId())
	if err != nil {
		return nil, errors.New("có lỗi xảy ra, không thể tìm thấy địa chỉ")
	}
	return &pb.GetAddressOrderResponse{
		Name:   address.Name,
		Phone:  address.Phone,
		Detail: address.Detail,
	}, nil
}

func (srv orderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	var err error
	orderSaga := saga.NewSaga("order-saga")
	var sagaStore = saga.New()

	ctx, span := tracer.Start(ctx, "OrderService.Create")
	defer span.End()

	md, _ := metadata.FromIncomingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, md)

	// auth
	claims, err := srv.authClient.GetUserClaims(ctx, _empty)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	log.Println("addr: ", req.Addr)
	if req.Addr == nil || req.Addr.GetName() == "" || req.Addr.GetPhone() == "" || req.Addr.GetDetail() == "" {
		return nil, errors.New("Địa chỉ không hợp lệ")
	}

	log.Println("Create address")
	var address repository.Address
	orderSaga.AddStep(&saga.Step{
		Name: "create address",
		Func: func(ctx context.Context) error {
			address, err = srv.orderRepo.CreateAddress(ctx, repository.CreateAddressParams{
				Name:   req.Addr.GetName(),
				Phone:  req.Addr.GetPhone(),
				Detail: req.Addr.GetDetail(),
			})
			if err != nil {
				return errors.New("Địa chỉ không hợp lệ")
			}
			return nil
		},
		CompensateFunc: func(ctx context.Context) error {
			log.Println("delete address", address)
			err := srv.orderRepo.DeleteAddress(ctx, address.ID)
			if err != nil {
				return errors.New("Địa chỉ không hợp lệ")
			}
			return nil
		},
	})

	fmt.Println("get list order from request", req.GetListOrder())
	if len(req.GetListOrder()) == 0 {
		return nil, errors.New("Vui lòng chọn sản phẩm cần mua")
	}

	// check product inventory + other order (waiting status)
	for i := 0; i < len(req.GetListOrder()); i++ {
		v := req.GetListOrder()[i]
		orderSaga.AddStep(&saga.Step{
			Name: fmt.Sprintf("Check inventory %d", i),
			Func: func(ctx context.Context) error {
				fmt.Println("order: ", v)
				inventory, err := srv.productClient.GetListProductInventory(ctx, &pb.GetInventoryRequest{
					ProductId: v.GetProductId(),
				})
				if err != nil {
					return err
				}
				if inventory.GetCount() < int64(v.GetOrderQuantity()) {
					return errors.New("Sản phẩm trong kho không đủ")
				}
				return nil
			},
			CompensateFunc: func(ctx context.Context) error {
				return nil
			},
		})
		log.Println("create order")
		customerID, _ := strconv.ParseInt(claims.GetId(), 10, 64)

		var order repository.Order
		orderSaga.AddStep(&saga.Step{
			Name: fmt.Sprintf("createOrder: %d", i),
			Func: func(ctx context.Context) error {
				_, span := tracer.Start(ctx, "OrderService.Database.Insert")
				defer span.End()
				order, err = srv.orderRepo.CreateOrder(ctx, repository.CreateOrderParams{
					CustomerID: customerID,
					SupplierID: v.GetSupplierId(),
					ProductID:  v.GetProductId(),
					Quantity:   v.GetOrderQuantity(),
					AddressID:  address.ID,
				})
				if err != nil {
					return err
				}
				return nil
			},
			CompensateFunc: func(ctx context.Context) error {
				return srv.orderRepo.DeleteOrder(ctx, order.ID)
			},
		})

		log.Println("update inventory")
		orderSaga.AddStep(&saga.Step{
			Name: fmt.Sprintf("Update inventory: %d", i),
			Func: func(ctx context.Context) error {
				_, err := srv.productClient.DescInventory(ctx, &pb.DescInventoryRequest{
					ProductId: v.GetProductId(),
					Count:     v.GetOrderQuantity(),
				}) // tp, tpErr := jaegerTraceProvider()
				// if tpErr != nil {
				// 	log.Fatal(tpErr)
				// }
				// otel.SetTracerProvider(tp)
				// otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
				return err
			},
			CompensateFunc: func(ctx context.Context) error {
				_, err := srv.productClient.IncInventory(ctx, &pb.IncInventoryRequest{
					ProductId: v.GetProductId(),
					Count:     v.GetOrderQuantity(),
				})
				return err
			},
		})

		// delete cart
		// orderSaga.AddStep(&saga.Step{
		// 	Name: fmt.Sprintf("Delete cart: %d", i),
		// 	Func: func(ctx context.Context) error {
		// 		_, err := srv.cartClient.DeleteCart(ctx, &pb.DeleteCartRequest{CartId: v.GetCartId()})
		// 		if err != nil {
		// 			return err
		// 		}
		// 		return nil
		// 	},
		// 	CompensateFunc: func(ctx context.Context) error {
		// 		_, err := srv.cartClient.CreateCart(ctx, &pb.CreateCartRequest{
		// 			ProductId: v.GetProductId(),
		// 			Quantity:  v.GetOrderQuantity(),
		// 		})
		// 		if err != nil {
		// 			return err
		// 		}
		// 		return nil
		// 	},
		// })

	}

	coordinator := saga.NewCoordinator(ctx, ctx, orderSaga, sagaStore)
	result := coordinator.Play()
	if result.ExecutionError != nil {
		log.Println("saga error: ", result.ExecutionError)
		return nil, fmt.Errorf("Tạo đơn hàng không thành công, %s", result.ExecutionError.Error())
	}

	return &pb.CreateOrderResponse{
		Message: "Tạo đơn hàng thành công",
	}, nil
}

func (srv orderService) GetSoldProduct(ctx context.Context, req *pb.GetSoldProductRequest) (*pb.GetSoldProductResponse, error) {
	listQuantity, _ := srv.orderRepo.CountOrderHandledByProductId(ctx, req.GetProductId())
	var cnt int64 = 0
	for _, v := range listQuantity {
		cnt += int64(v)
	}
	return &pb.GetSoldProductResponse{
		Count: cnt,
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
	log.Println("get product by supplierID: ", supplierID)

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
	log.Println("resp: ", resp, listID)
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
		addr, _ := srv.orderRepo.GetAddressById(ctx, order.AddressID)
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
			AddressName:   addr.Name,
			AddressPhone:  addr.Phone,
			AddressDetail: addr.Detail,
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
		addr, _ := srv.orderRepo.GetAddressById(ctx, order.AddressID)

		result = append(result, &pb.Order{
			ProductPrice:  product.Price,
			ProductName:   product.Name,
			ProductImage:  product.Thumbnail,
			OrderId:       order.ID,
			ProductId:     order.ProductID,
			OrderQuantity: order.Quantity,
			CustomerId:    order.CustomerID,
			SupplierId:    order.SupplierID,
			AddressName:   addr.Name,
			AddressPhone:  addr.Phone,
			AddressDetail: addr.Detail,
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

		addr, _ := srv.orderRepo.GetAddressById(ctx, order.AddressID)
		result = append(result, &pb.Order{
			ProductPrice:  product.Price,
			ProductName:   product.Name,
			ProductImage:  product.Thumbnail,
			OrderId:       order.ID,
			ProductId:     order.ProductID,
			OrderQuantity: order.Quantity,
			CustomerId:    order.CustomerID,
			SupplierId:    order.SupplierID,
			AddressName:   addr.Name,
			AddressPhone:  addr.Phone,
			AddressDetail: addr.Detail,
		})
	}

	return &pb.GetHandledOrderByCustomerResponse{
		ListOrder: result,
	}, nil
}

func (srv orderService) GetHandledOrderBySupllier(ctx context.Context, _ *empty.Empty) (*pb.GetHandledOrderBySupplierResponse, error) {
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
	listOrder, err := srv.orderRepo.GetHandledOrderBySupplier(ctx, customerID)
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
		return &pb.GetHandledOrderBySupplierResponse{
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

		addr, _ := srv.orderRepo.GetAddressById(ctx, order.AddressID)
		result = append(result, &pb.Order{
			ProductPrice:  product.Price,
			ProductName:   product.Name,
			ProductImage:  product.Thumbnail,
			OrderId:       order.ID,
			ProductId:     order.ProductID,
			OrderQuantity: order.Quantity,
			CustomerId:    order.CustomerID,
			SupplierId:    order.SupplierID,
			AddressName:   addr.Name,
			AddressPhone:  addr.Phone,
			AddressDetail: addr.Detail,
		})
	}

	return &pb.GetHandledOrderBySupplierResponse{
		ListOrder: result,
	}, nil
}
func (srv orderService) Ping(context.Context, *empty.Empty) (*pb.Pong, error) {
	return &pb.Pong{
		Message: "pong",
	}, nil
}

func (srv orderService) GetCancelOrderByCustomer(ctx context.Context, _ *empty.Empty) (*pb.GetHandledOrderByCustomerResponse, error) {
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
	listOrder, err := srv.orderRepo.GetCancelOrderByCustomer(ctx, customerID)
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

		addr, _ := srv.orderRepo.GetAddressById(ctx, order.AddressID)
		result = append(result, &pb.Order{
			ProductPrice:  product.Price,
			ProductName:   product.Name,
			ProductImage:  product.Thumbnail,
			OrderId:       order.ID,
			ProductId:     order.ProductID,
			OrderQuantity: order.Quantity,
			CustomerId:    order.CustomerID,
			SupplierId:    order.SupplierID,
			AddressName:   addr.Name,
			AddressPhone:  addr.Phone,
			AddressDetail: addr.Detail,
		})
	}

	return &pb.GetHandledOrderByCustomerResponse{
		ListOrder: result,
	}, nil
}

func (srv orderService) GetCancelOrderBySupplier(ctx context.Context, _ *empty.Empty) (*pb.GetHandledOrderBySupplierResponse, error) {
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

	supplierID, _ := strconv.ParseInt(claims.GetId(), 10, 64)
	listOrder, err := srv.orderRepo.GetCancelOrderBySupplier(ctx, supplierID)
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
		return &pb.GetHandledOrderBySupplierResponse{
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
		addr, _ := srv.orderRepo.GetAddressById(ctx, order.AddressID)

		result = append(result, &pb.Order{
			ProductPrice:  product.Price,
			ProductName:   product.Name,
			ProductImage:  product.Thumbnail,
			OrderId:       order.ID,
			ProductId:     order.ProductID,
			OrderQuantity: order.Quantity,
			CustomerId:    order.CustomerID,
			SupplierId:    order.SupplierID,
			AddressName:   addr.Name,
			AddressPhone:  addr.Phone,
			AddressDetail: addr.Detail,
		})
	}

	return &pb.GetHandledOrderBySupplierResponse{
		ListOrder: result,
	}, nil
}
