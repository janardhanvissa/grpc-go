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
	"time"

	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "google.golang.org/grpc/examples/features/proto/echo"
)

var addr = flag.String("addr", ":50051", "the server address to connect to")

func initOpenCensus() {
	// Initialize OpenCensus tracing with AlwaysSample sampler
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	log.Println("OpenCensus tracing initialized with B3 propagation")
}

func makeGRPCCallWithOpenCensus(client pb.EchoClient) {
	ctx, span := trace.StartSpan(context.Background(), "OpenCensus.ClientCall")
	defer span.End()

	span.AddAttributes(trace.StringAttribute("rpc.method", "UnaryEcho"))

	// Making the gRPC call
	resp, err := client.UnaryEcho(ctx, &pb.EchoRequest{Message: "Hello from OpenCensus"})
	if err != nil {
		log.Printf("OpenCensus call failed: %v", err)
		return
	}

	span.AddAttributes(trace.StringAttribute("response.message", resp.Message))
	fmt.Printf("OpenCensus Response: %s\n", resp.Message)
}

func main() {
	flag.Parse()

	// Initialize OpenCensus tracing
	initOpenCensus()

	// Create a gRPC client connection with OpenCensus stats handler
	conn, err := grpc.Dial(*addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(&ocgrpc.ClientHandler{}),
	)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := pb.NewEchoClient(conn)

	// Make gRPC calls with OpenCensus
	for {
		makeGRPCCallWithOpenCensus(client)
		time.Sleep(2 * time.Second)
	}
}
