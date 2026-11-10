package uploadscan

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
		{http.MethodPost, "https://matomo.example.org/matomo.php?idsite=1", "application/octet-stream"},
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

// Every type skipped when small, by default.
func skipTypes() []string {
	return append(CreateConfig().SkipContentTypes, "application/vnd.api+json")
}

// A small Next.js server action is skipped like a form post; plain text without its header, or a
// large one, is scanned.
func TestOnlyASmallServerActionSkipsPlainText(t *testing.T) {
	clamd := startClamd(t, 0)
	large := append(append([]byte{}, eicar...), bytes.Repeat([]byte(" "), 70<<10)...)
	cases := []struct {
		name    string
		action  bool
		body    []byte
		scanned bool
	}{
		{"server action", true, eicar, false},
		{"plain text file", false, eicar, true},
		{"large server action", true, large, true},
	}
	for _, c := range cases {
		s := newSetup(t, clamd.addr, nil)
		req := httptest.NewRequest(http.MethodPost, "https://auth.example.org/ui/v2/login/password", bytes.NewReader(c.body))
		req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
		if c.action {
			req.Header.Set("Next-Action", "7f3a9c")
		}

		s.handler.ServeHTTP(httptest.NewRecorder(), req)

		if got := len(s.logLines()) == 1; got != c.scanned {
			t.Errorf("%s: scanned=%v, want %v", c.name, got, c.scanned)
		}
	}
}

func TestSkipTypeDoesNotHideAFile(t *testing.T) {
	clamd := startClamd(t, 0)
	for _, ct := range skipTypes() {
		s := newSetup(t, clamd.addr, nil)
		req := httptest.NewRequest(http.MethodPost, "https://site.example.org/wp-json/wp/v2/media", bytes.NewReader(eicar))
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Content-Disposition", "attachment; filename=x.pdf")

		s.handler.ServeHTTP(httptest.NewRecorder(), req)

		if line := onlyLine(t, s); line["verdict"] != "infected" {
			t.Fatalf("%s: unexpected: %v", ct, line)
		}
	}
}

func TestLargeSkipTypeBodyIsScanned(t *testing.T) {
	clamd := startClamd(t, 0)
	sent := append([]byte{}, eicar...)
	sent = append(sent, bytes.Repeat([]byte(" "), 70<<10)...)
	for _, ct := range skipTypes() {
		s := newSetup(t, clamd.addr, nil)

		serve(s, http.MethodPost, "https://site.example.org/form", ct, sent)

		if len(s.logLines()) != 1 {
			t.Fatalf("%s: a body larger than a form must be scanned whatever its type", ct)
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

func TestAnyMethodWithABodyIsScanned(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)

	serve(s, http.MethodDelete, "https://site.example.org/api/files", "application/octet-stream", eicar)

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
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

func TestFormDataPlusJSONIsNotSkipped(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	body := "--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e\"\r\n\r\n" + string(eicar) + "\r\n--b--\r\n"

	serve(s, http.MethodPost, "https://site.example.org/upload", "multipart/form-data+json; boundary=b", []byte(body))

	if line := onlyLine(t, s); line["verdict"] != "infected" {
		t.Fatalf("unexpected: %v", line)
	}
}
