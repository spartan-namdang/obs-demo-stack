package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

func initTracer() func(context.Context) error {
	ctx := context.Background()
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint("tempo.monitoring.svc.cluster.local:4317"))
	if err != nil {
		log.Fatalf("failed to create trace exporter: %v", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("shipping-worker"),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp.Shutdown
}

func main() {
	shutdown := initTracer()
	defer shutdown(context.Background())

	tracer := otel.Tracer("shipping-worker")

	fmt.Println("Shipping Worker Starting...")

	// AWS/Localstack Config
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
		config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(
				func(service, region string, options ...interface{}) (aws.Endpoint, error) {
					if service == sqs.ServiceID {
						return aws.Endpoint{
							URL:               os.Getenv("SQS_ENDPOINT"),
							HostnameImmutable: true,
						}, nil
					}
					return aws.Endpoint{}, &aws.EndpointNotFoundError{}
				},
			),
		),
	)
	if err != nil {
		fmt.Printf("Unable to load AWS SDK config: %v\n", err)
	}

	client := sqs.NewFromConfig(cfg)
	queueURL := os.Getenv("SQS_QUEUE_URL")

	for {
		// Start a root span for the polling cycle
		ctx, span := tracer.Start(context.Background(), "poll-sqs")

		output, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &queueURL,
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     5,
		})

		if err != nil {
			fmt.Printf("Error receiving message: %v\n", err)
		} else if len(output.Messages) > 0 {
			for _, msg := range output.Messages {
				processMessage(ctx, tracer, msg.Body)
				client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
					QueueUrl:      &queueURL,
					ReceiptHandle: msg.ReceiptHandle,
				})
			}
		}
		span.End()
		time.Sleep(1 * time.Second)
	}
}

func processMessage(ctx context.Context, tracer trace.Tracer, body *string) {
	_, span := tracer.Start(ctx, "process_message")
	defer span.End()

	// Simulate work
	time.Sleep(50 * time.Millisecond)
	fmt.Printf("{\"level\":\"info\",\"msg\":\"Consumed message\",\"body\":%q}\n", *body)
}
