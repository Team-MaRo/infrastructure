package uploadscan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

// eicar is the antivirus test string, built from two halves so no file holds it whole - scanners
// on a developer's machine would flag this file otherwise.
var eicar = []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$` + `EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)

// fakeClamd answers INSTREAM like clamd: FOUND when the stream starts with the EICAR string, as
// ClamAV's signature requires. A delay makes it slow; it keeps every stream and its size.
type fakeClamd struct {
	addr    string
	delay   time.Duration
	mu      sync.Mutex
	sizes   []int
	streams [][]byte
}

// scans is how many streams clamd got, and their sizes.
func (c *fakeClamd) scans() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.sizes...)
}

// got is whether some stream to clamd was exactly b.
func (c *fakeClamd) got(b []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.streams {
		if bytes.Equal(s, b) {
			return true
		}
	}
	return false
}

func startClamd(t *testing.T, delay time.Duration) *fakeClamd {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	c := &fakeClamd{addr: ln.Addr().String(), delay: delay}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go c.serve(conn)
		}
	}()
	return c
}

func (c *fakeClamd) serve(conn net.Conn) {
	defer conn.Close()
	cmd := make([]byte, len("zINSTREAM\x00"))
	if _, err := io.ReadFull(conn, cmd); err != nil || string(cmd) != "zINSTREAM\x00" {
		return
	}
	var data bytes.Buffer
	size := make([]byte, 4)
	for {
		if _, err := io.ReadFull(conn, size); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(size)
		if n == 0 {
			break
		}
		if _, err := io.CopyN(&data, conn, int64(n)); err != nil {
			return
		}
	}
	c.mu.Lock()
	c.sizes = append(c.sizes, data.Len())
	if len(c.streams) < 1000 {
		c.streams = append(c.streams, append([]byte(nil), data.Bytes()...))
	}
	c.mu.Unlock()
	time.Sleep(c.delay)
	if bytes.Contains(data.Bytes(), []byte("BEYOND-LIMITS")) {
		_, _ = conn.Write([]byte("stream: Heuristics.Limits.Exceeded.MaxFileSize FOUND\x00"))
		return
	}
	if bytes.HasPrefix(data.Bytes(), eicar) {
		_, _ = conn.Write([]byte("stream: Eicar-Test-Signature FOUND\x00"))
		return
	}
	_, _ = conn.Write([]byte("stream: OK\x00"))
}

// setup builds the middleware in front of a backend that records the body it got, and captures
// the log lines.
type setup struct {
	handler  http.Handler
	got      *[]byte
	reached  *bool
	logLines func() []map[string]interface{}
}

// setMaxBytes changes the code's size limit for one test.
func setMaxBytes(t *testing.T, n int64) {
	t.Helper()
	old := maxBytes
	maxBytes = n
	t.Cleanup(func() { maxBytes = old })
}

// resetLimits lets each test size the shared pool from the limits it runs with.
func resetLimits() {
	limitsOnce = sync.Once{}
	pool = nil
	sessions = nil
}

// occupy takes n chunks of the pool for the rest of the test.
func occupy(t *testing.T, n int) {
	t.Helper()
	if ok, _ := pool.tryTake(n); !ok {
		t.Fatalf("could not take %d chunks", n)
	}
	t.Cleanup(func() { pool.give(n) })
}

func newSetup(t *testing.T, clamd string, change func(*Config)) setup {
	t.Helper()
	resetLimits()
	config := CreateConfig()
	config.Clamd = clamd
	config.Exclude = []string{"registry.example.org", "matomo.example.org/matomo.php"}
	if change != nil {
		change(config)
	}
	var got []byte
	reached := false
	var mu sync.Mutex
	backend := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		mu.Lock()
		reached = true
		got = body
		mu.Unlock()
		rw.WriteHeader(http.StatusOK)
	})
	handler, err := New(context.Background(), backend, config, "upload-scan")
	if err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	var captured bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&captured, r); close(done) }()
	finished := false
	finish := func() {
		if !finished {
			finished = true
			os.Stdout = stdout
			w.Close()
			<-done
		}
	}
	t.Cleanup(finish)

	return setup{
		handler: handler,
		got:     &got,
		reached: &reached,
		logLines: func() []map[string]interface{} {
			finish()
			var lines []map[string]interface{}
			for _, l := range strings.Split(strings.TrimSpace(captured.String()), "\n") {
				if l == "" {
					continue
				}
				var m map[string]interface{}
				if err := json.Unmarshal([]byte(l), &m); err != nil {
					t.Fatalf("log line is not JSON: %q", l)
				}
				lines = append(lines, m)
			}
			return lines
		},
	}
}

func multipartBody(t *testing.T, files map[string][]byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := mw.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		fw, err := mw.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, mw.FormDataContentType()
}

func serve(s setup, method, url, contentType string, body []byte) {
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	s.handler.ServeHTTP(httptest.NewRecorder(), req)
}

func onlyLine(t *testing.T, s setup) map[string]interface{} {
	t.Helper()
	lines := s.logLines()
	if len(lines) != 1 {
		t.Fatalf("want one log line, got %d: %v", len(lines), lines)
	}
	return lines[0]
}

