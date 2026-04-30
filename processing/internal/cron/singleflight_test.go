package cron

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSingleflight_SkipsConcurrent verifies that while one tick is running, a
// concurrent call to Singleflight is skipped — exactly one body executes.
func TestSingleflight_SkipsConcurrent(t *testing.T) {
	var busy atomic.Bool
	var ran atomic.Int32

	start := make(chan struct{})
	hold := make(chan struct{})
	done := make(chan struct{})

	go func() {
		Singleflight("test", &busy, func() {
			ran.Add(1)
			close(start)
			<-hold // hold the lock until the test releases it
		})
		close(done)
	}()

	<-start // ensure the first goroutine is inside fn

	// Second concurrent call must be skipped.
	Singleflight("test", &busy, func() {
		ran.Add(1)
	})

	if got := ran.Load(); got != 1 {
		t.Fatalf("expected exactly 1 fn execution while busy, got %d", got)
	}

	close(hold)
	<-done

	// After the first tick releases, a third call must succeed.
	Singleflight("test", &busy, func() {
		ran.Add(1)
	})
	if got := ran.Load(); got != 2 {
		t.Fatalf("expected fn to run after busy released, got %d total runs", got)
	}
}

// TestSingleflight_SequentialRunsAllExecute verifies that strictly sequential
// (non-overlapping) calls all execute fn.
func TestSingleflight_SequentialRunsAllExecute(t *testing.T) {
	var busy atomic.Bool
	var ran atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		Singleflight("test", &busy, func() {
			ran.Add(1)
			wg.Done()
		})
	}
	// All five calls were strictly sequential (Singleflight is not async),
	// so no skips are expected.
	wg.Wait()
	if got := ran.Load(); got != 5 {
		t.Fatalf("expected 5 sequential runs, got %d", got)
	}
}

// TestSingleflight_BusyResetsAfterPanic ensures that even if fn panics, busy
// is reset so subsequent ticks are not permanently blocked.
func TestSingleflight_BusyResetsAfterPanic(t *testing.T) {
	var busy atomic.Bool

	defer func() {
		// The first call panics, recover from it.
		if r := recover(); r == nil {
			t.Fatalf("expected panic to propagate")
		}
		// busy must be reset so a follow-up call runs.
		var ran atomic.Bool
		Singleflight("test", &busy, func() { ran.Store(true) })
		if !ran.Load() {
			t.Fatalf("expected fn to run after panic-induced reset")
		}
	}()

	// Run inside a small wrapper so the deferred reset fires before our test
	// recovers the panic.
	func() {
		Singleflight("test", &busy, func() {
			panic("boom")
		})
	}()

	_ = time.Now() // unreachable
}
