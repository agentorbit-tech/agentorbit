// Package cron contains helpers for scheduling background workers safely:
//
//   - Singleflight prevents two ticks of the same worker from running
//     concurrently inside a single process.
//   - WithAdvisoryLock prevents two replicas from running the same worker at
//     the same time across the database.
//
// Workers should compose them with Singleflight on the outside (cheap, in-
// process) and WithAdvisoryLock on the inside (cross-replica).
package cron

import (
	"log/slog"
	"sync/atomic"
)

// Singleflight runs fn only when busy is currently false; otherwise it logs at
// warn-level and returns immediately. It guarantees fn finishes before the
// next call observes a free slot. The boolean is owned by the caller so each
// worker keeps its own atomic without sharing state.
func Singleflight(name string, busy *atomic.Bool, fn func()) {
	if !busy.CompareAndSwap(false, true) {
		slog.Warn("cron skipped, previous tick still running", "cron", name)
		return
	}
	defer busy.Store(false)
	fn()
}