func TestInfectedMultipartIsReportedAndPassed(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body, ct := multipartBody(t, map[string][]byte{"invoice.pdf": eicar}, map[string]string{"name": "x"})
	sent := body.Bytes()

	serve(s, http.MethodPost, "https://shop.example.org/wp-admin/async-upload.php?x=1", ct, sent)

	if !*s.reached || !bytes.Equal(*s.got, sent) {
		t.Fatal("the backend did not get the identical body")
	}
	line := onlyLine(t, s)
	if line["verdict"] != "infected" || line["sig"] != "Eicar-Test-Signature" || line["parts"] != 2.0 {
		t.Fatalf("unexpected log line: %v", line)
	}
	if line["path"] != "/wp-admin/async-upload.php" || line["host"] != "shop.example.org" {
		t.Fatalf("path or host wrong (query must not be logged): %v", line)
	}
	if strings.Contains(strings.Join(mapValues(line), " "), "invoice") {
		t.Fatalf("file name leaked into the log: %v", line)
	}
}

func TestCleanFilesAreEachScanned(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body, ct := multipartBody(t, map[string][]byte{"a.txt": []byte("hello"), "b.txt": bytes.Repeat([]byte("x"), 200<<10)}, nil)

	serve(s, http.MethodPost, "https://site.example.org/upload", ct, body.Bytes())

	line := onlyLine(t, s)
	if line["verdict"] != "clean" || line["parts"] != 2.0 {
		t.Fatalf("unexpected log line: %v", line)
	}
	sizes := clamd.scans()
	if len(sizes) != 2 || sizes[0]+sizes[1] != 5+200<<10 {
		t.Fatalf("clamd got %v bytes, want exactly the two files", sizes)
	}
}

