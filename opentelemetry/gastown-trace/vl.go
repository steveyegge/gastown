package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LogEvent is a single VictoriaLogs event (newline-delimited JSON).
type LogEvent map[string]string

func (e LogEvent) Time() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, e["_time"])
	return t
}
func (e LogEvent) Msg() string         { return e["_msg"] }
func (e LogEvent) Str(k string) string { return e[k] }

// query executes a LogsQL query and returns events newest-first.
func vlQuery(logsBase, query string, limit int, start, end time.Time) ([]LogEvent, error) {
	params := url.Values{}
	params.Set("query", query)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if !start.IsZero() {
		params.Set("start", start.UTC().Format(time.RFC3339))
	}
	if !end.IsZero() {
		params.Set("end", end.UTC().Format(time.RFC3339))
	}

	resp, err := http.Get(logsBase + "/select/logsql/query?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("vlQuery %q: %w", query, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var events []LogEvent
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if line == "" {
			continue
		}
		var ev LogEvent
		if json.Unmarshal([]byte(line), &ev) == nil {
			events = append(events, ev)
		}
	}
	return events, nil
}

// extractStreamField pulls a field from a VictoriaLogs _stream string.
// e.g. `{gt.role="mayor",gt.actor="mayor"}` → "mayor" for "gt.role"
func extractStreamField(stream, field string) string {
	key := field + `="`
	idx := strings.Index(stream, key)
	if idx < 0 {
		return ""
	}
	rest := stream[idx+len(key):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// extractArgQ pulls --flag="quoted value" or --flag='quoted value', falling back to extractArgLong.
func extractArgQ(args, flag string) string {
	for _, q := range []string{`"`, `'`} {
		key := flag + "=" + q
		if idx := strings.Index(args, key); idx >= 0 {
			rest := args[idx+len(key):]
			end := strings.Index(rest, q)
			if end >= 0 {
				return rest[:end]
			}
		}
	}
	return extractArgLong(args, flag)
}

// extractArgLong pulls --flag=value where value extends to the next --flag or end of string.
// Handles unquoted multi-word values like --description=some long text --next-flag.
func extractArgLong(args, flag string) string {
	key := flag + "="
	idx := strings.Index(args, key)
	if idx < 0 {
		return ""
	}
	rest := args[idx+len(key):]
	// Find the next -- flag
	end := strings.Index(rest, " --")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

// extractArg pulls --flag=value or --flag value from an args string.
func extractArg(args, flag string) string {
	// Try --flag=value
	key := flag + "="
	if idx := strings.Index(args, key); idx >= 0 {
		rest := args[idx+len(key):]
		end := strings.IndexAny(rest, " \n\t")
		if end < 0 {
			return rest
		}
		return rest[:end]
	}
	// Try --flag value
	if idx := strings.Index(args, flag+" "); idx >= 0 {
		rest := strings.TrimSpace(args[idx+len(flag)+1:])
		end := strings.IndexAny(rest, " \n\t")
		if end < 0 {
			return rest
		}
		return rest[:end]
	}
	return ""
}

// firstPositionalArg returns the first non-flag word in an args string.
func firstPositionalArg(args string) string {
	for _, tok := range strings.Fields(args) {
		if !strings.HasPrefix(tok, "-") {
			return tok
		}
	}
	return ""
}
