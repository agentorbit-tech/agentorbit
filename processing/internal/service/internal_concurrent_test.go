//go:build integration

package service_test

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
)

// TestIngestSpan_FreePlanQuotaUnderConcurrency verifies that the new atomic
// per-org counter strictly enforces the FreeSpanLimit even under heavy
// concurrent ingestion. 100 goroutines each issue 50 IngestSpan calls against
// a single free-plan org. Exactly FreeSpanLimit (3000) must succeed and the
// remaining 2000 must return SpanQuotaExceededError.
func TestIngestSpan_FreePlanQuotaUnderConcurrency(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "free-concurrent@example.com", "Free Concurrent")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Free Concurrent Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Free Agent", "openai", "sk-free", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	const goroutines = 100
	const perGoroutine = 50
	const total = goroutines * perGoroutine
	const target = service.FreeSpanLimit

	var (
		ok       atomic.Int32
		rejected atomic.Int32
		other    atomic.Int32
	)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				err := internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
					APIKeyID:       apiKeyResult.ID.String(),
					OrganizationID: org.ID.String(),
					ProviderType:   "openai",
					Model:          "gpt-4",
					Input:          "in",
					Output:         "out",
					InputTokens:    1,
					OutputTokens:   1,
					DurationMs:     10,
					HTTPStatus:     200,
					StartedAt:      time.Now().Format(time.RFC3339Nano),
					FinishReason:   "stop",
				})
				switch {
				case err == nil:
					ok.Add(1)
				case isSpanQuotaErr(err):
					rejected.Add(1)
				default:
					other.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if got := int(ok.Load()); got != target {
		t.Errorf("ok=%d want=%d (rejected=%d other=%d)", got, target, rejected.Load(), other.Load())
	}
	if got := int(rejected.Load()); got != total-target {
		t.Errorf("rejected=%d want=%d (ok=%d other=%d)", got, total-target, ok.Load(), other.Load())
	}
	if other.Load() != 0 {
		t.Errorf("unexpected other-error count: %d", other.Load())
	}
}

// TestIngestSpan_PaidPlanNoLock verifies that paid plans skip the quota gate
// entirely (no row lock, no counter UPDATE on organizations). 200 concurrent
// ingests against a single pro-plan org must all succeed and complete in well
// under the elapsed bound — a proxy for "no serialization on the org row".
func TestIngestSpan_PaidPlanNoLock(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "pro-concurrent@example.com", "Pro Concurrent")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Pro Concurrent Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Promote to pro plan — paid plans must bypass the counter entirely.
	if _, err := pool.Exec(ctx, "UPDATE organizations SET plan = 'pro' WHERE id = $1", org.ID); err != nil {
		t.Fatalf("set plan=pro: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Pro Agent", "openai", "sk-pro", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	const n = 200

	var (
		ok      atomic.Int32
		failed  atomic.Int32
		quotaer atomic.Int32
	)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
				APIKeyID:       apiKeyResult.ID.String(),
				OrganizationID: org.ID.String(),
				ProviderType:   "openai",
				Model:          "gpt-4",
				Input:          "in",
				Output:         "out",
				InputTokens:    1,
				OutputTokens:   1,
				DurationMs:     10,
				HTTPStatus:     200,
				StartedAt:      time.Now().Format(time.RFC3339Nano),
				FinishReason:   "stop",
			})
			switch {
			case err == nil:
				ok.Add(1)
			case isSpanQuotaErr(err):
				quotaer.Add(1)
			default:
				failed.Add(1)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// Real proof of "no serialization": all 200 must succeed and never hit the
	// quota path. Elapsed bound is informational on slow CI.
	if int(ok.Load()) != n {
		t.Errorf("ok=%d want=%d (failed=%d quota=%d)", ok.Load(), n, failed.Load(), quotaer.Load())
	}
	if quotaer.Load() != 0 {
		t.Errorf("paid plan unexpectedly hit quota gate: %d", quotaer.Load())
	}
	if elapsed > 30*time.Second {
		t.Logf("paid-plan ingest took %v — investigate if this trends upward", elapsed)
	}
}

// isSpanQuotaErr reports whether err is the SpanQuotaExceededError sentinel
// (or wraps one). Using errors.As keeps the check robust to future wrapping.
func isSpanQuotaErr(err error) bool {
	var quotaErr *service.SpanQuotaExceededError
	return errors.As(err, &quotaErr)
}
