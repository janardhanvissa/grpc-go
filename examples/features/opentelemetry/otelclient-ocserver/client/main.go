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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/examples/features/proto/echo"
	"google.golang.org/grpc/stats/opentelemetry"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

var (
	// addr is the server address to connect to.
	addr = flag.String("addr", ":50051", "the server address to connect to")
	// prometheusEndpoint is the address for the Prometheus exporter.
	prometheusEndpoint = flag.String("prometheus_endpoint", ":9465", "the Prometheus exporter endpoint")
)

func main() {
	// Initialize the Prometheus exporter for metrics.
	exporter, err := prometheus.New()
	if err != nil {
		log.Fatalf("Failed to start prometheus exporter: %v", err)
	}
	// Set up the meter provider for metrics, which uses the Prometheus exporter.
	provider := metric.NewMeterProvider(metric.WithReader(exporter))

	// Set up in-memory span exporter for trace data, and a simple span processor
	// to process spans.
	spanExporter := tracetest.NewInMemoryExporter()
	spanProcessor := trace.NewSimpleSpanProcessor(spanExporter)
	// Initialize the tracer provider with the span processor.
	tracerProvider := trace.NewTracerProvider(trace.WithSpanProcessor(spanProcessor))
	// Set up the text map propagator for trace propagation across processes.
	textMapPropagator := propagation.NewCompositeTextMapPropagator(opentelemetry.GRPCTraceBinPropagator{})
	// Define trace options, including the tracer provider and propagator.
	traceOptions := opentelemetry.TraceOptions{
		TracerProvider:    tracerProvider,
		TextMapPropagator: textMapPropagator,
	}

	// Start the HTTP server to expose Prometheus metrics at the specified endpoint.
	go http.ListenAndServe(*prometheusEndpoint, promhttp.Handler())

	ctx := context.Background()
	// Set up the gRPC dial options, including metrics and trace options.
	do := opentelemetry.DialOption(opentelemetry.Options{
		MetricsOptions: opentelemetry.MetricsOptions{MeterProvider: provider},
		TraceOptions:   traceOptions,
	})

	// Establish a connection to the gRPC server using insecure credentials.
	cc, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()), do)
	if err != nil {
		log.Fatalf("Failed to start NewClient: %v", err)
	}
	defer cc.Close()

	// Create the Echo gRPC client.
	c := echo.NewEchoClient(cc)

	// Make an RPC every second. This should trigger telemetry on prometheus
	// server along with traces in the in memory exporter to be emitted from
	// the client and the server.
	for {
		// Make the UnaryEcho RPC call to the Echo service.
		r, err := c.UnaryEcho(ctx, &echo.EchoRequest{Message: "this is examples/opentelemetry"})
		if err != nil {
			log.Fatalf("UnaryEcho failed: %v", err)
		}
		fmt.Println(r)

		// Print the trace spans collected by the in-memory span exporter.
		for _, span := range spanExporter.GetSpans() {
			fmt.Printf("span: %v, %v\n", span.Name, span.SpanKind)
		}

		// Sleep for 1 second before sending the next RPC call.
		time.Sleep(time.Second)
	}
}
