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
	"google.golang.org/grpc/credentials/insecure"
	pb "google.golang.org/grpc/examples/features/proto/echo"
)

var (
	// addr is the server address to connect to.
	addr = flag.String("addr", ":50051", "The server address to connect to")
	// addr is the server address to connect to.
	prometheusEndpoint = flag.String("prometheus_endpoint", ":9466", "The Prometheus exporter endpoint")
)

var (
	mRequests = stats.Int64("echo/requests", "The number of requests received", stats.UnitDimensionless)
)

type EchoService struct {
	pb.UnimplementedEchoServer
}

func (s *EchoService) UnaryEcho(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	// Start OpenCensus tracing for each request
	ctx, span := trace.StartSpan(ctx, "EchoService.UnaryEcho")
	defer span.End()

	// Record the metrics
	stats.Record(ctx, mRequests.M(1))

	// Simulate processing delay
	time.Sleep(100 * time.Millisecond)

	// Return the response
	return &pb.EchoResponse{Message: fmt.Sprintf("Hello, %s!", req.Message)}, nil
}

func init() {
	// Register OpenCensus views for metrics
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
	// Start Prometheus HTTP server for metrics
	go func() {
		log.Printf("Prometheus server running on %s", *prometheusEndpoint)
		if err := http.ListenAndServe(*prometheusEndpoint, promhttp.Handler()); err != nil {
			log.Fatalf("Failed to start Prometheus server: %v", err)
		}
	}()

	// Create gRPC server and register Echo service
	grpcServer := grpc.NewServer()
	pb.RegisterEchoServer(grpcServer, &EchoService{})

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Failed to listen on %v: %v", addr, err)
	}

	// Start the gRPC server
	log.Printf("Server listening on %s", *addr)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("Failed to serve gRPC server: %v", err)
		}
	}()

	// Create gRPC client and call Echo service
	callEchoServiceClient()
}

func callEchoServiceClient() {
	// Set up the client-side gRPC connection
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to newclient: %v", err)
	}
	defer conn.Close()
	client := pb.NewEchoClient(conn)

	// Create a context with tracing for the client call
	ctx, span := trace.StartSpan(context.Background(), "EchoClient.UnaryEcho")
	defer span.End()

	// Make the UnaryEcho RPC call
	req := &pb.EchoRequest{Message: "OpenCensus"}
	_, err = client.UnaryEcho(ctx, req)
	if err != nil {
		log.Printf("Error calling UnaryEcho: %v", err)
	} else {
		log.Println("UnaryEcho call succeeded")
	}
}
