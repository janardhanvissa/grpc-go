/*
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
 */

package opentelemetry

import (
	"context"
	"log"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
	otelinternaltracing "google.golang.org/grpc/stats/opentelemetry/internal/tracing"
)

type serverTracingHandler struct {
	options Options
	tracer  trace.Tracer
}

// initializeTraces initializes the tracer used for server-side OpenTelemetry
// tracing.
func (h *serverTracingHandler) initializeTraces() {
	if h.options.TraceOptions.TracerProvider == nil {
		log.Printf("TraceProvider is not provided in trace options")
		return
	}
	h.tracer = h.options.TraceOptions.TracerProvider.Tracer("grpc-open-telemetry")
}

// unaryInterceptor is a UnaryServerInterceptor that instruments unary RPCs
// with OpenTelemetry traces.
func (h *serverTracingHandler) unaryInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	return handler(ctx, req)
}

// streamInterceptor is a StreamServerInterceptor that instruments streaming RPCs
// with OpenTelemetry traces.
func (h *serverTracingHandler) streamInterceptor(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return handler(srv, ss)
}

// TagRPC is called at the beginning of each RPC attempt. It starts a new
// span for the attempt and injects the tracing context into metadata for
// propagation. Requires a preceding handler to have set rpcInfo.
func (h *serverTracingHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	ri := getRPCInfo(ctx)
	// Fetch the rpcInfo set by a previously registered stats handler
	// (like serverStatsHandler). Assumes this handler runs after one
	// that sets the rpcInfo in the context.
	if ri == nil {
		logger.Error("ctx passed into server side stats handler metrics event handling has no server attempt data present")
		return ctx
	}
	ctx, ai := h.traceTagRPC(ctx, ri.ai)
	return setRPCInfo(ctx, &rpcInfo{ai: ai})
}

// traceTagRPC populates context with new span data using the TextMapPropagator
// supplied in trace options and internal itracing.Carrier. It creates a new
// incoming carrier which extracts an existing span context (if present) by
// deserializing from provided context. If valid span context is extracted, it
// is set as parent of the new span otherwise new span remains the root span.
// If TextMapPropagator is not provided in the trace options, it returns context
// as is.
func (h *serverTracingHandler) traceTagRPC(ctx context.Context, ai *attemptInfo) (context.Context, *attemptInfo) {
	mn := strings.Replace(ai.method, "/", ".", -1)
	var span trace.Span
	ctx = otel.GetTextMapPropagator().Extract(ctx, otelinternaltracing.NewIncomingCarrier(ctx))
	// If the context.Context provided in `ctx` to tracer.Start(), contains a
	// span then the newly-created Span will be a child of that span,
	// otherwise it will be a root span.
	ctx, span = h.tracer.Start(ctx, mn, trace.WithSpanKind(trace.SpanKindServer))
	ai.traceSpan = span
	return ctx, ai
}

// HandleRPC handles per-RPC attempt stats events for tracing. It populates
// the trace span with data from the RPCStats.
func (h *serverTracingHandler) HandleRPC(ctx context.Context, rs stats.RPCStats) {
	ri := getRPCInfo(ctx)
	if ri == nil {
		logger.Error("ctx passed into server side tracing stats handler has no server call data present")
		return
	}
	populateSpan(rs, ri.ai)
}

// TagConn exists to satisfy stats.Handler for tracing.
func (h *serverTracingHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

// HandleConn exists to satisfy stats.Handler for tracing.
func (h *serverTracingHandler) HandleConn(context.Context, stats.ConnStats) {}
