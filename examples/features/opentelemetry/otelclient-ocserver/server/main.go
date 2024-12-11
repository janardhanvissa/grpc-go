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
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
	pb "google.golang.org/grpc/examples/features/proto/echo"
)

var (
	// addr is the server address to connect to.
	addr = flag.String("addr", ":50051", "the server address to connect to")
	// prometheusEndpoint is the address for the Prometheus exporter.
	prometheusEndpoint = flag.String("prometheus_endpoint", ":9466", "the Prometheus exporter endpoint")
)

var (
	// mRequests is a measure to track the number of Echo requests received by
	// the server.
	mRequests = stats.Int64("echo/requests", "The number of requests received", stats.UnitDimensionless)
)

type EchoService struct {
	pb.UnimplementedEchoServer
}

// UnaryEcho is a gRPC method implementation that handles unary RPC calls.
func (s *EchoService) UnaryEcho(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	// Start OpenCensus tracing for each request to measure the time and track
	// the request flow.
	ctx, span := trace.StartSpan(ctx, "EchoService.UnaryEcho")
	defer span.End()

	// Record the number of requests.
	stats.Record(ctx, mRequests.M(1))

	// Simulate some processing delay.
	time.Sleep(100 * time.Millisecond)

	return &pb.EchoResponse{Message: fmt.Sprintf("Hello, %s!", req.Message)}, nil
}

func init() {
	// Register OpenCensus views to collect metrics, such as the count of Echo
	// requests received.
	if err := view.Register(
		&view.View{
			Name:        "echo/requests_count",
			Description: "The count of Echo requests received",
			Measure:     mRequests,
			Aggregation: view.Count(),
		},
	); err != nil {
		log.Fatalf("Failed to register view: %v", err)
	} else {
		log.Println("Metrics view registered successfully")
	}
}

func main() {
	// Start the Prometheus HTTP server to expose metrics.
	go func() {
		log.Printf("Prometheus server running on %s", *prometheusEndpoint)
		if err := http.ListenAndServe(*prometheusEndpoint, promhttp.Handler()); err != nil {
			log.Fatalf("Failed to start Prometheus server: %v", err)
		}
	}()

	// Create a new gRPC server and register the Echo service implementation.
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(grpcServerInterceptor))
	pb.RegisterEchoServer(grpcServer, &EchoService{})

	// Start listening for incoming connections on the specified address.
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Failed to listen on %v: %v", addr, err)
	}

	// Start the gRPC server.
	log.Printf("Server listening on gRPC port: %s", *addr)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve gRPC server: %v", err)
	}
}

// grpcServerInterceptor is a gRPC server interceptor to handle tracing for each
// RPC call.
func grpcServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Start a trace span for the incoming gRPC call.
	ctx, span := trace.StartSpan(ctx, info.FullMethod)
	defer span.End()

	// Call the gRPC handler.
	resp, err := handler(ctx, req)

	span.AddAttributes(
		trace.StringAttribute("method", info.FullMethod),
	)
	// If an error occurred, update the trace span status.
	if err != nil {
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInternal,
			Message: err.Error(),
		})
	}
	return resp, err
}
