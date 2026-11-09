package uploadscan

import (
	"net/http"
	"strings"
	"testing"
)

func TestLoggedTextsAreClipped(t *testing.T) {
	if got := clip(strings.Repeat("a", 5000)); len(got) > 210 {
		t.Fatalf("clip kept %d bytes", len(got))
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
