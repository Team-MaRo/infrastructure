package uploadscan

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"sync"
)

const chunkSize = 1 << 20 // 1 MiB

const firstChunkSize = 64 << 10

// From this size on, a join hands its chunks' memory back to the system at once.
const bigJoin = 16 << 20

// The largest body read and scanned (larger ones pass unscanned), and how many such bodies and
// clamd sessions one Traefik holds at once - together its memory for the scan. Code, not
// settings: they size memory once per start, and the plugin's source hash is what restarts
// Traefik.
var (
	maxBytes    int64 = 100 << 20 // 100 MiB: << 20 multiplies by 2^20, a MiB
	maxParallel       = 4
)

// Shared by every instance in this Traefik (Traefik creates one per router): the memory for the
// bodies being read and held, counted in chunks - room for maxParallel bodies of maxBytes, shared
// by however many bodies there are - and one token per open clamd session.
var (
	limitsOnce sync.Once
	pool       *memoryPool
	sessions   chan struct{}
	// One body at a time is copied into one buffer: while it copies, its chunks and the copy
	// exist together.
	joining = make(chan struct{}, 1)
)

func initLimits() {
	limitsOnce.Do(func() {
		pool = &memoryPool{size: maxParallel * (chunksFor(maxBytes) + 1)}
		sessions = make(chan struct{}, maxParallel)
	})
}

func chunksFor(size int64) int {
	return int((size + chunkSize - 1) / chunkSize)
}

// heldBody hands the app the body as it came: the chunks read for the scan, then whatever was
// not read. A chunk's memory goes back once the proxy has read past it, the rest when the request
// is over. The proxy may read it from another goroutine than the one that ends the request, also
// after closing it; once the request is over it gets an error, as from Go's own request bodies.
type heldBody struct {
	original io.ReadCloser
	pool     *memoryPool
	mu       sync.Mutex
	held     chunkReader
	passed   int // chunks the proxy has read past
	tokens   int
	released bool
}

func (h *heldBody) Read(p []byte) (int, error) {
	h.mu.Lock()
	if h.released {
		h.mu.Unlock()
		return 0, http.ErrBodyReadAfterClose
	}
	n, err := h.held.Read(p)
	for h.passed < h.held.next {
		// The scan is done before the proxy reads: a chunk it has passed on is not needed again.
		h.held.chunks[h.passed] = nil
		h.passed++
		if h.passed < len(h.held.chunks) && h.tokens > 1 {
			h.pool.give(1)
			h.tokens--
		}
	}
	if errors.Is(err, io.EOF) {
		h.reset()
		h.mu.Unlock()
		return h.original.Read(p)
	}
	h.mu.Unlock()
	return n, err
}

// reset gives back the memory still held and forgets the chunks; h.mu is held.
func (h *heldBody) reset() {
	h.pool.give(h.tokens)
	h.tokens = 0
	h.held = chunkReader{}
	h.passed = 0
}

func (h *heldBody) Close() error {
	h.release()
	return h.original.Close()
}

func (h *heldBody) took() {
	h.mu.Lock()
	h.tokens++
	h.mu.Unlock()
}

func (h *heldBody) hold(chunks [][]byte) {
	h.mu.Lock()
	h.held = chunkReader{chunks: chunks}
	h.passed = 0
	h.mu.Unlock()
}

func (h *heldBody) release() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.reset()
	h.released = true
	h.mu.Unlock()
}

// read is the outcome of reading a body for the scan: its chunks, as they came.
type read struct {
	chunks  [][]byte
	size    int64
	verdict string
	err     error
}

func (o *read) add(b []byte) {
	if len(b) > 0 {
		o.chunks = append(o.chunks, b)
		o.size += int64(len(b))
	}
}

// readBody reads the body for the scan - one of unknown length up to a chunk beyond maxBytes, to
// tell that it is too large - in chunks of up to 1 MiB, taking a chunk of the pool for each as it
// is allocated - so a body holds memory only for what it has sent, and none before its first
// byte: an upload that only connects holds nothing. It is admitted first (memoryPool); a body that
// gets no chunk in time stops being read for the scan (skipped:busy), and the app gets what was
// read followed by the rest. Only io.EOF is the body's end: a client that breaks off a chunked
// body gives ErrUnexpectedEOF.
func readBody(r io.Reader, known int64, q *queue, held *heldBody) read {
	var probe [1]byte
	n, err := fill(r, probe[:])
	if n == 0 {
		var none read
		switch {
		case errors.Is(err, io.EOF) && known > 0:
			none.verdict = "aborted"
			none.err = fmt.Errorf("body ended after 0 of %d bytes", known)
		case err != nil && !errors.Is(err, io.EOF):
			none.verdict = "aborted"
			none.err = err
		}
		return none
	}
	first := []byte{probe[0]}
	if err != nil {
		// A one-byte body, or a byte before an error: no memory needed for it.
		got := read{chunks: [][]byte{first}, size: 1}
		switch {
		case !errors.Is(err, io.EOF):
			got.verdict = "aborted"
			got.err = err
		case known > 1:
			got.verdict = "aborted"
			got.err = fmt.Errorf("body ended after 1 of %d bytes", known)
		}
		return got
	}

	need := 1
	if known > 0 {
		need = chunksFor(known)
	}
	in := held.pool.admit(q, need)
	if in == nil {
		return read{chunks: [][]byte{first}, size: 1, verdict: "skipped:busy"}
	}
	held.took()
	defer held.pool.done(in)
	held.pool.arrived(in, 1)
	r = &paced{r: r, pool: held.pool, in: in}
	if known >= 0 {
		return readKnown(r, known, first, in, q, held)
	}
	return readChunked(r, first, in, q, held)
}

