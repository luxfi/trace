// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package trace

type ExporterConfig struct {
	Type ExporterType `json:"type"`

	// Endpoint to send traces to. If empty, the default endpoint will be used.
	Endpoint string `json:"endpoint"`

	// Headers to send with traces
	Headers map[string]string `json:"headers"`

	// If true, don't use TLS
	Insecure bool `json:"insecure"`
}
