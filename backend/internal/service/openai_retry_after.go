package service

import (
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// Capture the provider deadline once; nested SSE/WS error advice takes
// precedence over event headers and the HTTP response/upgrade headers.
func openAIUpstreamRetryAfter(headers http.Header, payload []byte) (string, time.Time) {
	values := make([]string, 0, 4)
	for _, path := range []string{"error.headers", "response.error.headers", "headers"} {
		gjson.GetBytes(payload, path).ForEach(func(key, value gjson.Result) bool {
			if strings.EqualFold(key.String(), "Retry-After") && value.Type == gjson.String {
				values = append(values, value.Str)
			}
			return true
		})
	}
	values = append(values, headers.Get("Retry-After"))
	now := time.Now()
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		digits := true
		for _, c := range raw {
			digits = digits && c >= '0' && c <= '9'
		}
		if digits {
			if delay, err := time.ParseDuration(raw + "s"); err == nil {
				return raw, now.Add(delay)
			}
		} else if deadline, err := http.ParseTime(raw); err == nil {
			return raw, deadline
		}
	}
	return "", time.Time{}
}
