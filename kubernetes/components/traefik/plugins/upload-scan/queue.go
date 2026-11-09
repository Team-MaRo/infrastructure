package uploadscan

import (
	"context"
	"time"
)

// queue is the time a request may still spend waiting; it gives up at once when the client is
// gone.
type queue struct {
	left time.Duration
	ctx  context.Context
}

// turn takes a slot of sem - a clamd session, the join - within the queue's time.
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
