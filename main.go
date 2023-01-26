package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/e-commerce-microservices/order-service/pb"
	"github.com/e-commerce-microservices/order-service/repository"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}
}
func main() {
	// init user db connection
	pgDSN := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWD"), os.Getenv("DB_DBNAME"),
	)

	orderDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer orderDB.Close()
	if err := orderDB.Ping(); err != nil {
		log.Fatal("can't ping to user db", err)
	}

	grpcServer := grpc.NewServer()

	authConn, err := grpc.Dial("auth-service:8080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("can't dial auth service", err)
	}
	authClient := pb.NewAuthServiceClient(authConn)

	// init queries
	queries := repository.New(orderDB)
	orderService := orderService{
		authClient: authClient,
		orderRepo:  *queries,
	}
	pb.RegisterOrderServiceServer(grpcServer, orderService)

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal("cannot create listener: ", err)
	}
	log.Printf("start gRPC server on %s", listener.Addr().String())
	err = grpcServer.Serve(listener)
	if err != nil {
		log.Fatal("cannot create grpc server: ", err)
	}
}
