package uploadscan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const chunkSize = 1 << 20 // 1 MiB

// The largest body read and scanned (larger ones pass unscanned), and how many bodies and clamd
// sessions one Traefik holds at once - together its memory for the scan. Code, not settings:
// they size memory once per start, and the plugin's source hash is what restarts Traefik.
var (
	maxBytes    int64 = 100 << 20 // 100 MiB: << 20 multiplies by 2^20, a MiB
	maxParallel       = 4
)

// Shared by every instance in this Traefik (Traefik creates one per router): the memory held for
// bodies, counted in chunks - maxParallel bodies of maxBytes, or half as many chunked ones, which
// need twice their size while they are joined - and one token per open clamd session.
var (
	limitsOnce sync.Once
	pool       *memoryPool
	sessions   chan struct{}
)

func initLimits() {
	limitsOnce.Do(func() {
		pool = &memoryPool{size: maxParallel * (chunksFor(maxBytes) + 1), freed: make(chan struct{})}
		sessions = make(chan struct{}, maxParallel)
	})
}

func chunksFor(size int64) int {
	return int((size + chunkSize - 1) / chunkSize)
}

// memoryPool hands out chunks all or nothing: a request either gets its whole share or holds
// none while it waits, so no waiter keeps part of the pool from others - a small upload passes
// whenever its share is free.
type memoryPool struct {
	mu    sync.Mutex
	size  int
	used  int
	freed chan struct{} // closed and replaced whenever chunks come back
}

// tryTake takes n chunks if they are free now; else it returns the channel that is closed when
// some come back.
func (p *memoryPool) tryTake(n int) (bool, chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.used+n <= p.size {
		p.used += n
		return true, nil
	}
	return false, p.freed
}

func (p *memoryPool) give(n int) {
	if n <= 0 {
		return
	}
	p.mu.Lock()
	p.used -= n
	close(p.freed)
	p.freed = make(chan struct{})
	p.mu.Unlock()
}

func (p *memoryPool) inUse() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.used
}

// queue is the time a request may still spend waiting; it gives up at once when the client is
// gone.
type queue struct {
	left time.Duration
	ctx  context.Context
}

// wait blocks until ready is closed, the queue's time is up or the client is gone; it reports
// whether ready came.
func (q *queue) wait(ready chan struct{}) bool {
	start := time.Now()
	timer := time.NewTimer(q.left)
	defer timer.Stop()
	defer func() { q.left -= time.Since(start) }()
	select {
	case <-ready:
		return true
	case <-timer.C:
		return false
	case <-q.ctx.Done():
		return false
	}
}

// reserve takes n chunks of the pool, all or none, within the queue's time.
func (q *queue) reserve(n int) bool {
	if n > pool.size {
		return false
	}
	for {
		ok, freed := pool.tryTake(n)
		if ok {
			return true
		}
		if q.left <= 0 || !q.wait(freed) {
			return false
		}
	}
}

// session takes one clamd session within the queue's time.
func (q *queue) session() bool {
	// A free session first: with the queue's time used up, select below could pick the timer
	// although a session is free.
	select {
	case sessions <- struct{}{}:
		return true
	default:
	}
	start := time.Now()
	timer := time.NewTimer(q.left)
	defer timer.Stop()
	defer func() { q.left -= time.Since(start) }()
	select {
	case sessions <- struct{}{}:
		return true
	case <-timer.C:
		return false
	case <-q.ctx.Done():
		return false
	}
}

// heldBody hands the app the body as it came: the bytes read for the scan, then whatever was not
// read. Their memory goes back once the proxy has read past them, or when the request is over.
// The proxy may read it from another goroutine than the one that ends the request.
type heldBody struct {
	original io.ReadCloser
	mu       sync.Mutex
	data     []byte
	tokens   int
}

func (h *heldBody) Read(p []byte) (int, error) {
	h.mu.Lock()
	if len(h.data) == 0 {
		h.data = nil
		pool.give(h.tokens)
		h.tokens = 0
		h.mu.Unlock()
		return h.original.Read(p)
	}
	n := copy(p, h.data)
	h.data = h.data[n:]
	h.mu.Unlock()
	return n, nil
}

func (h *heldBody) Close() error {
	h.release()
	return h.original.Close()
}

func (h *heldBody) setTokens(n int) {
	h.mu.Lock()
	h.tokens = n
	h.mu.Unlock()
}

// hold keeps data for the app and gives back the chunks reserved beyond it. A chunked body keeps
// twice its size: the pieces it was joined from stay in memory until the garbage collector frees
// them.
func (h *heldBody) hold(data []byte, chunked bool) {
	h.mu.Lock()
	keep := chunksFor(int64(len(data)))
	if chunked {
		keep *= 2
	}
	if keep > h.tokens {
		keep = h.tokens
	}
	pool.give(h.tokens - keep)
	h.tokens = keep
	h.data = data
	h.mu.Unlock()
}

func (h *heldBody) release() {
	if h == nil {
		return
	}
	h.mu.Lock()
	pool.give(h.tokens)
	h.tokens = 0
	h.data = nil
	h.mu.Unlock()
}

// read is the outcome of reading a body for the scan: one contiguous buffer, which the
// multipart cutting needs.
type read struct {
	data    []byte
	size    int64
	verdict string
	err     error
}

// readBody reads the body, at most maxBytes+1, with its memory already reserved. A known length is
// read straight into one buffer of that size; an unknown one in chunks, joined once at the end.
// Only io.EOF is the body's end: a client that breaks off a chunked body gives ErrUnexpectedEOF.
func readBody(r io.Reader, known, maxBytes int64) read {
	var out read
	if known > 0 {
		out.data = make([]byte, known)
		n, err := fill(r, out.data)
		out.data = out.data[:n]
		out.size = int64(n)
		if err != nil && !errors.Is(err, io.EOF) {
			out.verdict = "aborted"
			out.err = err
		} else if out.size < known {
			out.verdict = "aborted"
			out.err = fmt.Errorf("body ended after %d of %d bytes", out.size, known)
		}
		return out
	}
	var chunks [][]byte
	for {
		chunk := make([]byte, chunkSize)
		n, err := fill(r, chunk)
		if n > 0 {
			chunks = append(chunks, chunk[:n])
			out.size += int64(n)
		}
		if out.size > maxBytes {
			out.verdict = "skipped:too-large"
			break
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			out.verdict = "aborted"
			out.err = err
			break
		}
	}
	out.data = bytes.Join(chunks, nil)
	return out
}

// fill reads until buf is full or the reader ends, passing on the reader's own error.
func fill(r io.Reader, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		m, err := r.Read(buf[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
