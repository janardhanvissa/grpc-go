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

	otelgrpc "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	pb "google.golang.org/grpc/examples/features/proto/echo"
)

var addr = flag.String("addr", ":50051", "the server address to listen on")

type echoServer struct {
	pb.UnimplementedEchoServer
}

func (s *echoServer) UnaryEcho(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Received UnaryEcho request")
	log.Printf("Received message: %s", req.Message)

	resp := &pb.EchoResponse{Message: fmt.Sprintf("%s (response from server)", req.Message)}
	return resp, nil
}

func initOpenTelemetry() *sdktrace.TracerProvider {
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tracerProvider)
	return tracerProvider
}

func main() {
	flag.Parse()

	// Initialize OpenTelemetry tracing
	tracerProvider := initOpenTelemetry()
	defer tracerProvider.Shutdown(context.Background())

	// Create a gRPC server with OpenTelemetry interceptor
	server := grpc.NewServer(
		grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
	)

	// Register the Echo service with the server
	pb.RegisterEchoServer(server, &echoServer{})

	// Start listening for incoming connections
	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Printf("OpenTelemetry gRPC server listening on %s", *addr)
	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
