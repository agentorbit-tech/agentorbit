package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentorbit-tech/agentorbit/proxy/internal/auth"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/masking"
)

// TestProxy_SSE_LargeSingleLine verifies that a single SSE data line larger than
// the legacy bufio.Scanner 1MiB cap is forwarded byte-for-byte to the agent.
//
// Plan: SP-1 Task 12 — replace bufio.Scanner SSE forwarder with byte-streaming.
// The 1MiB scanner cap silently truncated SSE lines that exceeded it (large
// tool_use payloads, base64 PDFs, multimodal output), violating the proxy's
// transparency contract.
func TestProxy_SSE_LargeSingleLine(t *testing.T) {
	const payloadSize = 5 * 1024 * 1024 // 5 MiB — well past the 1 MiB scanner cap
	bigJSON := `{"x":"` + strings.Repeat("A", payloadSize) + `"}`

	provider := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: "+bigJSON+"\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	})

	h, _, _ := setupProxyTest(t, provider, &auth.AuthVerifyResult{
		Valid:          true,
		APIKeyID:       "key-1",
		OrganizationID: "org-1",
		ProviderType:   "openai",
		ProviderKey:    auth.NewSecret("sk-123"),
	}, 0)

	// Use a real HTTP server so the SSE flush path is exercised.
	srv := httptest.NewServer(http.HandlerFunc(h.ServeHTTP))
	t.Cleanup(srv.Close)

	client := srv.Client()
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Authorization", "Bearer ao-abcdef1234567890abcdef1234567890")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	if !bytes.Contains(body, []byte(bigJSON)) {
		t.Fatalf("output missing %d-byte payload (got body len=%d)", len(bigJSON), len(body))
	}
	if !bytes.Contains(body, []byte("[DONE]")) {
		t.Fatalf("output missing [DONE] terminator (body len=%d)", len(body))
	}

	// Body should contain the entire bigJSON intact, plus framing.
	wantMin := len(bigJSON) + len("data: \n\n") + len("data: [DONE]\n\n")
	if len(body) < wantMin {
		t.Fatalf("body truncated: got %d bytes, want at least %d", len(body), wantMin)
	}
}

// TestProxy_SSE_AppendRing covers the three branches of the appendRing helper:
//   - next >= cap: result is the last cap bytes of next
//   - combined > cap: result is the last cap bytes of (existing ++ next)
//   - combined <= cap: result is existing ++ next (unchanged)
func TestProxy_SSE_AppendRing(t *testing.T) {
	t.Run("next_larger_than_cap", func(t *testing.T) {
		existing := []byte("EXISTING")
		next := bytes.Repeat([]byte("X"), 100)
		got := appendRing(existing, next, 32)
		if len(got) != 32 {
			t.Fatalf("len(got) = %d, want 32", len(got))
		}
		// Should be the last 32 bytes of next, ignoring existing.
		want := next[len(next)-32:]
		if !bytes.Equal(got, want) {
			t.Fatalf("got = %q, want %q", got, want)
		}
	})

	t.Run("combined_larger_than_cap", func(t *testing.T) {
		existing := []byte("AAAAAAAA") // 8 bytes
		next := []byte("BBBBBBBBBB")  // 10 bytes
		// combined = 18, cap = 12 → keep last 12 bytes of combined.
		got := appendRing(existing, next, 12)
		if len(got) != 12 {
			t.Fatalf("len(got) = %d, want 12", len(got))
		}
		want := []byte("AAAAAABBBBBB")
		// combined = "AAAAAAAA" + "BBBBBBBBBB" = "AAAAAAAABBBBBBBBBB" (18); last 12 = "AABBBBBBBBBB"
		want = []byte("AABBBBBBBBBB")
		if !bytes.Equal(got, want) {
			t.Fatalf("got = %q, want %q", got, want)
		}
	})

	t.Run("combined_within_cap", func(t *testing.T) {
		existing := []byte("AAA")
		next := []byte("BBB")
		got := appendRing(existing, next, 100)
		want := []byte("AAABBB")
		if !bytes.Equal(got, want) {
			t.Fatalf("got = %q, want %q", got, want)
		}
	})

	t.Run("empty_existing", func(t *testing.T) {
		got := appendRing(nil, []byte("hello"), 100)
		want := []byte("hello")
		if !bytes.Equal(got, want) {
			t.Fatalf("got = %q, want %q", got, want)
		}
	})
}

