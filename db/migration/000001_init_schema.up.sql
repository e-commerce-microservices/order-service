
CREATE TYPE order_status_enum AS ENUM ('waiting', 'handled');

CREATE TABLE "order" (
    "id" serial8 PRIMARY KEY,
    "customer_id" serial8  NOT NULL,
    "supplier_id" serial8 NOT NULL,
    "product_id" serial8 NOT NULL,
    "quantity" integer NOT NULL DEFAULT 1,
    "status" order_status_enum DEFAULT 'waiting',
    "created_at" timestamptz NOT NULL DEFAULT (now())
);
