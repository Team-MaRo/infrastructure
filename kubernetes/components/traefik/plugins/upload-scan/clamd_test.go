package uploadscan

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

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
	j := newScanning(context.Background(), time.Now().Add(time.Second))
	defer j.end()
	s.handler.(*UploadScan).scanMultipart(j, body.Bytes(), boundaries([]string{ct}), &res)
	if res.Files != 1 {
		t.Fatalf("want one attempt, then stop: %d", res.Files)
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

// manyParts is a multipart body of n file parts, the one at eicarAt (1-based, 0: none) holding
// EICAR.
func manyParts(n, eicarAt int) []byte {
	var b bytes.Buffer
	for i := 1; i <= n; i++ {
		b.WriteString("--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"f" + strconv.Itoa(i) + ".txt\"\r\n\r\n")
		if i == eicarAt {
			b.Write(eicar)
		} else {
			b.WriteString("file " + strconv.Itoa(i))
		}
		b.WriteString("\r\n")
	}
	b.WriteString("--b--\r\n")
	return b.Bytes()
}

// All parts of an upload go to clamd over one connection, a session, however many there are.
func TestPartsShareOneClamdConnection(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", manyParts(50, 48))

	if line := onlyLine(t, s); line["verdict"] != "infected" || line["parts"] != float64(48) || clamd.connections() != 1 {
		t.Fatalf("unexpected: %v over %d connections", line, clamd.connections())
	}
	until(t, "the session ended", func() bool { return len(clamd.endings()) == 1 })
	if end := clamd.endings()[0]; end != "zEND\x00" {
		t.Fatalf("the session ended with %q, not END", end)
	}
}

// A session clamd breaks off fails the part it was on; the next part opens a new one.
func TestBrokenSessionIsOpenedAgain(t *testing.T) {
	clamd := startClamd(t, 0)
	clamd.mu.Lock()
	clamd.closeAfter = 2
	clamd.mu.Unlock()
	s := newSetup(t, clamd.addr, nil)

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data; boundary=b", manyParts(4, 0))

	if line := onlyLine(t, s); line["verdict"] != "error" || !clamd.got([]byte("file 4")) || clamd.connections() != 2 {
		t.Fatalf("unexpected: %v over %d connections, last part scanned: %v", line, clamd.connections(), clamd.got([]byte("file 4")))
	}
}

// A client that leaves while clamd scans its part ends the scan at once: its session's connection
// is closed, not waited on.
func TestClientGoneClosesTheSession(t *testing.T) {
	clamd := startClamd(t, 5*time.Second)
	s := newSetup(t, clamd.addr, nil)
	ctx, cancel := context.WithCancel(context.Background())
	j := newScanning(ctx, time.Now().Add(10*time.Second))
	defer j.end()
	got := make(chan scanResult, 1)
	go func() { got <- s.handler.(*UploadScan).scan(j, bytes.NewReader([]byte("data"))) }()
	until(t, "clamd has the stream", func() bool { return len(clamd.scans()) == 1 })

	start := time.Now()
	cancel()
	answer := <-got
	if !answer.stopped || time.Since(start) > time.Second {
		t.Fatalf("the scan did not end with the client: %+v after %v", answer, time.Since(start))
	}
}
