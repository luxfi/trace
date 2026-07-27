// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Legacy OTLP exporter is gone. ZAP-native is the only real export
// path (tracer.go.New dispatches Type=ZAP to newZAPNativeExporter).
// Any type other than ZAP falls through to newExporter and resolves to an
// error so callers see a clear migration message instead of a
// silently-noop tracer.

package trace

import (
	"errors"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var errLegacyOTLPDisabled = errors.New(
	"trace: legacy OTLP exporter is removed — use Type=ZAP (native, no grpc)",
)

func newExporter(_ ExporterConfig) (sdktrace.SpanExporter, error) {
	return nil, errLegacyOTLPDisabled
}
