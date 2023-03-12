package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/e-commerce-microservices/order-service/pb"
	"github.com/e-commerce-microservices/order-service/repository"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"

	// postgres driver
	"net/http"
	_ "net/http/pprof"

	_ "github.com/lib/pq"
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}
	tp, tpErr := jaegerTraceProvider()
	if tpErr != nil {
		log.Fatal(tpErr)
	}
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
}
func jaegerTraceProvider() (*sdktrace.TracerProvider, error) {

	// exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://jaeger-all-in-one:14268/api/traces")))
	// exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://jaeger-all-in-one:14268/api/traces")))
	exp, err := jaeger.New(jaeger.WithAgentEndpoint(jaeger.WithAgentHost("10.3.68.12")))
	if err != nil {
		log.Println("err: ", err)
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("order service"),
			attribute.String("environment", "development"),
		)),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(1.0)),
	)

	return tp, nil
}

func getListMessage() []string {
	return make([]string, 0, 2<<20)
}

func main() {
	// pprofiling
	go func() {
		var memory = make(chan bool, 1)

		http.HandleFunc("/memory-leak", func(w http.ResponseWriter, r *http.Request) {
			var headers = [][]string{}
			for {
				select {
				case <-memory:
					for _, header := range headers {
						header = nil
						_ = header
					}
					headers = nil
					runtime.GC()
					w.Write([]byte("stop memory leak"))
					return
				default:
					time.Sleep(time.Millisecond * 500)
					headers = append(headers, getListMessage()[:5])
				}
			}
		})
		http.HandleFunc("/memory-leak/stop", func(w http.ResponseWriter, r *http.Request) {
			memory <- true
			w.Write([]byte("stop memory leak"))
		})

		var cpu = make(chan bool, 1)
		http.HandleFunc("/cpu-consume", func(w http.ResponseWriter, r *http.Request) {
			for {
				select {
				case <-cpu:
					w.Write([]byte("stop cpu consume"))
					return
				default:
					for i := 0; i < 100000; i++ {
					}
				}
			}
		})
		http.HandleFunc("/cpu-consume/stop", func(w http.ResponseWriter, r *http.Request) {
			cpu <- true
			w.Write([]byte("stop cpu consume"))
		})
		http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("pong"))
		})

		fmt.Println("server run on port", 6000)
		if err := http.ListenAndServe(":6000", nil); err != nil {
			log.Println("err: ", err)
		}
	}()

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

	authConn, err := grpc.Dial("auth-service:8080", grpc.WithInsecure(), grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	if err != nil {
		log.Fatal("can't dial auth service", err)
	}
	authClient := pb.NewAuthServiceClient(authConn)

	productConn, err := grpc.Dial("product-service:8080", grpc.WithInsecure(), grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	if err != nil {
		log.Fatal("can't dial product service", err)
	}
	productClient := pb.NewProductServiceClient(productConn)

	cartConn, err := grpc.Dial("cart-service:8080", grpc.WithInsecure(), grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	if err != nil {
		log.Fatal("can't dial cart service", err)
	}
	cartClient := pb.NewCartServiceClient(cartConn)

	// init queries
	queries := repository.New(orderDB)
	orderService := orderService{
		authClient:    authClient,
		orderRepo:     *queries,
		cartClient:    cartClient,
		productClient: productClient,
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