func TestRawBodyIsScannedWhole(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	sent := append([]byte{}, eicar...)

	serve(s, http.MethodPost, "https://site.example.org/wp-json/wp/v2/media", "application/pdf", sent)

	line := onlyLine(t, s)
	if line["verdict"] != "infected" || !bytes.Equal(*s.got, sent) {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestSkippedRequestsAreNotTouched(t *testing.T) {
	clamd := startClamd(t, 0)
	cases := []struct{ method, url, ct string }{
		{http.MethodPost, "https://site.example.org/svc", "application/grpc"},
		{http.MethodPost, "https://site.example.org/form", "application/x-www-form-urlencoded"},
		{http.MethodPost, "https://site.example.org/api", "application/json; charset=utf-8"},
		{http.MethodPost, "https://site.example.org/api", "application/vnd.api+json"},
		{http.MethodPost, "https://site.example.org/svc", "application/grpc+proto"},
		{http.MethodPost, "https://argocd.example.org/application.ApplicationService/List", "application/grpc-web+proto"},
		{http.MethodPost, "https://auth.example.org/zitadel.user.v2.UserService/ListUsers", "application/connect+proto"},
		{http.MethodPost, "https://auth.example.org/zitadel.user.v2.UserService/GetUser", "application/proto"},
		{http.MethodPut, "https://registry.example.org/v2/x/blobs/uploads/1", "application/octet-stream"},
		{http.MethodPost, "https://matomo.example.org/matomo.php?idsite=1", "text/plain"},
	}
	for _, c := range cases {
		s := newSetup(t, clamd.addr, nil)
		serve(s, c.method, c.url, c.ct, eicar)
		if !*s.reached || len(s.logLines()) != 0 {
			t.Fatalf("%s %s (%s) must pass untouched", c.method, c.url, c.ct)
		}
	}
	if n := clamd.scans(); len(n) > 0 {
		t.Fatalf("clamd was asked (%v bytes) for a skipped request", n)
	}
}

func TestTooLargeByHeaderIsNotRead(t *testing.T) {
	clamd := startClamd(t, 0)
	setMaxBytes(t, 10)
	s := newSetup(t, clamd.addr, nil)
	sent := bytes.Repeat([]byte("y"), 100)

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", sent)

	line := onlyLine(t, s)
	if line["verdict"] != "skipped:too-large" || !bytes.Equal(*s.got, sent) {
		t.Fatalf("unexpected: %v", line)
	}
}

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

func TestClamdDownIsAnErrorAndPasses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	s := newSetup(t, addr, nil)

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", eicar)

	line := onlyLine(t, s)
	if line["verdict"] != "error" || !*s.reached {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestSlowClamdTimesOut(t *testing.T) {
	clamd := startClamd(t, 2*time.Second)
	s := newSetup(t, clamd.addr, func(c *Config) { c.ScanTimeout = "200ms" })

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", []byte("data"))

	line := onlyLine(t, s)
	if line["verdict"] != "skipped:time" || line["parts"] != 0.0 {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestBusyWhenNoSessionIsFree(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "50ms" })
	for i := 0; i < cap(sessions); i++ {
		sessions <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(sessions); i++ {
			<-sessions
		}
	}()
	sent := []byte("data")

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", sent)

	line := onlyLine(t, s)
	if line["verdict"] != "skipped:busy" || line["bytes"] != 4.0 || !bytes.Equal(*s.got, sent) {
		t.Fatalf("unexpected: %v", line)
	}
}

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

func TestMalformedFilePartIsScanned(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	// filename twice: Go's parser gives up on the header, PHP still stores the file.
	body := "--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.php\"; filename=\"a.php\"\r\n\r\n" +
		string(eicar) + "\r\n--b--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))

	line := onlyLine(t, s)
	if line["verdict"] != "infected" || line["parts"] != 1.0 {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestQuotedPrintablePartIsScannedRaw(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	// Raw, the part holds the EICAR string; decoded as quoted-printable it would not.
	body := "--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"e.txt\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" + string(eicar) + "\r\n--b--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestExclusionsAreExactPerHost(t *testing.T) {
	clamd := startClamd(t, 0)
	cases := []struct {
		url     string
		scanned bool
	}{
		{"https://matomo.example.org/matomo.php", false},
		{"https://matomo.example.org/matomo.phpX", true},
		{"https://site.example.org/matomo.php", true},
		{"https://registry.example.org/v2/anything", false},
	}
	for _, c := range cases {
		s := newSetup(t, clamd.addr, nil)
		serve(s, http.MethodPost, c.url, "application/octet-stream", eicar)
		if got := len(s.logLines()) == 1; got != c.scanned {
			t.Fatalf("%s: scanned=%v, want %v", c.url, got, c.scanned)
		}
	}
}

func TestOneBudgetForAllFilesOfARequest(t *testing.T) {
	clamd := startClamd(t, 2*time.Second)
	s := newSetup(t, clamd.addr, func(c *Config) { c.ScanTimeout = "300ms" })
	files := map[string][]byte{}
	for i := 0; i < 5; i++ {
		files[string(rune('a'+i))+".txt"] = []byte("x")
	}
	body, ct := multipartBody(t, files, nil)

	start := time.Now()
	serve(s, http.MethodPost, "https://site.example.org/upload", ct, body.Bytes())

	if took := time.Since(start); took > time.Second {
		t.Fatalf("five files took %v, the budget is for the whole request", took)
	}
	if line := onlyLine(t, s); line["verdict"] != "skipped:time" {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestUnreachableClamdIsTriedOncePerRequest(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	s := newSetup(t, addr, nil)
	body, ct := multipartBody(t, map[string][]byte{"a": []byte("1"), "b": []byte("2"), "c": []byte("3")}, nil)

	serve(s, http.MethodPost, "https://site.example.org/upload", ct, body.Bytes())

	line := onlyLine(t, s)
	if line["verdict"] != "error" || line["parts"] != 0.0 {
		t.Fatalf("unexpected: %v", line)
	}
	res := result{}
	s.handler.(*UploadScan).scanMultipart(newScanning(context.Background(), time.Now().Add(time.Second)), body.Bytes(), boundaries([]string{ct}), &res)
	if res.Files != 1 {
		t.Fatalf("want one attempt, then stop: %d", res.Files)
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

func TestOnlyReportModeIsAccepted(t *testing.T) {
	config := CreateConfig()
	config.Clamd = "127.0.0.1:3310"
	config.Mode = "block"
	if _, err := New(context.Background(), http.NotFoundHandler(), config, "upload-scan"); err == nil {
		t.Fatal("block mode must be refused until phase 3")
	}
}

func mapValues(m map[string]interface{}) []string {
	var out []string
	for _, v := range m {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func TestAmbiguousBoundaryScansThePartsOfBoth(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	// PHP splits by the first "boundary" (evil) and stores the file; Go would split by "good" and
	// see only the text field.
	body := "--evil\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\r\n\r\n" + string(eicar) +
		"\r\n--evil--\r\n--good\r\nContent-Disposition: form-data; name=\"x\"\r\n\r\ny\r\n--good--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; xboundary=evil; boundary=good", []byte(body))

	line := onlyLine(t, s)
	if line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestSkipTypeDoesNotHideAFile(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/wp-json/wp/v2/media", bytes.NewReader(eicar))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Disposition", "attachment; filename=x.pdf")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestLargeSkipTypeBodyIsScanned(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	sent := append([]byte{}, eicar...)
	sent = append(sent, bytes.Repeat([]byte(" "), 70<<10)...)

	serve(s, http.MethodPost, "https://site.example.org/form", "application/x-www-form-urlencoded", sent)

	if len(s.logLines()) != 1 {
		t.Fatal("a body larger than a form must be scanned whatever its type")
	}
}

func TestEveryFilePartIsScannedEvenMany(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	var body strings.Builder
	for i := 0; i < 25; i++ {
		content := "x"
		if i == 21 {
			content = string(eicar)
		}
		body.WriteString("--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"" + strconv.Itoa(i) + ".txt\"\r\n\r\n" + content + "\r\n")
	}
	body.WriteString("--b--\r\n")

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body.String()))

	line := onlyLine(t, s)
	if line["verdict"] != "infected" || line["parts"] != 22.0 {
		t.Fatalf("want the 22nd file found on its own: %v", line)
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

func TestLimitsExceededIsNotInfected(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", []byte("BEYOND-LIMITS"))

	line := onlyLine(t, s)
	if line["verdict"] != "limits-exceeded" || line["sig"] != "Heuristics.Limits.Exceeded.MaxFileSize" {
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

type slowReader struct {
	data  []byte
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	end := 32
	if end > len(r.data) {
		end = len(r.data)
	}
	n := copy(p, r.data[:end])
	r.data = r.data[n:]
	return n, nil
}

// Browsers put the word into the boundary itself; that is not ambiguous.
func TestBrowserBoundaryIsReadPartByPart(t *testing.T) {
	clamd := startClamd(t, 0)
	for _, boundary := range []string{"----WebKitFormBoundary7MA4YWxkTrZu0gW", "----geckoformboundary4d1a"} {
		s := newSetup(t, clamd.addr, nil)
		field := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"comment\"\r\n\r\nhello\r\n"
		file := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\r\n\r\n" + string(eicar) + "\r\n"
		serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary="+boundary,
			[]byte(field+file+"--"+boundary+"--\r\n"))

		line := onlyLine(t, s)
		if line["verdict"] != "infected" || line["parts"] != 2.0 || line["note"] != nil {
			t.Fatalf("%s: unexpected: %v", boundary, line)
		}
	}
}

func TestStreamIsPassedOnUnread(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	r, w := io.Pipe()
	req := httptest.NewRequest(http.MethodPost, "https://argocd.example.org/application.ApplicationService/Watch", r)
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/grpc")
	done := make(chan struct{})
	go func() {
		s.handler.ServeHTTP(httptest.NewRecorder(), req)
		close(done)
	}()
	if _, err := w.Write([]byte("first frame")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	_ = w.Close()
	<-done
	if string(*s.got) != "first frame" || len(s.logLines()) != 0 {
		t.Fatalf("the stream must pass unread and unlogged, backend got %q", *s.got)
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

// Waiting for memory counts against the queue time, so waiters do not line up.
func TestWaitersDoNotQueueBehindEachOther(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "300ms" })
	occupy(t, pool.size)

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", []byte("data"))
		}()
	}
	wg.Wait()
	if took := time.Since(start); took > 1200*time.Millisecond {
		t.Fatalf("five waiters took %v, each may wait 300ms at most", took)
	}
	for _, line := range s.logLines() {
		if line["verdict"] != "skipped:busy" {
			t.Fatalf("unexpected: %v", line)
		}
	}
}

func TestUnparsableContentTypeIsReadByPHPsBoundary(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := "--good\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\r\n\r\n" + string(eicar) + "\r\n--good--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=good; boundary=evil", []byte(body))

	line := onlyLine(t, s)
	if line["verdict"] != "infected" || line["note"] != nil {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestPartsBeforeABreakAreScannedOnTheirOwn(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := "--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\r\n\r\n" + string(eicar) +
		"\r\n--b\r\nbroken header without colon\r\n\r\nrest"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
	}
}

// Without its closing boundary, a part ends in ErrUnexpectedEOF; PHP stores what came.
func TestPartWithoutClosingBoundaryIsScanned(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := "--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\r\n\r\n" + string(eicar)

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))

	if line := onlyLine(t, s); line["verdict"] != "infected" {
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

func TestClientGoneEndsTheWait(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "5s" })
	for i := 0; i < cap(sessions); i++ {
		sessions <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(sessions); i++ {
			<-sessions
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", bytes.NewReader(eicar)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/zip")
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if took := time.Since(start); took > time.Second {
		t.Fatalf("a gone client was waited for %v", took)
	}
	s.logLines()
}

func TestRepeatedContentTypeIsReadByEveryValue(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := "--x\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\r\n\r\n" + string(eicar) + "\r\n--x--\r\n"
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", strings.NewReader(body))
	req.Header.Add("Content-Type", "multipart/form-data")
	req.Header.Add("Content-Type", "text/plain; boundary=x")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
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

func TestBoundaries(t *testing.T) {
	cases := map[string][]string{
		"multipart/form-data; boundary=abc":                     {"abc"},
		`multipart/form-data; boundary="a b;c"`:                 {"a b;c", "a b"},
		"multipart/form-data; xboundary=evil; boundary=good":    {"evil", "good"},
		"multipart/form-data; BOUNDARY=up; charset=utf-8":       {"up"},
		"multipart/form-data; boundary = spaced":                {" spaced", "spaced"},
		"multipart/form-data; boundary=b ":                      {"b ", "b"},
		"multipart/form-data; boundary=----WebKitFormBoundaryX": {"----WebKitFormBoundaryX"},
	}
	for ct, want := range cases {
		got := boundaries([]string{ct})
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s: %q, want %q", ct, got, want)
		}
	}
}

func TestFormFieldsAreScannedToo(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body, ct := multipartBody(t, nil, map[string]string{"comment": string(eicar)})

	serve(s, http.MethodPost, "https://site.example.org/contact", ct, body.Bytes())

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("a field may be a file to some parser: %v", line)
	}
}

func TestBodyWithoutTheBoundaryIsScannedWhole(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", eicar)

	line := onlyLine(t, s)
	if line["verdict"] != "infected" || !strings.Contains(line["note"].(string), "scanned whole") {
		t.Fatalf("unexpected: %v", line)
	}
}

// Bodies other parsers read differently from Go's (found with PHP, 2026-11-06): each must still
// reach clamd with the file at the start of what is scanned.
func TestPartsThatParsersReadDifferently(t *testing.T) {
	e := string(eicar)
	cases := map[string]string{
		"after the closing delimiter": "--b\r\nContent-Disposition: form-data; name=\"t\"\r\n\r\nx\r\n--b--\r\n" +
			"--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\r\n\r\n" + e + "\r\n--b--\r\n",
		"LF delimiter inside a CRLF body": "--b\r\nContent-Disposition: form-data; name=\"t\"\r\n\r\nx\n--b\n" +
			"Content-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\n\n" + e + "\n--b--\r\n",
		"mixed endings, no closing CRLF": "--b\r\nContent-Disposition: form-data; name=\"t\"\r\n\r\nx\n--b\n" +
			"Content-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\n\n" + e + "\n--b--\n",
		"header line without a colon": "--b\r\njunk\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e.txt\"\r\n\r\n" +
			e + "\r\n--b--\r\n",
		"headers ending in LF CR LF": "--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e.php\"\n\r\n" +
			e + "\r\n--b--",
	}
	for name, body := range cases {
		clamd := startClamd(t, 0)
		s := newSetup(t, clamd.addr, nil)
		serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))
		if line := onlyLine(t, s); line["verdict"] != "infected" {
			t.Errorf("%s: unexpected: %v", name, line)
		}
	}
}

func TestLoggedTextsAreClipped(t *testing.T) {
	if got := clip(strings.Repeat("a", 5000)); len(got) > 210 {
		t.Fatalf("clip kept %d bytes", len(got))
	}
}

// Signatures over a whole file (hashes with its size) need the file exactly: no line break of the
// delimiter, no header in front.
func TestEachFileReachesClamdExactly(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	file := []byte("MZ\x90\x00 a file whose exact bytes matter\r\n\r\nincluding blank lines\n")
	body, ct := multipartBody(t, map[string][]byte{"a.exe": file}, map[string]string{"comment": "hi"})

	serve(s, http.MethodPost, "https://site.example.org/upload", ct, body.Bytes())
	s.logLines()

	if clamd.got(file) {
		return
	}
	t.Fatal("no stream to clamd was exactly the file")
}

// A boundary chosen to appear inside a file (not at a line start) must not cut the file.
func TestBoundaryInsideAFileDoesNotCutIt(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	file := []byte("MZ-HEAD--bTAIL-OF-FILE")
	body := "--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"x\"\r\n\r\n" + string(file) + "\r\n--b--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))
	s.logLines()

	if clamd.got(file) {
		return
	}
	t.Fatal("the file was not scanned in one piece")
}

func TestManyBoundaryValuesAreBounded(t *testing.T) {
	var values []string
	for i := 0; i < 1000; i++ {
		values = append(values, "multipart/form-data; boundary=b"+strconv.Itoa(i))
	}
	if got := boundaries(values); len(got) > 3*maxHeaderValues {
		t.Fatalf("%d boundaries from 1000 header values", len(got))
	}
}

func TestInvalidUTF8InContentTypeDoesNotPanic(t *testing.T) {
	ct := "multipart/form-data; \xff\xff\xff\xff\xff\xff\xff\xffboundary=real"
	if got := boundaries([]string{ct}); len(got) == 0 || got[0] != "real" {
		t.Fatalf("got %q", got)
	}
}

// Many delimiters must not build a list of pieces in memory; the time budget or the most parts end
// the scan.
func TestManyDelimitersStayWithinTheBudget(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.ScanTimeout = "300ms" })
	body := bytes.Repeat([]byte("\n--b\n\nx"), 200000)

	start := time.Now()
	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", body)
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("took %v", took)
	}
	line := onlyLine(t, s)
	if note := line["note"].(string); !strings.Contains(note, "time used up") && !strings.Contains(note, "more than 1000") {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestAnyContentTypeValueCanAskForTheScan(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := "--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e\"\r\n\r\n" + string(eicar) + "\r\n--b--\r\n"
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", strings.NewReader(body))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Content-Type", "multipart/form-data; boundary=b")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
	}
}

// Found by the random comparison with PHP (2026-11-06): a file whose content starts with a line
// beginning "--boundary" right after the headers is not cut there by parsers.
func TestDelimiterRightAfterTheHeadersIsContent(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	file := []byte("--b- not a delimiter\nrest of the file")
	body := "--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"x\"\r\n\r\n" + string(file) + "\r\n--b--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))
	s.logLines()

	if clamd.got(file) {
		return
	}
	t.Fatal("the file was not scanned as PHP stores it")
}

// Found by the random comparison too: a part without Content-Disposition whose headers are
// directly followed by a delimiter - PHP ends it there, and the next part is a file of its own.
func TestDelimiterRightAfterTheHeadersAlsoStartsAPart(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	file := []byte("the file PHP stores")
	body := "--bjunk\nmore junk\n\n--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"x\"\r\n\r\n" +
		string(file) + "\r\n--b--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body))
	s.logLines()

	if clamd.got(file) {
		return
	}
	t.Fatal("the second part was not scanned on its own")
}

// Cutting stays linear: a body of nothing but delimiters must not take minutes.
func TestOnlyDelimitersCutQuickly(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.ScanTimeout = "50ms" })
	body := bytes.Repeat([]byte("--b\n"), 256<<10)

	start := time.Now()
	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", body)
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("1 MiB of delimiters took %v", took)
	}
	s.logLines()
}

func TestAnyMethodWithABodyIsScanned(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)

	serve(s, http.MethodDelete, "https://site.example.org/api/files", "application/octet-stream", eicar)

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestLongHostAndPathAreClipped(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)

	serve(s, http.MethodPost, "https://site.example.org/"+strings.Repeat("p", 60000), "application/octet-stream", eicar)

	if line := onlyLine(t, s); len(line["path"].(string)) > 210 {
		t.Fatalf("path logged with %d bytes", len(line["path"].(string)))
	}
}

func TestPlainBodiesAreNotAmbiguous(t *testing.T) {
	goBody, goType := multipartBody(t, map[string][]byte{"a.txt": []byte("file")}, map[string]string{"c": "hi"})
	w := "----WebKitFormBoundary7MA4YWxkTrZu0gW"
	chrome := "--" + w + "\r\nContent-Disposition: form-data; name=\"c\"\r\n\r\nhi\r\n--" + w +
		"\r\nContent-Disposition: form-data; name=\"f\"; filename=\"a.txt\"\r\nContent-Type: text/plain\r\n\r\nfile\r\n--" + w + "--\r\n"
	cases := map[string]struct {
		ct   string
		body []byte
	}{
		"Go's writer":              {goType, goBody.Bytes()},
		"Chrome":                   {"multipart/form-data; boundary=" + w, []byte(chrome)},
		"quoted":                   {`multipart/form-data; boundary="q"`, []byte("--q\r\nContent-Disposition: form-data; name=\"c\"\r\n\r\nhi\r\n--q--\r\n")},
		"spaces after a delimiter": {"multipart/form-data; boundary=b", []byte("--b \r\nContent-Disposition: form-data; name=\"c\"\r\n\r\nhi\r\n--b-- \r\n")},
	}
	for name, c := range cases {
		if why := ambiguousFor([]string{c.ct}, c.body); why != "" {
			t.Errorf("%s: ambiguous (%s)", name, why)
		}
	}
}

func TestCraftedBodiesAreAmbiguous(t *testing.T) {
	file := "Content-Disposition: form-data; name=\"f\"; filename=\"e\"\r\n\r\nx\r\n"
	plain := "--b\r\n" + file + "--b--\r\n"
	cases := map[string]struct {
		values []string
		body   string
		want   string
	}{
		"two Content-Type headers": {[]string{"multipart/form-data; boundary=b", "text/plain"}, plain, "more than once"},
		"xboundary":                {[]string{"multipart/form-data; xboundary=evil; boundary=b"}, plain, "read differently"},
		"BOUNDARY and zboundary":   {[]string{"multipart/form-data; BOUNDARY=b; zboundary=c"}, plain, "read differently"},
		"boundaryx":                {[]string{"multipart/form-data; boundaryx=b"}, plain, "read differently"},
		"boundary twice":           {[]string{"multipart/form-data; boundary=b; boundary=c"}, plain, "not parsable"},
		"space after =":            {[]string{"multipart/form-data; boundary= b"}, plain, "read differently"},
		"preamble":                 {[]string{"multipart/form-data; boundary=b"}, "preamble\r\n" + plain, "does not start"},
		"boundary in content":      {[]string{"multipart/form-data; boundary=b"}, "--b\r\n" + strings.Repeat("x", 100) + "--b\r\n" + file + "--b--\r\n", "inside the content"},
		"mixed line breaks":        {[]string{"multipart/form-data; boundary=b"}, "--b\r\n" + "Content-Disposition: form-data; name=\"f\"\r\n\r\nx\n--b\r\n" + file + "--b--\r\n", "mixed"},
		"no closing":               {[]string{"multipart/form-data; boundary=b"}, "--b\r\n" + file, "no closing"},
		"after the closing":        {[]string{"multipart/form-data; boundary=b"}, plain + "--b\r\n" + file, "after the closing"},
		"header without colon":     {[]string{"multipart/form-data; boundary=b"}, "--b\r\njunk\r\n" + file + "--b--\r\n", "not valid"},
		"first line LF, then CRLF": {[]string{"multipart/form-data; boundary=b"}, "--b\n" + file + "--b--\n", "mixed"},
		"closing in the middle":    {[]string{"multipart/form-data; boundary=b"}, plain + "--b\r\n" + file + "--b--\r\n", "after the closing"},
		"multipart/mixed":          {[]string{"multipart/mixed; boundary=b"}, plain, "other than form-data"},
		"tab before ;":             {[]string{"multipart/form-data\t; boundary=b"}, plain, "other than form-data"},
		"long header line":         {[]string{"multipart/form-data; boundary=b"}, "--b\r\nX-Pad: " + strings.Repeat("a", 5112) + "\r\n" + file + "--b--\r\n", "part headers too long"},
		"too many parts":           {[]string{"multipart/form-data; boundary=b"}, strings.Repeat("--b\r\n"+file, 1001) + "--b--\r\n", "more than 1000"},
		"delimiter line goes on":   {[]string{"multipart/form-data; boundary=b"}, "--b\r\n" + file + "--bogus\r\n" + file + "--b--\r\n", "inside the content"},
		"transfer encoding":        {[]string{"multipart/form-data; boundary=b"}, "--b\r\nContent-Transfer-Encoding: base64\r\n" + file + "--b--\r\n", "transfer encoding"},
	}
	for name, c := range cases {
		if why := ambiguousFor(c.values, []byte(c.body)); !strings.Contains(why, c.want) {
			t.Errorf("%s: %q, want it to name %q", name, why, c.want)
		}
	}
}

func TestStoppedPartwayIsIncomplete(t *testing.T) {
	clamd := startClamd(t, 300*time.Millisecond)
	s := newSetup(t, clamd.addr, func(c *Config) { c.ScanTimeout = "500ms" })
	var body strings.Builder
	for i := 0; i < 10; i++ {
		body.WriteString("--b\r\nContent-Disposition: form-data; name=\"f" + strconv.Itoa(i) + "\"\r\n\r\nx\r\n")
	}
	body.WriteString("--b--\r\n")

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body.String()))

	if line := onlyLine(t, s); line["verdict"] != "incomplete" {
		t.Fatalf("parts were left unscanned, yet: %v", line)
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

func TestAmbiguousIsLogged(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b",
		[]byte("preamble\r\n--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e\"\r\n\r\n"+string(eicar)+"\r\n--b--\r\n"))

	line := onlyLine(t, s)
	if line["verdict"] != "infected" || line["ambiguous"] == nil {
		t.Fatalf("unexpected: %v", line)
	}
}

