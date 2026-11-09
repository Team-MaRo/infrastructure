package uploadscan

import (
	"context"
	"time"
)

// queue is the time a request may still spend waiting; it gives up at once when the client is
// gone. capped: the last wait for memory ended with its address's share held.
type queue struct {
	left   time.Duration
	ctx    context.Context
	capped bool
}

// busy is the verdict of a body that got no memory in time: its address's share full, or the
// pool.
func (q *queue) busy() string {
	if q.capped {
		return "skipped:too-many"
	}
	return "skipped:busy"
}

// turn takes a slot of sem - a clamd slot, the join - within the queue's time.
func (q *queue) turn(sem chan struct{}) bool {
	// A free slot first: with the queue's time used up, select below could pick the timer
	// although a slot is free.
	select {
	case sem <- struct{}{}:
		return true
	default:
	}
	start := time.Now()
	timer := time.NewTimer(q.left)
	defer timer.Stop()
	defer func() { q.left -= time.Since(start) }()
	select {
	case sem <- struct{}{}:
		return true
	case <-timer.C:
		return false
	case <-q.ctx.Done():
		return false
	}
}
