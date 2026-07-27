// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package trace

import (
	"errors"
	"fmt"
	"strings"
)

// Exporters this package can construct. ZAP is the transport; there is no
// other. gRPC, HTTP/protobuf and OTLP are not options here — a build of this
// package pulls none of them.
const (
	Disabled ExporterType = iota
	ZAP
)

var (
	errUnknownExporterType = errors.New("unknown exporter type")
	errMissingQuotes       = errors.New("first and last characters should be quotes")
)

func ExporterTypeFromString(exporterTypeStr string) (ExporterType, error) {
	switch strings.ToLower(exporterTypeStr) {
	case "disabled":
		return Disabled, nil
	case "zap":
		return ZAP, nil
	default:
		return 0, fmt.Errorf("%w: %q", errUnknownExporterType, exporterTypeStr)
	}
}

type ExporterType byte

func (t ExporterType) MarshalJSON() ([]byte, error) {
	str, ok := t.toString()
	if !ok {
		return nil, fmt.Errorf("%w: %d", errUnknownExporterType, t)
	}
	return []byte(`"` + str + `"`), nil
}

func (t *ExporterType) UnmarshalJSON(b []byte) error {
	str := string(b)
	if str == "null" { // If "null", do nothing
		return nil
	}
	if len(str) < 2 {
		return errMissingQuotes
	}

	lastIndex := len(str) - 1
	if str[0] != '"' || str[lastIndex] != '"' {
		return errMissingQuotes
	}

	exporterType, err := ExporterTypeFromString(str[1:lastIndex])
	if err != nil {
		return err
	}
	*t = exporterType
	return nil
}

func (t ExporterType) String() string {
	str, _ := t.toString()
	return str
}

func (t ExporterType) toString() (string, bool) {
	switch t {
	case Disabled:
		return "disabled", true
	case ZAP:
		return "zap", true
	default:
		return "unknown", false
	}
}
