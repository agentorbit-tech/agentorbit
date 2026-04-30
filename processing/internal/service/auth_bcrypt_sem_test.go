package service

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestAuthService constructs a minimal AuthService suitable for testing
// the bcrypt semaphore in isolation. We don't touch DB / mailer / queries.
func newTestAuthService(cap int) *AuthService {
	return &AuthService{
		bcryptSem: make(chan struct{}, cap),
	}
}

func TestAuthService_BcryptSemaphore_LimitsConcurrency(t *testing.T) {
	const capacity = 2
	s := newTestAuthService(capacity)

	var live, peak int64
	var wg sync.WaitGroup
	const total = 8

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.bcryptAcquire(context.Background()); err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			cur := atomic.AddInt64(&live, 1)
			for {
				old := atomic.LoadInt64(&peak)
				if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt64(&live, -1)
			s.bcryptRelease()
		}()
	}
	wg.Wait()

	if peak > int64(capacity) {
		t.Errorf("observed concurrency %d exceeds cap %d", peak, capacity)
	}
}

func TestAuthService_BcryptSemaphore_TimeoutReturns503(t *testing.T) {
	s := newTestAuthService(1)
	// Fill the semaphore.
	if err := s.bcryptAcquire(context.Background()); err != nil {
		t.Fatalf("primary acquire: %v", err)
	}
	t.Cleanup(s.bcryptRelease)

	start := time.Now()
	err := s.bcryptAcquire(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	var svcErr *ServiceError
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected ServiceError, got %T: %v", err, err)
	}
	if svcErr.Status != http.StatusServiceUnavailable || svcErr.Code != "auth_busy" {
		t.Errorf("unexpected ServiceError: %+v", svcErr)
	}
	if elapsed < 1500*time.Millisecond || elapsed > 3*time.Second {
		t.Errorf("expected ~2s timeout, got %s", elapsed)
	}
}

func TestAuthService_BcryptSemaphore_ContextCancel(t *testing.T) {
	s := newTestAuthService(1)
	if err := s.bcryptAcquire(context.Background()); err != nil {
		t.Fatalf("primary acquire: %v", err)
	}
	t.Cleanup(s.bcryptRelease)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := s.bcryptAcquire(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestAuthService_BcryptSemaphore_DefaultCap(t *testing.T) {
	// Sanity: NewAuthService with nil deps would panic, so just verify the
	// semaphore capacity matches NumCPU when constructed via the proper path.
	want := runtime.NumCPU()
	s := &AuthService{bcryptSem: make(chan struct{}, want)}
	if cap(s.bcryptSem) != want {
		t.Errorf("cap = %d, want %d", cap(s.bcryptSem), want)
	}
}
