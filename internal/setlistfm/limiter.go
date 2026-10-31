package setlistfm

import (
	"context"
	"sync"
	"time"
)

// limiter spaces calls at least interval apart, across all requests in the
// process. It's enough because Cloud Run runs at most one instance
// (ADR-003, ADR-005).
type limiter struct {
	interval time.Duration

	mu   sync.Mutex
	next time.Time // earliest time the next call may start
}

func newLimiter(interval time.Duration) *limiter { return &limiter{interval: interval} }

// wait reserves the next free slot and sleeps until it, or returns ctx's
// error if ctx ends first. A cancelled wait gives its slot back if no later
// call has reserved one since; otherwise the slot stays used, which only
// errs on the safe side.
func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	slot := l.next
	if slot.Before(now) {
		slot = now
	}
	l.next = slot.Add(l.interval)
	l.mu.Unlock()

	d := time.Until(slot)
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		l.mu.Lock()
		if l.next.Equal(slot.Add(l.interval)) {
			l.next = slot
		}
		l.mu.Unlock()
		return ctx.Err()
	}
}