// A large upload waiting for memory holds none of it, so a small one behind it gets through.
func TestSmallUploadDoesNotWaitBehindALargeOne(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "2s" })
	occupy(t, pool.size-2) // two chunks left: the large body needs three
	large := make(chan struct{})
	go func() {
		defer close(large)
		serve(s, http.MethodPost, "https://site.example.org/large", "application/zip", bytes.Repeat([]byte("l"), 2*chunkSize+10))
	}()
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	serve(s, http.MethodPost, "https://site.example.org/small", "application/zip", []byte("data"))
	took := time.Since(start)
	<-large

	verdicts := map[string]interface{}{}
	for _, line := range s.logLines() {
		verdicts[line["path"].(string)] = line["verdict"]
	}
	if verdicts["/small"] != "clean" || verdicts["/large"] != "skipped:busy" || took > time.Second {
		t.Fatalf("small took %v: %v", took, verdicts)
	}
}

func TestChunkedBodyHoldsTwiceItsChunks(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	var during int
	backend := s.handler.(*UploadScan).next
	s.handler.(*UploadScan).next = http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		during = pool.inUse()
		backend.ServeHTTP(rw, req)
	})
	req := httptest.NewRequest(http.MethodPut, "https://site.example.org/upload", io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("c"), chunkSize))))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	s.logLines()
	if during != 2 || pool.inUse() != 0 {
		t.Fatalf("a chunked 1 MiB body held %d chunks (want 2), %d left after", during, pool.inUse())
	}
}

