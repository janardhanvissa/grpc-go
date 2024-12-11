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
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/stats/opentelemetry"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	pb "google.golang.org/grpc/examples/features/proto/echo"
)

var (
	// addr is the server address to connect to.
	addr = flag.String("addr", ":50051", "the server address to connect to")
	// prometheusEndpoint is the address for the Prometheus exporter.
	prometheusEndpoint = flag.String("prometheus_endpoint", ":9467", "the Prometheus exporter endpoint")
)

// echoServer is a gRPC server that implements the Echo service.
type echoServer struct {
	pb.UnimplementedEchoServer
	addr string // The address of the server
}

// UnaryEcho handles unary Echo requests.
func (s *echoServer) UnaryEcho(_ context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	return &pb.EchoResponse{Message: fmt.Sprintf("%s (from %s)", req.Message, s.addr)}, nil
}

func main() {
	flag.Parse()

	// Setup Prometheus exporter for metrics collection.
	exporter, err := prometheus.New()
	if err != nil {
		log.Fatalf("Failed to create Prometheus exporter: %v", err)
	}
	meterProvider := metric.NewMeterProvider(metric.WithReader(exporter))
	defer meterProvider.Shutdown(context.Background())

	// Setup In-Memory Span Exporter for tracing.
	spanExporter := tracetest.NewInMemoryExporter()
	spanProcessor := trace.NewSimpleSpanProcessor(spanExporter)
	tracerProvider := trace.NewTracerProvider(trace.WithSpanProcessor(spanProcessor))
	defer tracerProvider.Shutdown(context.Background())

	// Start Prometheus HTTP server.
	go func() {
		log.Printf("Prometheus server running on %s", *prometheusEndpoint)
		if err := http.ListenAndServe(*prometheusEndpoint, promhttp.Handler()); err != nil {
			log.Fatalf("Failed to start Prometheus server: %v", err)
		}
	}()

	// Create a text map propagator for gRPC.
	textMapPropagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
		opentelemetry.GRPCTraceBinPropagator{})

	// Create server options for OpenTelemetry integration.
	serverOptions := opentelemetry.ServerOption(opentelemetry.Options{
		MetricsOptions: opentelemetry.MetricsOptions{
			MeterProvider: meterProvider,
		},
		TraceOptions: opentelemetry.TraceOptions{
			TracerProvider:    tracerProvider,
			TextMapPropagator: textMapPropagator,
		},
	})

	// Create a listener on the specified address
	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create the gRPC server with the OpenTelemetry options applied.
	s := grpc.NewServer(serverOptions)

	// Register the Echo service with the server.
	pb.RegisterEchoServer(s, &echoServer{addr: *addr})

	log.Printf("Serving on %s\n", *addr)

	// Setup OS signal handler to gracefully shut down the server when
	// interrupted.
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		// Wait for an interrupt signal and gracefully stop the server.
		<-signalChan
		log.Println("Shutting down server...")
		s.GracefulStop()
	}()

	// Start the gRPC server to handle incoming requests.
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
