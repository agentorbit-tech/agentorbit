package span

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agentorbit-tech/agentorbit/proxy/internal/errtrack"
)

// MaskingMapEntry records one original-to-masked replacement for span storage.
// Only populated for LLM Only mode (D-10).
type MaskingMapEntry struct {
	MaskType      string `json:"mask_type"`
	OriginalValue string `json:"original_value"`
	MaskedValue   string `json:"masked_value"`
}

// SpanPayload is the data sent to Processing for span ingestion.
// Fields match SpanIngestRequest on the Processing side.
type SpanPayload struct {
	APIKeyID           string            `json:"api_key_id"`
	OrganizationID     string            `json:"organization_id"`
	ProviderType       string            `json:"provider_type"`
	Model              string            `json:"model"`
	Input              string            `json:"input"`
	Output             string            `json:"output"`
	InputTokens        int               `json:"input_tokens"`
	OutputTokens       int               `json:"output_tokens"`
	DurationMs         int64             `json:"duration_ms"`
	HTTPStatus         int               `json:"http_status"`
	StartedAt          string            `json:"started_at"`
	FinishReason       string            `json:"finish_reason,omitempty"`
	ExternalSessionID  string            `json:"external_session_id,omitempty"`
	AgentName          string            `json:"agent_name,omitempty"`
	MaskingApplied     bool              `json:"masking_applied"`
	MaskingMap         []MaskingMapEntry `json:"masking_map,omitempty"`
	ClientDisconnected bool              `json:"client_disconnected"`
}

// SpanDispatcher sends span payloads to Processing asynchronously via a buffered channel.
// When the channel is full, payloads are dropped silently (fail-open).
//
// Shutdown sequence:
//  1. Caller invokes Drain(deadline). This flips acceptingNew to false,
//     waits for in-flight Dispatch() callers to release the channel write,
//     closes the channel, and lets workers consume remaining items until
//     the channel is empty or the deadline expires.
//  2. After Drain returns, Dispatch() calls drop silently and increment dropped.
type SpanDispatcher struct {
	ch            chan SpanPayload
	processingURL string
	internalToken string
	client        *http.Client
	dropped       atomic.Int64
	sendTimeout   time.Duration
	drainTimeout  time.Duration
	numWorkers    int

	acceptingNew atomic.Bool
	dispatchWG   sync.WaitGroup
	workerCtx    context.Context
	workerCancel context.CancelFunc
	workersDone  chan struct{}
}

// NewSpanDispatcher creates a new SpanDispatcher with the given buffer size and send timeout.
// drainTimeout controls how long to wait for buffered spans on shutdown (0 defaults to 5s).
// numWorkers controls how many concurrent worker goroutines send spans (0 defaults to 3).
func NewSpanDispatcher(processingURL, internalToken string, bufferSize int, client *http.Client, sendTimeout time.Duration, drainTimeout time.Duration, numWorkers int) *SpanDispatcher {
	if drainTimeout <= 0 {
		drainTimeout = 5 * time.Second
	}
	if numWorkers <= 0 {
		numWorkers = 3
	}
	d := &SpanDispatcher{
		ch:            make(chan SpanPayload, bufferSize),
		processingURL: processingURL,
		internalToken: internalToken,
		client:        client,
		sendTimeout:   sendTimeout,
		drainTimeout:  drainTimeout,
		numWorkers:    numWorkers,
		workersDone:   make(chan struct{}),
	}
	d.acceptingNew.Store(true)
	// Initialize workerCtx here so Drain/Start ordering races cannot occur and
	// the cancel function is always non-nil. Start may overwrite it with a
	// caller-derived context for cooperative shutdown.
	d.workerCtx, d.workerCancel = context.WithCancel(context.Background())
	return d
}

