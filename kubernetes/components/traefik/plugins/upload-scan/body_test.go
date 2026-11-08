package uploadscan

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestTooLargeWhileReadingKeepsTheBody(t *testing.T) {
	clamd := startClamd(t, 0)
	setMaxBytes(t, 10)
	s := newSetup(t, clamd.addr, nil)
	sent := bytes.Repeat([]byte("z"), 100)
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", io.NopCloser(bytes.NewReader(sent)))
	req.ContentLength = -1 // chunked: the size shows only while reading
	req.Header.Set("Content-Type", "application/zip")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	line := onlyLine(t, s)
	if line["verdict"] != "skipped:too-large" || !bytes.Equal(*s.got, sent) {
		t.Fatalf("unexpected: %v, backend got %d bytes", line, len(*s.got))
	}
}

// No room for the body's announced size: it is not scanned, the app still gets the whole body,
// and nothing stays taken.
func TestBusyWhenNoMemoryIsFreeKeepsTheBody(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "50ms" })
	occupy(t, pool.size-1) // one chunk left: the body below needs three
	sent := bytes.Repeat([]byte("m"), 2*chunkSize+10)

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", sent)

	line := onlyLine(t, s)
	if line["verdict"] != "skipped:busy" || line["bytes"] != float64(len(sent)) || !bytes.Equal(*s.got, sent) {
		t.Fatalf("unexpected: %v, backend got %d bytes", line, len(*s.got))
	}
	if pool.inUse() != pool.size-1 {
		t.Fatalf("%d chunks in use after the request, want only the occupied %d", pool.inUse(), pool.size-1)
	}
}

func TestMemoryIsReturnedAfterEveryRequest(t *testing.T) {
	clamd := startClamd(t, 0)
	for _, size := range []int{10, chunkSize, 3*chunkSize + 7} {
		s := newSetup(t, clamd.addr, nil)
		serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", bytes.Repeat([]byte("r"), size))
		s.logLines()
		if pool.inUse() != 0 {
			t.Fatalf("after a %d byte body %d chunks are still held", size, pool.inUse())
		}
	}
}

func TestChunkedBodyIsScannedAndKept(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	sent := append(eicar[:len(eicar):len(eicar)], []byte("\n")...)
	req := httptest.NewRequest(http.MethodPut, "https://site.example.org/upload", io.NopCloser(bytes.NewReader(sent)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	line := onlyLine(t, s)
	if line["verdict"] != "infected" || !bytes.Equal(*s.got, sent) {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestAbortedUploadIsNotAnError(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload",
		io.NopCloser(io.MultiReader(bytes.NewReader([]byte("part")), iotest.ErrReader(io.ErrClosedPipe))))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/zip")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "aborted" {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestSlowBodyIsNotBusy(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "50ms" })
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload",
		io.NopCloser(&slowReader{data: append([]byte{}, eicar...), delay: 100 * time.Millisecond}))
	req.ContentLength = int64(len(eicar))
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("reading slowly must not count as waiting: %v", line)
	}
}

func TestBodyShorterThanAnnouncedIsAborted(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", io.NopCloser(bytes.NewReader(eicar)))
	req.ContentLength = 10 << 20
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "aborted" {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestKnownLengthTakesExactlyItsMemory(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	before := pool.inUse()
	var during int
	backend := s.handler.(*UploadScan).next
	s.handler.(*UploadScan).next = http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		during = pool.inUse() - before
		backend.ServeHTTP(rw, req)
	})

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", bytes.Repeat([]byte("k"), chunkSize))

	s.logLines()
	if during != 1 || pool.inUse() != before {
		t.Fatalf("a 1 MiB body held %d chunks (want 1), %d left after", during, pool.inUse()-before)
	}
}

func TestBrokenChunkedBodyIsAborted(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	req := httptest.NewRequest(http.MethodPut, "https://site.example.org/upload",
		io.NopCloser(io.MultiReader(bytes.NewReader(eicar), iotest.ErrReader(io.ErrUnexpectedEOF))))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "aborted" {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestPanicDownstreamGivesTheMemoryBack(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	s.handler.(*UploadScan).next = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("downstream")
	})
	func() {
		defer func() { _ = recover() }()
		serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", bytes.Repeat([]byte("p"), 3*chunkSize))
	}()
	s.logLines()
	if pool.inUse() != 0 {
		t.Fatalf("%d chunks leaked by a panic", pool.inUse())
	}
}

func TestChunksAreGivenBackAsTheyArePassedOn(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	var whileRunning int
	s.handler.(*UploadScan).next = http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		whileRunning = pool.inUse()
	})

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", bytes.Repeat([]byte("c"), 3*chunkSize))

	s.logLines()
	if whileRunning != 0 {
		t.Fatalf("%d tokens still held after the app read the body", whileRunning)
	}
}

