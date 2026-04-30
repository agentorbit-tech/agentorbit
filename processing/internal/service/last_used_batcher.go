package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/google/uuid"
)

// BatchUpdater is the contract for the SQL writer; production wires it to the
// sqlc-generated UpdateApiKeysLastUsedBatch.
type BatchUpdater interface {
	UpdateBatch(ctx context.Context, ids []uuid.UUID, ts time.Time) error
}

const lastUsedBufferSize = 1024

// LastUsedBatcher coalesces api_keys.last_used_at writes. Each VerifyAPIKey call
// enqueues a key id via Touch (non-blocking); a single goroutine flushes the
// pending set every flushEvery or once it reaches maxBatch, collapsing N writes
// into one UPDATE ... WHERE id = ANY($1).
type LastUsedBatcher struct {
	in         chan uuid.UUID
	updater    BatchUpdater
	flushEvery time.Duration
	maxBatch   int
	doneFlush  chan struct{}
	dropped    atomic.Int64
}

// NewLastUsedBatcher starts the batching goroutine. It exits when ctx is done,
// flushing remaining IDs synchronously before signalling doneFlush.
func NewLastUsedBatcher(ctx context.Context, u BatchUpdater, flushEvery time.Duration, maxBatch int) *LastUsedBatcher {
	b := &LastUsedBatcher{
		in:         make(chan uuid.UUID, lastUsedBufferSize),
		updater:    u,
		flushEvery: flushEvery,
		maxBatch:   maxBatch,
		doneFlush:  make(chan struct{}),
	}
	go b.run(ctx)
	return b
}

// Touch enqueues an API key ID for last_used_at update. Non-blocking: drops if
// the buffer is full. Drops are counted via the dropped atomic; the run loop
// emits a single summary warning per flush window so sustained overflow is
// visible without flooding logs (AKEY-05 must never block the verify path).
func (b *LastUsedBatcher) Touch(id uuid.UUID) {
	select {
	case b.in <- id:
	default:
		b.dropped.Add(1)
	}
}

func (b *LastUsedBatcher) run(ctx context.Context) {
	defer close(b.doneFlush)
	pending := make(map[uuid.UUID]struct{}, b.maxBatch)
	ticker := time.NewTicker(b.flushEvery)
	defer ticker.Stop()

	flush := func() {
		if len(pending) == 0 {
			return
		}
		ids := make([]uuid.UUID, 0, len(pending))
		for id := range pending {
			ids = append(ids, id)
		}
		ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := b.updater.UpdateBatch(ctx2, ids, time.Now()); err != nil {
			slog.Warn("last_used_at batch update failed", "err", err, "n", len(ids))
		}
		if dropped := b.dropped.Swap(0); dropped > 0 {
			slog.Warn("last_used_at touches dropped due to full buffer", "dropped", dropped)
		}
		for k := range pending {
			delete(pending, k)
		}
	}

	for {
		select {
		case id := <-b.in:
			pending[id] = struct{}{}
			if len(pending) >= b.maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			// Drain remaining inbox so a Touch racing with cancel still lands.
			for {
				select {
				case id := <-b.in:
					pending[id] = struct{}{}
				default:
					flush()
					return
				}
			}
		}
	}
}

// WaitFlush blocks until run() returned (after final flush) or the deadline elapses.
func (b *LastUsedBatcher) WaitFlush(d time.Duration) error {
	select {
	case <-b.doneFlush:
		return nil
	case <-time.After(d):
		return errors.New("last_used_at flush deadline exceeded")
	}
}

// sqlcBatchUpdater adapts the sqlc-generated UpdateApiKeysLastUsedBatch query
// to the BatchUpdater interface.
type sqlcBatchUpdater struct {
	q *db.Queries
}

func newSQLCBatchUpdater(q *db.Queries) BatchUpdater { return &sqlcBatchUpdater{q: q} }

func (u *sqlcBatchUpdater) UpdateBatch(ctx context.Context, ids []uuid.UUID, ts time.Time) error {
	return u.q.UpdateApiKeysLastUsedBatch(ctx, db.UpdateApiKeysLastUsedBatchParams{
		Column1:    ids,
		LastUsedAt: sql.NullTime{Time: ts, Valid: true},
	})
}
