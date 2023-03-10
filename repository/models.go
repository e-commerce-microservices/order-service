// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.15.0

package repository

import (
	"database/sql/driver"
	"fmt"
	"time"
)

type OrderStatusEnum string

const (
	OrderStatusEnumWaiting OrderStatusEnum = "waiting"
	OrderStatusEnumHandled OrderStatusEnum = "handled"
)

func (e *OrderStatusEnum) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = OrderStatusEnum(s)
	case string:
		*e = OrderStatusEnum(s)
	default:
		return fmt.Errorf("unsupported scan type for OrderStatusEnum: %T", src)
	}
	return nil
}

type NullOrderStatusEnum struct {
	OrderStatusEnum OrderStatusEnum
	Valid           bool // Valid is true if String is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullOrderStatusEnum) Scan(value interface{}) error {
	if value == nil {
		ns.OrderStatusEnum, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.OrderStatusEnum.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullOrderStatusEnum) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return ns.OrderStatusEnum, nil
}

type Order struct {
	ID         int64
	CustomerID int64
	SupplierID int64
	ProductID  int64
	Quantity   int32
	Status     NullOrderStatusEnum
	CreatedAt  time.Time
}