func TestChunkedBusyLogsNoSize(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "50ms" })
	occupy(t, pool.size)
	req := httptest.NewRequest(http.MethodPut, "https://site.example.org/upload", io.NopCloser(bytes.NewReader([]byte("data"))))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "skipped:busy" || line["bytes"] != 0.0 {
		t.Fatalf("unexpected: %v", line)
	}
}

func ambiguousFor(values []string, body []byte) string {
	return ambiguous(http.Header{"Content-Type": values}, boundaries(values), body)
}

func TestCompressedMultipartIsAmbiguous(t *testing.T) {
	values := []string{"multipart/form-data; boundary=b"}
	body := []byte("--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e\"\r\n\r\nx\r\n--b--\r\n")
	header := http.Header{"Content-Type": values, "Content-Encoding": {"gzip"}}
	if why := ambiguous(header, boundaries(values), body); !strings.Contains(why, "Content-Encoding") {
		t.Fatalf("%q", why)
	}
	header["Content-Encoding"] = []string{"identity"}
	if why := ambiguous(header, boundaries(values), body); why != "" {
		t.Fatalf("identity is no encoding: %q", why)
	}
}

// A part that hit ClamAV's limits and then the time running out leaves parts unscanned.
func TestStoppedAfterLimitsExceededIsIncomplete(t *testing.T) {
	res := result{Files: 1, Verdict: "limits-exceeded", Sig: "Heuristics.Limits.Exceeded.MaxFiles"}
	u := &UploadScan{}
	res.Files++
	u.record(&res, scanResult{stopped: true})
	res.finish()
	if res.Verdict != "incomplete" || res.answered() != 1 || !strings.Contains(res.Note, "limits-exceeded") {
		t.Fatalf("unexpected: %+v", res)
	}
}

