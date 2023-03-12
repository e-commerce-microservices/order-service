package main

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"

	_ "net/http/pprof"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func main() {
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

	fmt.Println("server run on port", 6001)
	if err := http.ListenAndServe(":6001", nil); err != nil {
		log.Println("err: ", err)
	}
}

func getListMessage() []string {
	return make([]string, 0, 2<<19)
}

func jaegerTraceProvider() (*sdktrace.TracerProvider, error) {

	// exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://jaeger-all-in-one:14268/api/traces")))
	// 31741
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://192.168.49.2:30444/api/traces")))
	// exp, err := jaeger.New(jaeger.WithAgentEndpoint(jaeger.WithAgentHost("10.3.68.12")))
	if err != nil {
		log.Println("err: ", err)
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("tracing"),
			attribute.String("environment", "development"),
		)),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(1.0)),
	)

	return tp, nil
}
