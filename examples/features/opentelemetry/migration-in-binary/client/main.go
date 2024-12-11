/*
 *
 * Copyright 2024 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.opencensus.io/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "google.golang.org/grpc/examples/features/proto/echo"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// addr is the server address to connect to.
	addr = flag.String("addr", ":50051", "the server address to connect to")
	// prometheusEndpoint is the address for the Prometheus exporter.
	prometheusEndpoint = flag.String("prometheus_endpoint", ":9466", "the Prometheus exporter endpoint")
)

// makeGRPCCallWithOpenCensus performs a gRPC call using OpenCensus for tracing.
// It starts a new span for the call, sends a request to the Echo service, and
// logs the response message.
func makeGRPCCallWithOpenCensus(client pb.EchoClient, ctx context.Context) {
	// Start an OpenCensus span for the client call.
	ctx, span := trace.StartSpan(ctx, "OpenCensus.ClientCall")
	defer span.End()

	// Make the gRPC request.
	resp, err := client.UnaryEcho(ctx, &pb.EchoRequest{Message: "from OpenCensus"})
	if err != nil {
		log.Printf("OpenCensus call failed: %v", err)
		return
	}
	fmt.Printf("OpenCensus Response: %s\n", resp.Message)
}

// makeGRPCCallWithOpenTelemetry performs a gRPC call using OpenTelemetry for tracing.
// It starts a new span for the call, sends a request to the Echo service, and
// logs the response message.
func makeGRPCCallWithOpenTelemetry(client pb.EchoClient, ctx context.Context) {
	// Get the OpenTelemetry tracer.
	tracer := otel.Tracer("migration-example")
	// Start an OpenTelemetry span for the client call.
	ctx, span := tracer.Start(ctx, "OpenTelemetry.ClientCall")
	defer span.End()

	// Make the gRPC request.
	resp, err := client.UnaryEcho(ctx, &pb.EchoRequest{Message: "from OpenTelemetry"})
	if err != nil {
		log.Printf("OpenTelemetry call failed: %v", err)
		return
	}
	fmt.Printf("OpenTelemetry Response: %s\n", resp.Message)
}

func main() {
	flag.Parse()

	// Setup Prometheus exporter to collect and expose metrics.
	promExporter, err := prometheus.New()
	if err != nil {
		log.Fatalf("failed to create prometheus exporter: %v", err)
	}
	meterProvider := metric.NewMeterProvider(metric.WithReader(promExporter))
	defer meterProvider.Shutdown(context.Background())
	otel.SetMeterProvider(meterProvider)

	// Setup stdout trace exporter to export trace data to stdout.
	traceExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatalf("failed to create trace exporter: %v", err)
	}

	// Initialize the OpenTelemetry TracerProvider with batcher and sampler.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	// Start Prometheus HTTP server to expose metrics at the given endpoint.
	go func() {
		log.Printf("Prometheus server running on %s", *prometheusEndpoint)
		if err := http.ListenAndServe(*prometheusEndpoint, promhttp.Handler()); err != nil {
			log.Fatalf("Failed to start Prometheus server: %v", err)
		}
	}()

	// Connect to the gRPC server using insecure credentials.
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewEchoClient(conn)

	// Continuously make gRPC calls with OpenCensus and OpenTelemetry.
	for {
		ctx := context.Background()
		// Make a gRPC call using OpenCensus.
		makeGRPCCallWithOpenCensus(client, ctx)
		// Make a gRPC call using OpenTelemetry.
		makeGRPCCallWithOpenTelemetry(client, ctx)
		// Sleep for a second before making the next set of calls.
		time.Sleep(time.Second)
	}
}