func TestExcludedPathIsCaseSensitive(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.Exclude = []string{"Site.Example.org/Upload.php"} })

	serve(s, http.MethodPost, "https://SITE.example.org/Upload.php", "application/zip", []byte("data"))
	serve(s, http.MethodPost, "https://site.example.org/upload.php", "application/zip", []byte("data"))

	lines := s.logLines()
	if len(lines) != 1 || lines[0]["path"] != "/upload.php" {
		t.Fatalf("want only /upload.php scanned: %v", lines)
	}
}

// Report mode lets every request pass, even when the scan itself fails.
func TestPanicInTheScanStillPasses(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	close(sessions) // taking a session now panics
	sent := []byte("data")

	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", sent)

	line := onlyLine(t, s)
	if line["verdict"] != "error" || !strings.Contains(line["error"].(string), "panic") || !bytes.Equal(*s.got, sent) {
		t.Fatalf("unexpected: %v, backend got %q", line, *s.got)
	}
}

func TestFreeSessionIsTakenWithTheQueueTimeUsedUp(t *testing.T) {
	clamd := startClamd(t, 0)
	newSetup(t, clamd.addr, nil)
	q := &queue{left: 0, ctx: context.Background()}
	for i := 0; i < 20; i++ {
		if !q.session() {
			t.Fatal("a free session was not taken")
		}
		<-sessions
	}
}

