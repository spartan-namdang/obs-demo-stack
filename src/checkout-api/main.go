package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var inventoryServiceURL = os.Getenv("INVENTORY_SERVICE_URL")

func initTracer() func(context.Context) error {
	// Sends traces to Tempo (via OTel Collector or direct)
	ctx := context.Background()
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint("tempo.monitoring.svc.cluster.local:4317"))
	if err != nil {
		log.Fatalf("failed to create trace exporter: %v", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("checkout-api"),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp.Shutdown
}

func main() {
	shutdown := initTracer()
	defer shutdown(context.Background())

	r := gin.Default()

	// Add Prometheus Middleware
	p := ginprometheus.NewPrometheus("gin")
	p.Use(r) // Exposes /metrics automatically

	r.Use(otelgin.Middleware("checkout-api")) // Auto-instrument Gin

	r.GET("/ping", func(c *gin.Context) {
		// Log with TraceID for Loki correlation
		span := trace.SpanFromContext(c.Request.Context())
		fmt.Printf("{\"level\":\"info\",\"msg\":\"Received ping request\",\"trace_id\":\"%s\"}\n", span.SpanContext().TraceID().String())

		// Call Inventory Service
		client := http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
		resp, err := client.Get(inventoryServiceURL + "/data")
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		defer resp.Body.Close()

		c.JSON(200, gin.H{"message": "Pong from checkout-api", "inventory_service_status": resp.Status})
	})

	r.Run(":8080")
}