// nextChunk takes the chunk for a body's next piece; the first came with its admission.
func nextChunk(out *read, in *intake, q *queue, held *heldBody) bool {
	if len(out.chunks) == 0 {
		return true
	}
	if !held.pool.more(in, q) {
		return false
	}
	held.took()
	return true
}

func readKnown(r io.Reader, known int64, first []byte, in *intake, q *queue, held *heldBody) read {
	var out read
	pending := first
	for out.size < known {
		if !nextChunk(&out, in, q, held) {
			out.verdict = "skipped:busy"
			return out
		}
		size := known - out.size
		if size > chunkSize {
			size = chunkSize
		}
		chunk := make([]byte, size)
		n := copy(chunk, pending)
		pending = nil
		m, err := fill(r, chunk[n:])
		n += m
		out.add(chunk[:n])
		if err != nil && !errors.Is(err, io.EOF) {
			out.verdict = "aborted"
			out.err = err
			return out
		}
		if out.size < known && (n < len(chunk) || errors.Is(err, io.EOF)) {
			out.verdict = "aborted"
			out.err = fmt.Errorf("body ended after %d of %d bytes", out.size, known)
			return out
		}
	}
	return out
}

// readChunked reads a body of unknown length, starting with its first byte. After a full piece, one
// byte read ahead tells whether the body goes on before another chunk is taken for it, so a body
// ending exactly where a piece ends takes none; that byte stays part of the body whatever comes
// next.
func readChunked(r io.Reader, first []byte, in *intake, q *queue, held *heldBody) read {
	var out read
	ahead := first
	for {
		if len(out.chunks) > 0 {
			ahead = nil
			var probe [1]byte
			n, err := fill(r, probe[:])
			if n > 0 {
				ahead = append([]byte(nil), probe[0])
			}
			if err != nil {
				out.add(ahead)
				return out.ended(err)
			}
		}
		if !nextChunk(&out, in, q, held) {
			out.add(ahead)
			out.verdict = "skipped:busy"
			return out
		}
		size := chunkSize
		if len(out.chunks) == 0 {
			size = firstChunkSize
		}
		chunk := make([]byte, size)
		n := copy(chunk, ahead)
		m, err := fill(r, chunk[n:])
		out.add(chunk[:n+m])
		if out.size > maxBytes {
			out.verdict = "skipped:too-large"
			return out
		}
		if err != nil {
			return out.ended(err)
		}
	}
}

// ended is a chunked body read up to an error: its end at io.EOF, else broken off.
func (o read) ended(err error) read {
	switch {
	case o.size > maxBytes:
		o.verdict = "skipped:too-large"
	case !errors.Is(err, io.EOF):
		o.verdict = "aborted"
		o.err = err
	}
	return o
}

// join copies a body's chunks into one buffer, which cutting a multipart body needs, gives it to
// held in their place - it keeps their memory in the pool - and clears them. One body at a time
// per Traefik: while it copies, the chunks and the copy exist together. Cleared because Yaegi keeps
// values it has handed around reachable; a large body's chunks also go back to the system before
// the next body may copy, since its buffer cannot reuse 1 MiB pieces and would take fresh memory.
// False when its turn does not come within the queue's time, or the client leaves meanwhile.
func join(q *queue, chunks [][]byte, held *heldBody) ([]byte, bool) {
	if len(chunks) == 1 {
		return chunks[0], true
	}
	if !q.turn(joining) {
		return nil, false
	}
	defer func() { <-joining }()
	out := bytes.Join(chunks, nil)
	held.hold([][]byte{out})
	for i := range chunks {
		chunks[i] = nil
	}
	if len(out) >= bigJoin {
		debug.FreeOSMemory()
	}
	return out, true
}

// chunkReader reads a body's chunks in order without changing them. Written here rather than
// io.MultiReader, which misbehaves under Yaegi with readers the plugin defines.
type chunkReader struct {
	chunks [][]byte
	next   int
	offset int
}

// Read moves on to the next chunk as soon as one is used up, so next counts the chunks read past.
func (c *chunkReader) Read(p []byte) (int, error) {
	for c.next < len(c.chunks) && c.offset == len(c.chunks[c.next]) {
		c.next++
		c.offset = 0
	}
	if c.next >= len(c.chunks) {
		return 0, io.EOF
	}
	n := copy(p, c.chunks[c.next][c.offset:])
	c.offset += n
	if c.offset == len(c.chunks[c.next]) {
		c.next++
		c.offset = 0
	}
	return n, nil
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
