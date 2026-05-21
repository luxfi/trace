//go:build grpc

// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package trace

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const tracerProviderExportCreationTimeout = 5 * time.Second

// ExporterConfig is declared in exporter_config.go (no build tag) so
// the same shape is available to callers regardless of -tags grpc.

func newExporter(config ExporterConfig) (sdktrace.SpanExporter, error) {
	var client otlptrace.Client
	switch config.Type {
	case GRPC:
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithHeaders(config.Headers),
			otlptracegrpc.WithTimeout(tracerExportTimeout),
		}
		if config.Endpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(config.Endpoint))
		}
		if config.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		client = otlptracegrpc.NewClient(opts...)
	case HTTP, ZAP:
		// ZAP falls back to HTTP in gRPC build since both are available
		opts := []otlptracehttp.Option{
			otlptracehttp.WithHeaders(config.Headers),
			otlptracehttp.WithTimeout(tracerExportTimeout),
		}
		if config.Endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(config.Endpoint))
		}
		if config.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		client = otlptracehttp.NewClient(opts...)
	default:
		return nil, errUnknownExporterType
	}

	ctx, cancel := context.WithTimeout(context.Background(), tracerProviderExportCreationTimeout)
	defer cancel()
	return otlptrace.New(ctx, client)
}