// TestProxy_SSE_LastJSONDataLineFromTail covers the helper that walks the tail
// ring buffer backwards and extracts the JSON payload of the last "data: {...}"
// line, skipping "data: [DONE]" and other non-JSON event lines.
func TestProxy_SSE_LastJSONDataLineFromTail(t *testing.T) {
	t.Run("extracts_last_json_skipping_done", func(t *testing.T) {
		tail := []byte("data: {\"a\":1}\n\ndata: {\"a\":2}\n\ndata: [DONE]\n\n")
		got := lastJSONDataLineFromTail(tail)
		want := `{"a":2}`
		if got != want {
			t.Fatalf("got = %q, want %q", got, want)
		}
	})

	t.Run("returns_empty_when_only_done", func(t *testing.T) {
		tail := []byte("data: [DONE]\n\n")
		got := lastJSONDataLineFromTail(tail)
		if got != "" {
			t.Fatalf("got = %q, want empty", got)
		}
	})

	t.Run("returns_empty_for_no_data_lines", func(t *testing.T) {
		tail := []byte("event: ping\n\n: keepalive\n\n")
		got := lastJSONDataLineFromTail(tail)
		if got != "" {
			t.Fatalf("got = %q, want empty", got)
		}
	})

	t.Run("extracts_single_json_line", func(t *testing.T) {
		tail := []byte("data: {\"model\":\"gpt-4\",\"usage\":{\"prompt_tokens\":10}}\n\n")
		got := lastJSONDataLineFromTail(tail)
		want := `{"model":"gpt-4","usage":{"prompt_tokens":10}}`
		if got != want {
			t.Fatalf("got = %q, want %q", got, want)
		}
	})

	t.Run("empty_tail", func(t *testing.T) {
		got := lastJSONDataLineFromTail(nil)
		if got != "" {
			t.Fatalf("got = %q, want empty", got)
		}
	})
}

// TestUnmaskStreaming_CrossBoundary verifies that a masked placeholder which
// straddles a chunk boundary is fully unmasked. Pre-fix the streamer sliced
// the placeholder at the safeEnd boundary — its first byte was unmasked in
// isolation, leaving "[PHONE_1]" leaked literally to the agent.
func TestUnmaskStreaming_CrossBoundary(t *testing.T) {
	original := "+71234567890"
	masked := "[PHONE_1]"
	entries := []masking.MaskEntry{{
		MaskType: masking.MaskType("phone"),
		Original: original,
		Masked:   masked,
	}}

	// Build an input where the placeholder starts at offset chunkSize-1, so
	// the first byte of "[PHONE_1]" lands inside the unmask region of chunk #1
	// while the rest must be carried over to chunk #2.
	const chunkSize = 64 // small chunk for deterministic boundary
	prefix := strings.Repeat("A", chunkSize-1)
	suffix := strings.Repeat("Z", 32)
	input := prefix + masked + suffix

	var out bytes.Buffer
	emitted, err := unmaskStreaming(&out, strings.NewReader(input), entries, chunkSize)
	if err != nil {
		t.Fatalf("unmaskStreaming err: %v", err)
	}

	got := out.String()
	want := prefix + original + suffix
	if got != want {
		t.Fatalf("cross-boundary unmask leaked placeholder.\n got = %q\nwant = %q", got, want)
	}
	if strings.Contains(got, masked) {
		t.Fatalf("output still contains masked placeholder %q", masked)
	}
	if emitted != len(want) {
		t.Errorf("emitted = %d, want %d", emitted, len(want))
	}
}

// TestUnmaskStreaming_NoEntries is a passthrough sanity check.
func TestUnmaskStreaming_NoEntries(t *testing.T) {
	input := "the quick brown fox jumps over the lazy dog"
	var out bytes.Buffer
	if _, err := unmaskStreaming(&out, strings.NewReader(input), nil, 16); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.String() != input {
		t.Fatalf("got = %q, want %q", out.String(), input)
	}
}

// TestUnmaskStreaming_PlaceholderAtEOF covers the end-of-stream branch where
// the lookahead window holds a complete placeholder until EOF.
func TestUnmaskStreaming_PlaceholderAtEOF(t *testing.T) {
	original := "+71234567890"
	masked := "[PHONE_1]"
	entries := []masking.MaskEntry{{Original: original, Masked: masked}}

	input := strings.Repeat("X", 100) + masked
	var out bytes.Buffer
	if _, err := unmaskStreaming(&out, strings.NewReader(input), entries, 32); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := strings.Repeat("X", 100) + original
	if out.String() != want {
		t.Fatalf("got = %q, want %q", out.String(), want)
	}
}
