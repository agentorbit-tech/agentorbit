package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// PerKeyIngestRateLimit is a chi middleware that enforces a per-API-key
// rate limit on /internal/spans/ingest. The api_key_id is parsed from the
// JSON body via a streaming decoder to avoid materialising the full
// payload twice. After the body is read, it is rewrapped with an
// io.NopCloser so downstream handlers can re-decode it.
//
// max is the number of requests permitted per window. Defaults: max=600,
// window=1 minute. The internal bucket map is bounded by maxBuckets and
// uses a TTL sweep to keep memory finite during sustained load.
//
// Buckets keyed by an unparseable / missing api_key_id are silently
// passed through (the handler will return 400) so a malformed body cannot
// pin global state.
func PerKeyIngestRateLimit(ctx context.Context, max int, window time.Duration) func(http.Handler) http.Handler {
	const maxBuckets = 50000
	const sweepInterval = 5 * time.Minute
	const ttl = 5 * time.Minute

	type bucket struct {
		timestamps []time.Time
		lastSeen   time.Time
	}

	var (
		mu      sync.Mutex
		buckets = make(map[string]*bucket)
	)

	go func() {
		t := time.NewTicker(sweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				mu.Lock()
				cutoff := time.Now().Add(-ttl)
				for k, b := range buckets {
					if b.lastSeen.Before(cutoff) {
						delete(buckets, k)
					}
				}
				mu.Unlock()
			}
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			_ = r.Body.Close()
			// Restore the body for the downstream handler before any early-exit path.
			r.Body = io.NopCloser(bytes.NewReader(body))

			apiKeyID := extractAPIKeyID(body)
			if apiKeyID == "" {
				// Unparseable / missing — let the handler decide (400).
				next.ServeHTTP(w, r)
				return
			}

			now := time.Now()
			mu.Lock()
			b, ok := buckets[apiKeyID]
			if !ok {
				if len(buckets) >= maxBuckets {
					// Evict the bucket with the oldest lastSeen — bounded
					// O(N) scan only at the cap. New keys win against
					// abandoned ones.
					var evictKey string
					var evictTime time.Time
					first := true
					for k, v := range buckets {
						if first || v.lastSeen.Before(evictTime) {
							evictKey = k
							evictTime = v.lastSeen
							first = false
						}
					}
					if evictKey != "" {
						delete(buckets, evictKey)
						slog.Warn("ingest rate limiter at capacity, evicting LRU", "max", maxBuckets)
					}
				}
				b = &bucket{}
				buckets[apiKeyID] = b
			}
			cutoff := now.Add(-window)
			valid := b.timestamps[:0]
			for _, ts := range b.timestamps {
				if ts.After(cutoff) {
					valid = append(valid, ts)
				}
			}
			b.timestamps = valid
			b.lastSeen = now
			over := len(b.timestamps) >= max
			if !over {
				b.timestamps = append(b.timestamps, now)
			}
			mu.Unlock()

			if over {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate_limit_exceeded","message":"Per-key ingest rate limit exceeded. Slow down."}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractAPIKeyID streams through the JSON body looking for the top-level
// "api_key_id" field. Returns "" if the body is not a JSON object, the
// field is absent, or the value is not a string.
func extractAPIKeyID(body []byte) string {
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return ""
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return ""
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return ""
		}
		key, ok := keyTok.(string)
		if !ok {
			return ""
		}
		if key == "api_key_id" {
			valTok, err := dec.Token()
			if err != nil {
				return ""
			}
			if s, ok := valTok.(string); ok {
				return s
			}
			return ""
		}
		// Skip the value associated with this key. Token() returns one
		// token per scalar; for objects/arrays we descend.
		if err := skipJSONValue(dec); err != nil {
			return ""
		}
	}
	return ""
}

func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); ok {
		switch d {
		case '{':
			for dec.More() {
				if _, err := dec.Token(); err != nil { // key
					return err
				}
				if err := skipJSONValue(dec); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil { // closing }
				return err
			}
		case '[':
			for dec.More() {
				if err := skipJSONValue(dec); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil { // closing ]
				return err
			}
		}
	}
	return nil
}
