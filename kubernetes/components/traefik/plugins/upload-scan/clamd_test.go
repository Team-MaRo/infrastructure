package uploadscan

import (
	"context"
	"net"
	"net/http"
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
	s.handler.(*UploadScan).scanMultipart(newScanning(context.Background(), time.Now().Add(time.Second)), body.Bytes(), boundaries([]string{ct}), &res)
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
