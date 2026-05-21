//go:build !grpc && !otlp

// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Default build:
//   - ZAP-native exporter is always compiled (exporter_zap.go, no tag)
//   - OTLP/HTTP+protobuf exporter is gated behind -tags otlp
//   - legacy OTLP/gRPC exporter is gated behind -tags grpc
//
// This file supplies newExporter for the otlp-untagged build, used by
// trace.New only when Type ∈ {HTTP, GRPC}. Type=ZAP and Type=Disabled
// never reach this path — they're handled directly in tracer.go.

package trace

import (
	"errors"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var errNoOTLPExporter = errors.New(
	"trace: OTLP exporter not compiled (rebuild with -tags otlp or set ExporterConfig.Type=ZAP for the native transport)",
)

func newExporter(_ ExporterConfig) (sdktrace.SpanExporter, error) {
	return nil, errNoOTLPExporter
}
