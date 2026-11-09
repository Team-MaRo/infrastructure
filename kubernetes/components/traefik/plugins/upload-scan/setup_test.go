package uploadscan

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

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
	joining = make(chan struct{}, 1)
}

// occupy takes n chunks of the pool for the rest of the test.
func occupy(t *testing.T, n int) {
	t.Helper()
	pool.mu.Lock()
	if pool.used+n > pool.size {
		pool.mu.Unlock()
		t.Fatalf("could not take %d chunks", n)
	}
	pool.used += n
	pool.mu.Unlock()
	t.Cleanup(func() { pool.give(n) })
}

// stall holds back a stallingBody until the test ends, also when it fails early.
func stall(t *testing.T, b *stallingBody) func() {
	var once sync.Once
	release := func() { once.Do(func() { close(b.stalled) }) }
	t.Cleanup(release)
	return release
}

// until waits for cond, failing the test if it does not come true within 2 s.
func until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never: %s", what)
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

func mapValues(m map[string]interface{}) []string {
	var out []string
	for _, v := range m {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
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

func ambiguousFor(values []string, body []byte) string {
	return ambiguous(http.Header{"Content-Type": values}, boundaries(values), body)
}

// stallingBody sends its first bytes, then nothing until stalled is closed, then breaks off.
type stallingBody struct {
	first   []byte
	stalled chan struct{}
}

func (b *stallingBody) Read(p []byte) (int, error) {
	if len(b.first) > 0 {
		n := copy(p, b.first)
		b.first = b.first[n:]
		return n, nil
	}
	<-b.stalled
	return 0, io.ErrUnexpectedEOF
}
