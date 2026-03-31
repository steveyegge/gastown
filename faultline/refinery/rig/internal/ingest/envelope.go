package ingest

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	maxEnvelopeSize = 200 << 20 // 200 MiB
	maxItemSize     = 1 << 20   // 1 MiB
)

// EnvelopeHeader is the first line of a Sentry envelope.
type EnvelopeHeader struct {
	EventID string `json:"event_id"`
	DSN     string `json:"dsn"`
	SentAt  string `json:"sent_at"`
}

// ItemHeader describes one item in the envelope.
type ItemHeader struct {
	Type   string `json:"type"`
	Length int    `json:"length"`
}

// EnvelopeItem is one parsed item from the envelope.
type EnvelopeItem struct {
	Type    string
	Payload json.RawMessage
}

// ParseEnvelope reads a Sentry envelope from an HTTP request.
// Handles gzip Content-Encoding transparently.
func ParseEnvelope(r *http.Request) (*EnvelopeHeader, []EnvelopeItem, error) {
	body := http.MaxBytesReader(nil, r.Body, maxEnvelopeSize)
	defer body.Close()

	var reader io.Reader = body
	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(body)
		if err != nil {
			return nil, nil, fmt.Errorf("gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("read body: %w", err)
	}

	return parseEnvelopeBytes(data)
}

func parseEnvelopeBytes(data []byte) (*EnvelopeHeader, []EnvelopeItem, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, maxItemSize), maxItemSize)

	// Line 1: envelope header.
	if !scanner.Scan() {
		return nil, nil, fmt.Errorf("empty envelope")
	}
	var hdr EnvelopeHeader
	if err := json.Unmarshal(scanner.Bytes(), &hdr); err != nil {
		return nil, nil, fmt.Errorf("envelope header: %w", err)
	}

	var items []EnvelopeItem
	remaining := data[len(scanner.Bytes())+1:] // skip past header + newline

	for len(remaining) > 0 {
		// Trim leading newlines between items.
		remaining = bytes.TrimLeft(remaining, "\n\r")
		if len(remaining) == 0 {
			break
		}

		// Item header line.
		nl := bytes.IndexByte(remaining, '\n')
		if nl < 0 {
			// Last line with no payload — could be an empty item header.
			break
		}
		headerLine := remaining[:nl]
		remaining = remaining[nl+1:]

		var ih ItemHeader
		if err := json.Unmarshal(headerLine, &ih); err != nil {
			// Skip malformed item headers.
			continue
		}

		// Read payload.
		var payload []byte
		if ih.Length > 0 {
			if ih.Length > len(remaining) {
				return nil, nil, fmt.Errorf("item payload truncated: want %d, have %d", ih.Length, len(remaining))
			}
			payload = remaining[:ih.Length]
			remaining = remaining[ih.Length:]
		} else {
			// No length: read until next newline or end.
			nl = bytes.IndexByte(remaining, '\n')
			if nl < 0 {
				payload = remaining
				remaining = nil
			} else {
				payload = remaining[:nl]
				remaining = remaining[nl+1:]
			}
		}

		if len(payload) > maxItemSize {
			continue // skip oversized items
		}

		items = append(items, EnvelopeItem{
			Type:    ih.Type,
			Payload: json.RawMessage(payload),
		})
	}

	return &hdr, items, nil
}

// ParseDSNProjectID extracts the project ID from a Sentry DSN string.
// DSN format: https://<key>@<host>/<project_id>
func ParseDSNProjectID(dsn string) (int64, error) {
	idx := strings.LastIndex(dsn, "/")
	if idx < 0 {
		return 0, fmt.Errorf("invalid DSN: no slash")
	}
	return strconv.ParseInt(dsn[idx+1:], 10, 64)
}
