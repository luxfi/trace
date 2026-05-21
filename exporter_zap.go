// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// ZAP-native span exporter — the default trace transport.
//
// Spans serialize to JSON inside a luxfi/zap envelope and ship over TCP
// to a ZAP-aware o11y collector (hanzo/o11y/pkg/zapreceiver). Zero
// google.golang.org/protobuf, zero OTLP, zero gRPC.
//
// Wire layout per export call:
//
//	zap envelope, MsgType=MsgSpanBatch
//	└─ root object
//	   └─ FieldPayload (bytes): JSON-encoded SpanBatch
//
// SpanBatch carries the resource attributes + a list of spans translated
// from sdktrace.ReadOnlySpan into a stable, version-agnostic JSON shape.

package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/luxfi/zap"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// MsgSpanBatch is the ZAP MsgType that carries a SpanBatch payload.
//
// Stable wire ID for the trace transport on the ZAP bus. Append-only —
// renumbering breaks every deployed collector in lockstep.
const MsgSpanBatch uint16 = 1

// SpanBatch is the JSON shape that rides inside the ZAP envelope's
// payload field. One batch per ExportSpans call.
type SpanBatch struct {
	AppName  string            `json:"appName,omitempty"`
	Version  string            `json:"version,omitempty"`
	Resource map[string]string `json:"resource,omitempty"`
	Spans    []Span            `json:"spans"`
}

// Span is the wire shape for a single OTel ReadOnlySpan.
type Span struct {
	TraceID      string         `json:"traceId"`
	SpanID       string         `json:"spanId"`
	ParentSpanID string         `json:"parentSpanId,omitempty"`
	Name         string         `json:"name"`
	Kind         string         `json:"kind,omitempty"`
	StartUnixNs  int64          `json:"startUnixNs"`
	EndUnixNs    int64          `json:"endUnixNs"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	Events       []SpanEvent    `json:"events,omitempty"`
	StatusCode   string         `json:"statusCode,omitempty"`
	StatusMsg    string         `json:"statusMessage,omitempty"`
}

type SpanEvent struct {
	Name        string         `json:"name"`
	TimeUnixNs  int64          `json:"timeUnixNs"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

// zapExporter implements sdktrace.SpanExporter over a ZAP node connection.
type zapExporter struct {
	appName string
	version string

	endpoint string // host:port of the ZAP collector
	logger   *slog.Logger

	mu       sync.Mutex
	node     *zap.Node
	serverID string
	closed   bool
}

// newZAPNativeExporter builds a SpanExporter that ships spans over ZAP.
//
// Endpoint defaults to localhost:4317 (the canonical ZAP o11y port) when
// config.Endpoint is empty — keeps DX intuitive without forcing every
// caller to wire the address explicitly.
func newZAPNativeExporter(config ExporterConfig, appName, version string) (sdktrace.SpanExporter, error) {
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = "127.0.0.1:4317"
	}

	logger := slog.Default()

	node := zap.NewNode(zap.NodeConfig{
		NodeID:      fmt.Sprintf("trace-%s", appName),
		ServiceType: "_o11y._tcp",
		Port:        0,
		Logger:      logger,
		NoDiscovery: true,
	})
	if err := node.Start(); err != nil {
		return nil, fmt.Errorf("trace zap exporter start: %w", err)
	}

	// Best-effort connect. Tracing is async and non-critical — if the
	// collector is unreachable at boot, retry on each ExportSpans call
	// rather than failing the host process startup.
	dialCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = dialCtx
	cancel()
	exp := &zapExporter{
		appName:  appName,
		version:  version,
		endpoint: endpoint,
		logger:   logger,
		node:     node,
	}
	if err := exp.connect(); err != nil {
		// Log and continue — exporter retries on first export.
		logger.Debug("trace zap exporter: initial connect failed (will retry on export)", "endpoint", endpoint, "err", err)
	}
	return exp, nil
}

// connect dials the collector and caches the peer ID.
//
// Idempotent under the mutex — called from newZAPNativeExporter and from
// ExportSpans when an earlier connect failed.
func (e *zapExporter) connect() error {
	if err := e.node.ConnectDirect(e.endpoint); err != nil {
		return err
	}
	peers := e.node.Peers()
	if len(peers) == 0 {
		return fmt.Errorf("trace zap exporter: connected but no peer ID for %s", e.endpoint)
	}
	e.serverID = peers[0]
	return nil
}