// Only form-data is cut into files by parsers; the app gets any other multipart body as it is.
func TestOtherMultipartIsScannedWholeToo(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := []byte("--b\r\nContent-Type: text/plain\r\n\r\none\r\n--b\r\nContent-Type: text/plain\r\n\r\ntwo\r\n--b--\r\n")

	serve(s, http.MethodPost, "https://site.example.org/wp-json/wp/v2/media", "multipart/mixed; boundary=b", body)

	line := onlyLine(t, s)
	if line["verdict"] != "clean" || line["parts"] != 3.0 || !clamd.got(body) || !clamd.got([]byte("one")) {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestFormDataPlusJSONIsNotSkipped(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := "--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e\"\r\n\r\n" + string(eicar) + "\r\n--b--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data+json; boundary=b", []byte(body))

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
	}
}

// "boundary=b " is read as "b " and "b", which cut at the same lines: each part once.
func TestPrefixBoundariesScanEachPartOnce(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	var body strings.Builder
	for i := 0; i < 3; i++ {
		body.WriteString("--b \r\nContent-Disposition: form-data; name=\"f" + strconv.Itoa(i) + "\"; filename=\"x\"\r\n\r\npart" + strconv.Itoa(i) + "\r\n")
	}
	body.WriteString("--b --\r\n")

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b ", []byte(body.String()))

	if line := onlyLine(t, s); line["parts"] != 3.0 || len(clamd.scans()) != 3 {
		t.Fatalf("unexpected: %v, clamd got %d streams", line, len(clamd.scans()))
	}
}

