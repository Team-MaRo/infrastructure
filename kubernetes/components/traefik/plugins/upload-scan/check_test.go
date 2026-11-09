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
