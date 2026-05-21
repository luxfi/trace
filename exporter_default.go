//go:build !grpc

// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Default-build OTLP fallback. The real OTLP-gRPC + OTLP-HTTP exporters
// live in exporter_grpc.go (//go:build grpc) — that path pulls
// google.golang.org/grpc. Default builds: callers asking for Type=GRPC
// or Type=HTTP get a noop so the host process stays grpc-free.
//
// Type=ZAP is the canonical real export path; it never reaches this
// function (tracer.go.New dispatches it directly to newZAPNativeExporter).

package trace

import (
	"errors"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var errLegacyOTLPDisabled = errors.New(
	"trace: legacy OTLP exporter not compiled (Type=ZAP is the canonical default; rebuild with -tags grpc to enable OTLP-gRPC)",
)

// newExporter (default build) returns errLegacyOTLPDisabled. The
// caller's Tracer comes back as Noop because tracer.go.New propagates
// this error and the host wraps it as "fall back to Noop". The grpc
// build replaces this with a real OTLP exporter in exporter_grpc.go.
func newExporter(_ ExporterConfig) (sdktrace.SpanExporter, error) {
	return nil, errLegacyOTLPDisabled
}
