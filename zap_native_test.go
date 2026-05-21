// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package trace_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/trace"
	"github.com/luxfi/zap"
)

// TestZAPNativeExporterRoundTrip wires the exporter against a stand-in
// ZAP receiver and proves spans flow over the wire as JSON inside a ZAP
// envelope tagged MsgSpanBatch.
//
// Stand-in (not the o11y receiver) so the trace package stays free of
// the o11y dependency.
func TestZAPNativeExporterRoundTrip(t *testing.T) {
	var (
		mu      sync.Mutex
		payload []byte
		done    = make(chan struct{}, 1)
	)

	port := freePort(t)
	srv := zap.NewNode(zap.NodeConfig{
		NodeID:      "test-receiver",
		ServiceType: "_o11y._tcp",
		Port:        port,
		NoDiscovery: true,
	})
	srv.Handle(trace.MsgSpanBatch, func(_ context.Context, _ string, m *zap.Message) (*zap.Message, error) {
		mu.Lock()
		payload = append([]byte(nil), m.Root().Bytes(0)...)
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
		return nil, nil
	})
	if err := srv.Start(); err != nil {
		t.Fatalf("server: %v", err)
	}
	defer srv.Stop()

	tracer, err := trace.New(trace.Config{
		ExporterConfig: trace.ExporterConfig{
			Type:     trace.ZAP,
			Endpoint: fmt.Sprintf("127.0.0.1:%d", port),
		},
		AppName:         "trace-test",
		Version:         "v0.0.0",
		TraceSampleRate: 1,
	})
	if err != nil {
		t.Fatalf("tracer: %v", err)
	}
	defer tracer.Close()

	_, span := tracer.Start(context.Background(), "round-trip")
	span.End()

	// Force a flush by closing the tracer; the BatchSpanProcessor drains
	// to the exporter on shutdown.
	if err := tracer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for span batch")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(payload) == 0 {
		t.Fatal("received empty payload")
	}

	var batch struct {
		AppName string `json:"appName"`
		Spans   []struct {
			Name string `json:"name"`
		} `json:"spans"`
	}
	if err := json.Unmarshal(payload, &batch); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if batch.AppName != "trace-test" {
		t.Fatalf("appName: got %q want %q", batch.AppName, "trace-test")
	}
	if len(batch.Spans) != 1 || batch.Spans[0].Name != "round-trip" {
		t.Fatalf("spans: %+v", batch.Spans)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}
