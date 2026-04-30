package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeUpdater struct {
	calls atomic.Int32
	ids   chan []uuid.UUID
}

func (f *fakeUpdater) UpdateBatch(ctx context.Context, ids []uuid.UUID, ts time.Time) error {
	f.calls.Add(1)
	select {
	case f.ids <- ids:
	default:
	}
	return nil
}

func TestLastUsedBatcher_Batches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upd := &fakeUpdater{ids: make(chan []uuid.UUID, 10)}
	b := NewLastUsedBatcher(ctx, upd, 10*time.Millisecond, 256)

	id := uuid.New()
	for i := 0; i < 1000; i++ {
		b.Touch(id)
	}
	time.Sleep(50 * time.Millisecond)

	if got := upd.calls.Load(); got > 5 {
		t.Errorf("expected <=5 batched calls, got %d", got)
	}
}

func TestLastUsedBatcher_Dedupes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upd := &fakeUpdater{ids: make(chan []uuid.UUID, 10)}
	b := NewLastUsedBatcher(ctx, upd, 10*time.Millisecond, 256)

	id1, id2, id3 := uuid.New(), uuid.New(), uuid.New()
	b.Touch(id1)
	b.Touch(id2)
	b.Touch(id1)
	b.Touch(id3)
	b.Touch(id2)
	time.Sleep(30 * time.Millisecond)

	got := <-upd.ids
	if len(got) != 3 {
		t.Errorf("dedupe failed: got %d ids, want 3", len(got))
	}
}

func TestLastUsedBatcher_FlushOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	upd := &fakeUpdater{ids: make(chan []uuid.UUID, 10)}
	b := NewLastUsedBatcher(ctx, upd, 1*time.Hour, 256) // never auto-flush

	for i := 0; i < 5; i++ {
		b.Touch(uuid.New())
	}
	cancel()
	if err := b.WaitFlush(2 * time.Second); err != nil {
		t.Fatalf("WaitFlush: %v", err)
	}
	if upd.calls.Load() == 0 {
		t.Errorf("shutdown did not flush")
	}
}
