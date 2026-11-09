package uploadscan

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

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