func TestEmptyChunkedBodyIsNotScanned(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", io.NopCloser(bytes.NewReader(nil)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if lines := s.logLines(); len(lines) != 0 || len(clamd.scans()) != 0 {
		t.Fatalf("an empty body was scanned: %v", lines)
	}
}

// A chunked body holds a chunk per piece it sent: a small one a single chunk, 1 MiB two (64 KiB
// first, then the rest) - never the most a body may take.
func TestChunkedBodyHoldsWhatItSent(t *testing.T) {
	for _, c := range []struct{ size, want int }{{2 << 10, 1}, {chunkSize, 2}} {
		clamd := startClamd(t, 0)
		s := newSetup(t, clamd.addr, nil)
		var during int
		backend := s.handler.(*UploadScan).next
		s.handler.(*UploadScan).next = http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			during = pool.inUse()
			backend.ServeHTTP(rw, req)
		})
		req := httptest.NewRequest(http.MethodPut, "https://site.example.org/upload", io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("c"), c.size))))
		req.ContentLength = -1
		req.Header.Set("Content-Type", "application/octet-stream")

		s.handler.ServeHTTP(httptest.NewRecorder(), req)

		s.logLines()
		if during != c.want || pool.inUse() != 0 {
			t.Fatalf("a chunked %d byte body held %d chunks (want %d), %d left after", c.size, during, c.want, pool.inUse())
		}
	}
}

// A chunked body that is not admitted logs what was read - the one byte read before asking - and
// the app still gets all of it.
func TestChunkedBusyLogsWhatWasRead(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "50ms" })
	occupy(t, pool.size)
	req := httptest.NewRequest(http.MethodPut, "https://site.example.org/upload", io.NopCloser(bytes.NewReader([]byte("data"))))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "skipped:busy" || line["bytes"] != 1.0 || string(*s.got) != "data" {
		t.Fatalf("unexpected: %v, backend got %q", line, *s.got)
	}
}

// A client that announces 100 MiB and then stalls holds memory only for what it sent: the chunk
// it filled and the one being filled - not its announced size.
func TestSlowSenderHoldsOnlyWhatItSent(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := &stallingBody{first: bytes.Repeat([]byte("s"), chunkSize), stalled: make(chan struct{})}
	release := stall(t, body)
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", body)
	req.ContentLength = maxBytes
	req.Header.Set("Content-Type", "application/zip")
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handler.ServeHTTP(httptest.NewRecorder(), req)
	}()

	until(t, "the upload holds two chunks", func() bool { return pool.inUse() >= 2 })
	time.Sleep(50 * time.Millisecond)
	held := pool.inUse()
	release()
	<-done

	if line := onlyLine(t, s); held != 2 || line["verdict"] != "aborted" || pool.inUse() != 0 {
		t.Fatalf("a stalled 100 MiB upload held %d chunks (want 2), %d left after: %v", held, pool.inUse(), line)
	}
}

// A multipart body over one chunk is joined, then cut exactly: the large file and the one after it.
func TestLargeMultipartIsJoinedAndCut(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	large := make([]byte, 3*chunkSize+77)
	for i := range large {
		large[i] = byte(i * 13)
	}
	body := "--b\r\nContent-Disposition: form-data; name=\"a\"; filename=\"a.bin\"\r\n\r\n" + string(large) +
		"\r\n--b\r\nContent-Disposition: form-data; name=\"e\"; filename=\"e.txt\"\r\n\r\n" + string(eicar) + "\r\n--b--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))

	if line := onlyLine(t, s); line["verdict"] != "infected" || !clamd.got(large) || !bytes.Equal(*s.got, []byte(body)) {
		t.Fatalf("unexpected: %v, clamd got the large file exactly: %v", line, clamd.got(large))
	}
}

