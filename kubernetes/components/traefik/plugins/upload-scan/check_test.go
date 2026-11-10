package uploadscan

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

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

func TestBodyWithoutTheBoundaryIsScannedWhole(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", eicar)

	line := onlyLine(t, s)
	if line["verdict"] != "infected" || !strings.Contains(line["note"].(string), "scanned whole") {
		t.Fatalf("unexpected: %v", line)
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

// A part beyond ClamAV's limits decides the request, also when time then runs out with parts left;
// a part whose scan time ran out does not, so the request is incomplete.
func TestStoppedAfterLimitsExceeded(t *testing.T) {
	u := &UploadScan{}
	for sig, want := range map[string]string{
		"Heuristics.Limits.Exceeded.MaxFiles":    "limits-exceeded",
		"Heuristics.Limits.Exceeded.MaxScanTime": "incomplete",
	} {
		res := result{Files: 2, Verdict: "limits-exceeded", Sig: sig}
		u.record(&res, scanResult{stopped: true})
		res.finish()
		if res.Verdict != want || res.answered() != 1 || !strings.Contains(res.Note, "time used up") {
			t.Errorf("%s: %+v, want %s", sig, res, want)
		}
	}
}

// A limit the file itself exceeds outweighs a scan time running out, whichever part came first:
// a retry would fail on it again.
func TestExceededLimitOutweighsScanTime(t *testing.T) {
	u := &UploadScan{}
	exceeded := scanResult{sig: "Heuristics.Limits.Exceeded.MaxFileSize"}
	timedOut := scanResult{sig: "Heuristics.Limits.Exceeded.MaxScanTime"}
	for _, order := range [][]scanResult{{exceeded, timedOut}, {timedOut, exceeded}} {
		var res result
		for _, s := range order {
			res.Files++
			u.record(&res, s)
		}
		res.finish()
		if res.Verdict != "limits-exceeded" || res.Sig != exceeded.sig {
			t.Errorf("%s then %s: %+v", order[0].sig, order[1].sig, res)
		}
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

// In block mode a request the scan does not call clean gets its refusal, logged, and never reaches
// the app; a clean one does.
func TestBlockModeRefusesWhatIsNotClean(t *testing.T) {
	clamd := startClamd(t, 0)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	unreachable := ln.Addr().String()
	ln.Close()
	ambiguousBody, ambiguousType := multipartBody(t, map[string][]byte{"a.txt": []byte("hello")}, nil)
	var manyParts strings.Builder
	for i := 0; i < 1005; i++ {
		manyParts.WriteString("--b\r\nContent-Disposition: form-data; name=\"f" + strconv.Itoa(i) + "\"\r\n\r\nx\r\n")
	}
	manyParts.WriteString("--b--\r\n")
	cases := []struct {
		name, clamd, contentType, encoding string
		body                               []byte
		before                             func(t *testing.T)
		during                             func(t *testing.T)
		status                             int
		retryAfter                         string
	}{
		{"infected", clamd.addr, "application/pdf", "", eicar, nil, nil, 422, ""},
		{"clean", clamd.addr, "application/zip", "", []byte("data"), nil, nil, 200, ""},
		{"clamd unreachable", unreachable, "application/zip", "", []byte("data"), nil, nil, 503, "30"},
		{"ambiguous", clamd.addr, ambiguousType, "gzip", ambiguousBody.Bytes(), nil, nil, 422, ""},
		{"compressed raw body", clamd.addr, "application/octet-stream", "br", []byte("data"), nil, nil, 422, ""},
		{"more parts than scanned", clamd.addr, "multipart/form-data; boundary=b", "", []byte(manyParts.String()), nil, nil, 422, ""},
		{"too large", clamd.addr, "application/zip", "", bytes.Repeat([]byte("y"), 100), func(t *testing.T) { setMaxBytes(t, 10) }, nil, 413, ""},
		{"too many", clamd.addr, "application/zip", "", []byte("data"), func(t *testing.T) { setMaxPerClient(t, 0) }, nil, 429, "10"},
		{"busy", clamd.addr, "application/zip", "", []byte("data"), nil, func(t *testing.T) {
			for i := 0; i < cap(sessions); i++ {
				sessions <- struct{}{}
			}
		}, 503, "10"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.before != nil {
				c.before(t)
			}
			s := newSetup(t, c.clamd, func(config *Config) {
				config.Mode = "block"
				config.QueueTimeout = "50ms"
			})
			if c.during != nil {
				c.during(t)
			}
			req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", bytes.NewReader(c.body))
			req.Header.Set("Content-Type", c.contentType)
			if c.encoding != "" {
				req.Header.Set("Content-Encoding", c.encoding)
			}
			rec := httptest.NewRecorder()
			s.handler.ServeHTTP(rec, req)

			line := onlyLine(t, s)
			refused := c.status != 200
			if rec.Code != c.status || rec.Header().Get("Retry-After") != c.retryAfter || *s.reached == refused {
				t.Errorf("%d, Retry-After %q, app reached %v; log %v", rec.Code, rec.Header().Get("Retry-After"), *s.reached, line)
			}
			if line["upload_scan"] != "block" || refused && line["refused"] != float64(c.status) || !refused && line["refused"] != nil {
				t.Errorf("log line %v", line)
			}
		})
	}
}

// A client that left before the scan's answer gets the status Traefik gives it, 499, and nothing
// else; the app is not reached.
func TestBlockModeClientGoneGets499(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(config *Config) { config.Mode = "block" })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", bytes.NewReader(eicar)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/pdf")
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)

	if line := onlyLine(t, s); rec.Code != statusClientGone || rec.Body.Len() != 0 || *s.reached || line["refused"] != 499.0 {
		t.Fatalf("%d %q, app reached %v, log %v", rec.Code, rec.Body.String(), *s.reached, line)
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

	res := s.handler.(*UploadScan).scanBody(req, q, read{chunks: [][]byte{body}, size: int64(len(body))}, &heldBody{})

	if res.Files != 1 || res.Verdict != "error" {
		t.Fatalf("want one attempt, then stop: %+v", res)
	}
}

// A raw body is scanned from its chunks, byte for byte, without being joined.
func TestRawBodyReachesClamdFromItsChunks(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	sent := make([]byte, 2*chunkSize+12345)
	for i := range sent {
		sent[i] = byte(i * 7)
	}
	req := httptest.NewRequest(http.MethodPut, "https://site.example.org/upload", io.NopCloser(bytes.NewReader(sent)))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")

	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if line := onlyLine(t, s); line["verdict"] != "clean" || !clamd.got(sent) || !bytes.Equal(*s.got, sent) {
		t.Fatalf("unexpected: %v, clamd got it exactly: %v", line, clamd.got(sent))
	}
}