// Dispatch enqueues a span payload for async delivery. Non-blocking: if the
// channel is full, the payload is dropped and the dropped counter increments.
// After Drain has been called, Dispatch silently drops every payload.
func (d *SpanDispatcher) Dispatch(payload SpanPayload) {
	if !d.acceptingNew.Load() {
		d.dropped.Add(1)
		return
	}
	d.dispatchWG.Add(1)
	defer d.dispatchWG.Done()
	// Re-check after registering — Drain may have flipped the flag and is
	// waiting for the WaitGroup; in that case do not write to the channel.
	if !d.acceptingNew.Load() {
		d.dropped.Add(1)
		return
	}
	select {
	case d.ch <- payload:
		// enqueued
	default:
		count := d.dropped.Add(1)
		if count == 1 || count%100 == 0 {
			slog.Warn("span buffer full", "dropped_total", count)
		}
	}
}

// Dropped returns the total number of payloads dropped due to a full channel
// or post-Drain refusal.
func (d *SpanDispatcher) Dropped() int64 {
	return d.dropped.Load()
}

// DrainOne reads one payload from the channel without blocking.
// Returns the payload and true if one was available, or zero value and false otherwise.
// Intended for use in tests to inspect dispatched spans.
func (d *SpanDispatcher) DrainOne() (SpanPayload, bool) {
	select {
	case p := <-d.ch:
		return p, true
	default:
		return SpanPayload{}, false
	}
}

// Start launches numWorkers goroutines that read from the channel and send
// payloads to Processing. The ctx parameter is the parent for the internal
// workerCtx — when ctx is canceled, in-flight HTTP sends abort and workers
// exit. The primary stop signal remains Drain (which closes the channel),
// but honoring ctx provides a defensive backstop and bounds shutdown latency.
func (d *SpanDispatcher) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	d.workerCtx, d.workerCancel = context.WithCancel(ctx)
	var workersWG sync.WaitGroup
	for i := 0; i < d.numWorkers; i++ {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			for {
				select {
				case payload, ok := <-d.ch:
					if !ok {
						return
					}
					d.send(d.workerCtx, payload)
				case <-d.workerCtx.Done():
					return
				}
			}
		}()
	}
	go func() {
		workersWG.Wait()
		close(d.workersDone)
	}()
}

// Drain stops accepting new payloads, waits for in-flight Dispatch() callers
// to finish writing, closes the channel, and lets workers consume remaining
// items until the channel is empty or deadline expires. Idempotent — repeat
// calls are no-ops.
func (d *SpanDispatcher) Drain(deadline time.Duration) {
	if !d.acceptingNew.CompareAndSwap(true, false) {
		return
	}
	d.dispatchWG.Wait()
	close(d.ch)
	if deadline <= 0 {
		deadline = d.drainTimeout
	}
	select {
	case <-d.workersDone:
	case <-time.After(deadline):
		if remaining := len(d.ch); remaining > 0 {
			slog.Warn("span dispatcher drain timed out", "lost", remaining)
		}
		if d.workerCancel != nil {
			d.workerCancel()
		}
	}
}

// send POSTs a span payload to Processing's /internal/spans/ingest endpoint.
// Errors are logged but do not affect proxy operation (fail-open). Cancelling
// parent ctx (e.g. from Drain's deadline branch) aborts in-flight requests so
// shutdown latency is bounded by min(sendTimeout, drain deadline).
func (d *SpanDispatcher) send(parent context.Context, payload SpanPayload) {
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("span dispatch marshal failed", "error", err)
		errtrack.Capture(err, errtrack.Fields{"component": "span_dispatch", "stage": "marshal"})
		return
	}

	sendCtx, cancel := context.WithTimeout(parent, d.sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(sendCtx, http.MethodPost, strings.TrimRight(d.processingURL, "/")+"/internal/spans/ingest", bytes.NewReader(body))
	if err != nil {
		slog.Error("span dispatch request creation failed", "error", err)
		errtrack.Capture(err, errtrack.Fields{"component": "span_dispatch", "stage": "request"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", d.internalToken)

	resp, err := d.client.Do(req)
	if err != nil {
		slog.Error("span dispatch send failed", "error", err)
		errtrack.Capture(err, errtrack.Fields{"component": "span_dispatch", "stage": "send"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Warn("span dispatch unexpected status", "status", resp.StatusCode)
	}
}