// Go's proxy may read the body after closing it: that must not panic, the memory is back, and it
// gets an error - not the rest of the body with the chunks it had not read missing.
func TestReadAfterCloseIsSafe(t *testing.T) {
	clamd := startClamd(t, 0)
	newSetup(t, clamd.addr, nil)
	occupyRaw := func(n int) {
		pool.mu.Lock()
		pool.used += n
		pool.mu.Unlock()
	}
	occupyRaw(3)
	h := &heldBody{original: io.NopCloser(strings.NewReader("rest")), pool: pool}
	for i := 0; i < 3; i++ {
		h.took()
	}
	h.hold([][]byte{bytes.Repeat([]byte("a"), 10), bytes.Repeat([]byte("b"), 10), bytes.Repeat([]byte("c"), 10)})
	buf := make([]byte, 15)
	if _, err := io.ReadFull(h, buf); err != nil {
		t.Fatal(err)
	}
	_ = h.Close()

	got, err := io.ReadAll(h)
	if !errors.Is(err, http.ErrBodyReadAfterClose) || len(got) != 0 || pool.inUse() != 0 {
		t.Fatalf("after Close: %q, %v, %d chunks still in use", got, err, pool.inUse())
	}
}

// A chunked body that ends exactly where a chunk ends is complete: no further chunk taken to
// find its end, so a nearly full pool does not make it busy.
func TestChunkedBodyEndingOnAChunkBoundary(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "50ms" })
	occupy(t, pool.size-2)
	sent := bytes.Repeat([]byte("e"), firstChunkSize+chunkSize)
	req := httptest.NewRequest(http.MethodPut, "https://site.example.org/upload", io.NopCloser(bytes.NewReader(sent)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "clean" || !bytes.Equal(*s.got, sent) || pool.inUse() != pool.size-2 {
		t.Fatalf("unexpected: %v, %d chunks in use", line, pool.inUse())
	}
}

// Each chunk's memory goes back as the proxy reads past it, not only at the end.
func TestChunksGoBackOneByOne(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	var halfway int
	s.handler.(*UploadScan).next = http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		_, _ = io.ReadFull(req.Body, make([]byte, chunkSize+chunkSize/2))
		halfway = pool.inUse()
		_, _ = io.Copy(io.Discard, req.Body)
	})

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", bytes.Repeat([]byte("g"), 3*chunkSize))

	s.logLines()
	if halfway != 2 || pool.inUse() != 0 {
		t.Fatalf("after reading past one of three chunks %d were held (want 2), %d after", halfway, pool.inUse())
	}
}

// A chunked upload that runs out of memory halfway passes on every byte, the one read ahead to see
// whether it goes on included.
func TestChunkedBusyHalfwayKeepsEveryByte(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "50ms" })
	occupy(t, pool.size-1) // one left: the first piece takes it
	sent := make([]byte, 3*chunkSize)
	for i := range sent {
		sent[i] = byte(i * 11)
	}
	req := httptest.NewRequest(http.MethodPut, "https://site.example.org/upload", io.NopCloser(bytes.NewReader(sent)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	line := onlyLine(t, s)
	if line["verdict"] != "skipped:busy" || !bytes.Equal(*s.got, sent) || pool.inUse() != pool.size-1 {
		t.Fatalf("unexpected: %v, backend got %d of %d bytes, %d chunks in use", line, len(*s.got), len(sent), pool.inUse())
	}
}

// A join from 16 MiB hands its chunks back to the system (debug.FreeOSMemory, also under Yaegi);
// the body is still cut exactly.
func TestLargeJoinIsCutExactly(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	large := make([]byte, bigJoin+chunkSize)
	for i := range large {
		large[i] = byte(i * 3)
	}
	body := "--b\r\nContent-Disposition: form-data; name=\"a\"; filename=\"a.bin\"\r\n\r\n" + string(large) + "\r\n--b--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))

	if line := onlyLine(t, s); line["verdict"] != "clean" || !clamd.got(large) || !bytes.Equal(*s.got, []byte(body)) || pool.inUse() != 0 {
		t.Fatalf("unexpected: %v, clamd got it exactly: %v, %d chunks in use", line, clamd.got(large), pool.inUse())
	}
}

// An upload that connects and sends nothing holds no memory: it is admitted only with its first
// byte.
func TestIdleUploadHoldsNoMemory(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := &stallingBody{stalled: make(chan struct{})}
	release := stall(t, body)
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", body)
	req.ContentLength = maxBytes
	req.Header.Set("Content-Type", "application/zip")
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handler.ServeHTTP(httptest.NewRecorder(), req)
	}()

	time.Sleep(100 * time.Millisecond)
	held := pool.inUse()
	release()
	<-done

	if line := onlyLine(t, s); held != 0 || line["verdict"] != "aborted" || pool.inUse() != 0 {
		t.Fatalf("an upload that sent nothing held %d chunks: %v", held, line)
	}
}
