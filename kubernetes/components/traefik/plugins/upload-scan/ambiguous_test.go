package uploadscan

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"
)

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

func TestCompressedMultipartIsAmbiguous(t *testing.T) {
	values := []string{"multipart/form-data; boundary=b"}
	body := []byte("--b\r\nContent-Disposition: form-data; name=\"f\"; filename=\"e\"\r\n\r\nx\r\n--b--\r\n")
	header := http.Header{"Content-Type": values, "Content-Encoding": {"gzip"}}
	if why := ambiguous(header, boundaries(values), body); !strings.Contains(why, "Content-Encoding") {
		t.Fatalf("%q", why)
	}
	for _, none := range []string{"identity", "", " "} {
		header["Content-Encoding"] = []string{none}
		if why := ambiguous(header, boundaries(values), body); why != "" {
			t.Fatalf("%q is no encoding: %q", none, why)
		}
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
