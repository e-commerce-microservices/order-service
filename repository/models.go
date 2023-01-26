// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.15.0

package repository

import (
	"time"

	"github.com/google/uuid"
)

type Order struct {
	ID         uuid.UUID
	CustomerID int64
	SupplierID int64
	ProductID  int64
	Quantity   int32
	Status     interface{}
	CreatedAt  time.Time
}