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
	addr               = flag.String("addr", ":50051", "the server address to connect to")
	prometheusEndpoint = flag.String("prometheus_endpoint", ":9466", "the Prometheus exporter endpoint")
)

func makeGRPCCallWithOpenCensus(client pb.EchoClient, ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "OpenCensus.ClientCall")
	defer span.End()

	resp, err := client.UnaryEcho(ctx, &pb.EchoRequest{Message: "from OpenCensus"})
	if err != nil {
		log.Printf("OpenCensus call failed: %v", err)
		return
	}
	fmt.Printf("OpenCensus Response: %s\n", resp.Message)
}

func makeGRPCCallWithOpenTelemetry(client pb.EchoClient, ctx context.Context) {
	tracer := otel.Tracer("migration-example")
	ctx, span := tracer.Start(ctx, "OpenTelemetry.ClientCall")
	defer span.End()

	resp, err := client.UnaryEcho(ctx, &pb.EchoRequest{Message: "from OpenTelemetry"})
	if err != nil {
		log.Printf("OpenTelemetry call failed: %v", err)
		return
	}
	fmt.Printf("OpenTelemetry Response: %s\n", resp.Message)
}

func main() {
	flag.Parse()

	// Setup Prometheus exporter
	promExporter, err := prometheus.New()
	if err != nil {
		log.Fatalf("failed to create prometheus exporter: %v", err)
	}
	meterProvider := metric.NewMeterProvider(metric.WithReader(promExporter))
	otel.SetMeterProvider(meterProvider)

	// Setup stdout trace exporter
	traceExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatalf("failed to create trace exporter: %v", err)
	}

	// Create TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	// Start Prometheus HTTP server
	go func() {
		log.Printf("Prometheus server running on %s", *prometheusEndpoint)
		if err := http.ListenAndServe(*prometheusEndpoint, promhttp.Handler()); err != nil {
			log.Fatalf("Failed to start Prometheus server: %v", err)
		}
	}()

	// Create gRPC connection
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewEchoClient(conn)

	// Make alternating calls using OpenCensus and OpenTelemetry
	for {
		ctx := context.Background()
		makeGRPCCallWithOpenCensus(client, ctx)
		makeGRPCCallWithOpenTelemetry(client, ctx)
		time.Sleep(time.Second)
	}
}
