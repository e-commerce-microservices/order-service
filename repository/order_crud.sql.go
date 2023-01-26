// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.15.0
// source: order_crud.sql

package repository

import (
	"context"

	"github.com/google/uuid"
)

const createOrder = `-- name: CreateOrder :exec
INSERT INTO "order" (
    "customer_id", "supplier_id", "product_id", "quantity"
) VALUES (
    $1, $2, $3, $4
)
`

type CreateOrderParams struct {
	CustomerID int64
	SupplierID int64
	ProductID  int64
	Quantity   int32
}

func (q *Queries) CreateOrder(ctx context.Context, arg CreateOrderParams) error {
	_, err := q.db.ExecContext(ctx, createOrder,
		arg.CustomerID,
		arg.SupplierID,
		arg.ProductID,
		arg.Quantity,
	)
	return err
}

const updateOrderStatus = `-- name: UpdateOrderStatus :exec
UPDATE "order"
SET "status" = $1
WHERE "id" = $2
`

type UpdateOrderStatusParams struct {
	Status interface{}
	ID     uuid.UUID
}

func (q *Queries) UpdateOrderStatus(ctx context.Context, arg UpdateOrderStatusParams) error {
	_, err := q.db.ExecContext(ctx, updateOrderStatus, arg.Status, arg.ID)
	return err
}