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
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	pb "google.golang.org/grpc/examples/features/proto/echo"
)

var (
	// addr is the server address to connect to.
	addr = flag.String("addr", ":50051", "the server address to connect to")
	// addr is the server address to connect to.
	prometheusEndpoint = flag.String("prometheus_endpoint", ":9467", "the Prometheus exporter endpoint")
)

type echoServer struct {
	pb.UnimplementedEchoServer
}

func (s *echoServer) UnaryEcho(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	// Handle the unary echo request
	return &pb.EchoResponse{Message: req.Message}, nil
}

func main() {
	flag.Parse()

	// Set up the Prometheus exporter for metrics
	exporter, err := prometheus.New()
	if err != nil {
		log.Fatalf("Failed to create Prometheus exporter: %v", err)
	}
	meterProvider := metric.NewMeterProvider(metric.WithReader(exporter))
	defer meterProvider.Shutdown(context.Background())

	// Set up the In-Memory Span Exporter for tracing
	spanExporter := tracetest.NewInMemoryExporter()
	spanProcessor := trace.NewSimpleSpanProcessor(spanExporter)
	tracerProvider := trace.NewTracerProvider(trace.WithSpanProcessor(spanProcessor))
	defer tracerProvider.Shutdown(context.Background())

	// Set up the OpenTelemetry server options
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)

	// Start Prometheus HTTP server
	go func() {
		log.Printf("Prometheus server running on %s", *prometheusEndpoint)
		if err := http.ListenAndServe(*prometheusEndpoint, promhttp.Handler()); err != nil {
			log.Fatalf("Failed to start Prometheus server: %v", err)
		}
	}()

	// Create a listener on the specified address
	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create the gRPC server and register the Echo service
	s := grpc.NewServer()

	// Register the Echo service with the server
	pb.RegisterEchoServer(s, &echoServer{})

	log.Printf("Serving on %s\n", *addr)

	// Handle OS signals for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		log.Println("Shutting down server...")
		s.GracefulStop()
	}()

	// Start serving requests
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
