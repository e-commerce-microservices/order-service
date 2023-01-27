-- name: CreateOrder :exec
INSERT INTO "order" (
    "customer_id", "supplier_id", "product_id", "quantity"
) VALUES (
    $1, $2, $3, $4
);

-- name: UpdateOrderStatus :exec
UPDATE "order"
SET "status" = $1
WHERE "id" = $2;


-- name: GetWaitingOrderBySupplier :many
SELECT * FROM "order"
WHERE "supplier_id" = $1;

-- name: GetWaitingOrderByCustomer :many
SELECT * FROM "order"
WHERE "customer_id" = $1;