func TestManyDelimitersStopTheAmbiguityCheckEarly(t *testing.T) {
	values := []string{"multipart/form-data; boundary=b"}
	body := bytes.Repeat([]byte("--b\n"), 10<<20/4)
	start := time.Now()
	why := ambiguousFor(values, body)
	if took := time.Since(start); !strings.Contains(why, "more than 1000") || took > time.Second {
		t.Fatalf("%q after %v", why, took)
	}
}

// A part without an empty line has headers up to the next delimiter: bounded, not walked line by
// line.
func TestHeadersWithoutEndAreBounded(t *testing.T) {
	values := []string{"multipart/form-data; boundary=b"}
	body := append(append([]byte("--b\r\n"), bytes.Repeat([]byte("a\n"), 10<<20/2)...), "--b--\r\n"...)
	start := time.Now()
	why := ambiguousFor(values, body)
	if took := time.Since(start); !strings.Contains(why, "part headers too long") || took > time.Second {
		t.Fatalf("%q after %v", why, took)
	}
}

func TestScanningStopsAtTheMostParts(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	var body strings.Builder
	for i := 0; i < maxParts+5; i++ {
		body.WriteString("--b\r\nContent-Disposition: form-data; name=\"f\"\r\n\r\nx\r\n")
	}
	body.WriteString("--b--\r\n")

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", []byte(body.String()))

	line := onlyLine(t, s)
	if line["verdict"] != "incomplete" || line["parts"] != float64(maxParts) || !strings.Contains(line["note"].(string), "more than 1000") {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestAnyContentDispositionIsScanned(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/wp-json/wp/v2/media", bytes.NewReader(eicar))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Disposition", "")
	req.Header.Add("Content-Disposition", "attachment; filename=x.txt")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
	}
}

func TestThousandPartsAreNotTooMany(t *testing.T) {
	values := []string{"multipart/form-data; boundary=b"}
	body := strings.Repeat("--b\r\nContent-Disposition: form-data; name=\"f\"\r\n\r\nx\r\n", maxParts) + "--b--\r\n"
	if why := ambiguousFor(values, []byte(body)); why != "" {
		t.Fatalf("%q", why)
	}
}

func TestExcludeWithAPortIsRefused(t *testing.T) {
	config := CreateConfig()
	config.Clamd = "clamd:3310"
	config.Exclude = []string{"api.example.org:8443/ingest"}
	if _, err := New(context.Background(), http.NotFoundHandler(), config, "upload-scan"); err == nil {
		t.Fatal("an exclusion with a port was accepted")
	}
}

// Mid-line matches are skipped by the standard library: a large body of them is cut at once.
func TestMidLineMatchesAreSkippedQuickly(t *testing.T) {
	body := append(bytes.Repeat([]byte("x--b"), 10<<20/4), "\n--b\n"...)
	start := time.Now()
	at := nextDelimiter(body, []byte("--b"), 0)
	if took := time.Since(start); at != len(body)-4 || took > 500*time.Millisecond {
		t.Fatalf("found %d after %v", at, took)
	}
}

func TestNextDelimiterOnlyAtLineStarts(t *testing.T) {
	body := []byte("--b\nx--b\n--b--")
	for _, c := range []struct{ from, want int }{{0, 0}, {1, 9}, {9, 9}, {10, -1}, {len(body), -1}, {len(body) + 1, -1}} {
		if got := nextDelimiter(body, []byte("--b"), c.from); got != c.want {
			t.Errorf("from %d: %d, want %d", c.from, got, c.want)
		}
	}
}

// The check ends at the first closing delimiter, however many follow.
func TestClosingDelimitersEndTheAmbiguityCheck(t *testing.T) {
	values := []string{"multipart/form-data; boundary=b"}
	body := append([]byte("--b\r\nContent-Disposition: form-data; name=\"f\"\r\n\r\nx\r\n"), bytes.Repeat([]byte("--b--\r\n"), 10<<20/7)...)
	start := time.Now()
	why := ambiguousFor(values, body)
	if took := time.Since(start); why != "data after the closing boundary" || took > 500*time.Millisecond {
		t.Fatalf("%q after %v", why, took)
	}
}

func TestOtherMultipartTriesAnUnreachableClamdOnce(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	s := newSetup(t, addr, nil)
	body := []byte("--b\r\nContent-Type: text/plain\r\n\r\none\r\n--b\r\nContent-Type: text/plain\r\n\r\ntwo\r\n--b--\r\n")
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", "multipart/mixed; boundary=b")
	q := &queue{left: time.Second, ctx: context.Background()}

	res := s.handler.(*UploadScan).scanBody(req, q, read{data: body, size: int64(len(body))})

	if res.Files != 1 || res.Verdict != "error" {
		t.Fatalf("want one attempt, then stop: %+v", res)
	}
}

// PHP takes "multipart/form-data\t; ..." for some other type and leaves the body to the app.
func TestFormDataAsPHPReadsIt(t *testing.T) {
	cases := map[string]bool{
		"multipart/form-data; boundary=b":      true,
		"Multipart/Form-Data;boundary=b":       true,
		"multipart/form-data, boundary=b":      true,
		"multipart/form-data\t; boundary=b":    false,
		"multipart/form-data+json; boundary=b": false,
		"multipart/form-datax; boundary=b":     false,
		"multipart/mixed; boundary=b":          false,
	}
	for value, want := range cases {
		if got := isFormData([]string{value}); got != want {
			t.Errorf("%q: %v, want %v", value, got, want)
		}
	}
}
