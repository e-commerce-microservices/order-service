.PHONY: rebuild
rebuild:
	docker build -t ngoctd/ecommerce-order:latest . && \
	docker push ngoctd/ecommerce-order

.PHONY: redeploy
redeploy:
	kubectl rollout restart deployment depl-order

.PHONY: protogen
protogen:
	protoc --proto_path=proto proto/order_service.proto proto/auth_service.proto proto/product_service.proto proto/general.proto \
	--go_out=pb --go_opt=paths=source_relative \
	--go-grpc_out=pb --go-grpc_opt=paths=source_relative

.PHONY: sqlcgen
sqlcgen:
	sqlc generate

.PHONY: migratecreate
migratecreate:
	migrate create -ext sql -dir db/migration -seq ${f}

DB_DSN := postgres://admin:admin@192.168.49.2:30014/order?sslmode=disable
.PHONY: migrateup
migrateup:
	migrate -path db/migration -database "${DB_DSN}" -verbose up ${v}

.PHONY: migratedown
migratedown:
	migrate -path db/migration -database "${DB_DSN}" -verbose down ${v}

.PHONY: migrateforce
migrateforce:
	migrate -path db/migration -database "${DB_DSN}" -verbose force ${v}