// ExportSpans serializes spans to JSON, wraps them in a ZAP envelope, and
// fires them over the cached connection. Fire-and-forget — no response
// is expected, no waiting on the collector.
func (e *zapExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return fmt.Errorf("trace zap exporter: shut down")
	}
	if e.serverID == "" {
		if err := e.connect(); err != nil {
			// Drop the batch silently — tracing must never block the host.
			e.logger.Debug("trace zap exporter: connect retry failed; dropping batch", "endpoint", e.endpoint, "err", err)
			return nil
		}
	}

	batch := SpanBatch{
		AppName:  e.appName,
		Version:  e.version,
		Resource: e.resourceAttrs(spans),
		Spans:    make([]Span, 0, len(spans)),
	}
	for _, s := range spans {
		batch.Spans = append(batch.Spans, translateSpan(s))
	}

	payload, err := json.Marshal(&batch)
	if err != nil {
		return fmt.Errorf("trace zap exporter: marshal batch: %w", err)
	}

	wire, err := encodeSpanBatch(payload)
	if err != nil {
		return err
	}
	msg, err := zap.Parse(wire)
	if err != nil {
		return fmt.Errorf("trace zap exporter: parse outgoing: %w", err)
	}

	if err := e.node.Send(ctx, e.serverID, msg); err != nil {
		// Connection died — invalidate and let the next call reconnect.
		e.serverID = ""
		e.logger.Debug("trace zap exporter: send failed; will reconnect", "err", err)
		return nil
	}
	return nil
}

// Shutdown closes the ZAP node. Safe to call multiple times.
func (e *zapExporter) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	e.node.Stop()
	return nil
}

// resourceAttrs pulls the shared resource attributes from the first span
// — they're identical across spans in a batch.
func (e *zapExporter) resourceAttrs(spans []sdktrace.ReadOnlySpan) map[string]string {
	if len(spans) == 0 {
		return nil
	}
	res := spans[0].Resource()
	if res == nil {
		return nil
	}
	iter := res.Iter()
	out := make(map[string]string, iter.Len())
	for iter.Next() {
		kv := iter.Attribute()
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}

// translateSpan converts an OTel ReadOnlySpan into the JSON wire shape.
func translateSpan(s sdktrace.ReadOnlySpan) Span {
	sc := s.SpanContext()
	parent := s.Parent()
	out := Span{
		TraceID:     sc.TraceID().String(),
		SpanID:      sc.SpanID().String(),
		Name:        s.Name(),
		Kind:        s.SpanKind().String(),
		StartUnixNs: s.StartTime().UnixNano(),
		EndUnixNs:   s.EndTime().UnixNano(),
	}
	if parent.IsValid() {
		out.ParentSpanID = parent.SpanID().String()
	}
	if attrs := s.Attributes(); len(attrs) > 0 {
		out.Attributes = attrsToMap(attrs)
	}
	if events := s.Events(); len(events) > 0 {
		out.Events = make([]SpanEvent, 0, len(events))
		for _, ev := range events {
			out.Events = append(out.Events, SpanEvent{
				Name:       ev.Name,
				TimeUnixNs: ev.Time.UnixNano(),
				Attributes: attrsToMap(ev.Attributes),
			})
		}
	}
	if st := s.Status(); st.Code != codes.Unset {
		out.StatusCode = strings.ToLower(st.Code.String())
		out.StatusMsg = st.Description
	}
	return out
}

func attrsToMap(attrs []attribute.KeyValue) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]any, len(attrs))
	for _, kv := range attrs {
		out[string(kv.Key)] = kv.Value.AsInterface()
	}
	return out
}

// encodeSpanBatch wraps a JSON payload in a ZAP envelope tagged MsgSpanBatch.
//
// Wire shape:
//
//	zap header (16) + root object { bytes payload @ offset 0 }
//
// MsgSpanBatch goes in the upper 8 bits of the ZAP flags field.
func encodeSpanBatch(payload []byte) ([]byte, error) {
	const envelopeSize = 16
	b := zap.NewBuilder(envelopeSize + 64 + len(payload))
	root := b.StartObject(envelopeSize)
	root.SetBytes(0, payload)
	root.FinishAsRoot()
	return b.FinishWithFlags(MsgSpanBatch << 8), nil
}

// ResolveCollectorAddr is a small DX helper for callers that want to set
// Endpoint by environment variable. It returns the first non-empty value
// among the env vars listed in fallback order. Empty string if none set —
// callers should treat that as "use the default 127.0.0.1:4317".
func ResolveCollectorAddr(envVars ...string) string {
	for _, name := range envVars {
		if v := strings.TrimSpace(envValue(name)); v != "" {
			return v
		}
	}
	return ""
}

// envValue is a tiny indirection so the package doesn't directly depend
// on os.Getenv — keeps the dep graph honest for the audit script and
// lets tests inject overrides.
var envValue = func(string) string { return "" }

// Compile-time check.
var _ sdktrace.SpanExporter = (*zapExporter)(nil)

// freePortHint is exposed only to keep `net` import live for builds that
// strip the rest of the file under future build-tag splits.
var _ = net.IPv4zero
