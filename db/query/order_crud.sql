-- name: CreateOrder :one
INSERT INTO "order" (
    "customer_id", "supplier_id", "product_id", "quantity", "address_id"
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: UpdateOrderStatus :exec
UPDATE "order"
SET "status" = $1
WHERE "id" = $2;


-- name: GetWaitingOrderBySupplier :many
SELECT * FROM "order"
WHERE "supplier_id" = $1 AND "status" = 'waiting';

-- name: GetWaitingOrderByCustomer :many
SELECT * FROM "order"
WHERE "customer_id" = $1 AND "status" = 'waiting';

-- name: CountOrderByProductId :one
SELECT COUNT(*) from "order"
WHERE "product_id" = $1;

-- name: CountOrderHandledByProductId :many
SELECT "quantity" from "order"
WHERE "product_id" = $1 AND "status" = 'handled';

-- name: GetHandledOrderByCustomer :many
SELECT * FROM "order"
WHERE "customer_id" = $1 AND "status" = 'handled';

-- name: GetCancelOrderByCustomer :many
SELECT * FROM "order"
WHERE "customer_id" = $1 AND "status" = 'cancel';

-- name: GetCancelOrderBySupplier :many
SELECT * FROM "order"
WHERE "supplier_id" = $1 AND "status" = 'cancel';

-- name: GetHandledOrderBySupplier :many
SELECT * FROM "order"
WHERE "supplier_id" = $1 AND "status" = 'handled';

-- name: GetAddressById :one
SELECT * FROM "address"
WHERE "id" = $1
LIMIT 1;

-- name: DeleteOrder :exec
UPDATE "order"
SET "status" = 'cancel'
WHERE "id" = $1;

-- name: GetOrderByID :one
SELECT * FROM "order"
WHERE "id" = $1 LIMIT 1;

-- name: HandleOrder :exec
UPDATE "order"
SET "status" = 'handled'
WHERE "id" = $1;

-- name: CancelOrder :exec
UPDATE "order"
SET "status" = 'cancel'
WHERE "id" = $1;

-- name: CheckOrderIsHandled :one
SELECT COUNT(*) FROM "order"
WHERE "product_id" = $1 AND "customer_id" = $2 AND "status" = 'handled';

-- name: CreateAddress :one
INSERT INTO "address" (
    "name", "phone", "detail"
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: DeleteAddress :exec
DELETE FROM "address"
WHERE "id" = $1;