//go:build !grpc && !otlp

// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Default build: no OTLP/protobuf exporter. Callers who want OTLP export
// must opt in with -tags otlp (HTTP+protobuf) or -tags grpc (legacy gRPC).
//
// With this file compiled, trace.New(Config{Type: Disabled}) returns
// trace.Noop and no go.opentelemetry.io/proto/otlp/* or
// google.golang.org/protobuf import is pulled into the binary.

package trace

import (
	"errors"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// errNoOTLPExporter is returned when a caller asks for a real exporter on
// a default (otlp-untagged) build. Rebuild with -tags otlp to enable
// OTLP/HTTP export, or set Config.ExporterConfig.Type = Disabled to use
// the noop tracer.
var errNoOTLPExporter = errors.New(
	"trace: OTLP exporter not compiled (rebuild with -tags otlp or use ExporterConfig.Type=Disabled)",
)

// newExporter — default build returns an error. Callers should set
// Type=Disabled (which short-circuits before this is reached in
// trace.New) or rebuild with -tags otlp.
func newExporter(_ ExporterConfig) (sdktrace.SpanExporter, error) {
	return nil, errNoOTLPExporter
}
