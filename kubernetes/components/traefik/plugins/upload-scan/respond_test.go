package uploadscan

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const browserAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

func answer(r *refusal, accept, acceptLanguage string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	rec := httptest.NewRecorder()
	respond(rec, req, r)
	return rec
}

func TestAnswerAsJSON(t *testing.T) {
	rec := answer(&refusal{status: 503, retryAfter: 10, reason: "busy"}, "application/json", "fr-CH")
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %q", rec.Body.String())
	}
	if rec.Code != 503 || rec.Header().Get("Retry-After") != "10" || rec.Header().Get("Content-Type") != "application/json" ||
		rec.Header().Get("Content-Language") != "fr" {
		t.Fatalf("status %d, headers %v", rec.Code, rec.Header())
	}
	if got["code"] != "busy" || got["retry_after"] != 10.0 || got["message"] != messages["fr"]["busy"] {
		t.Fatalf("unexpected answer: %v", got)
	}

	rec = answer(&refusal{status: 422, reason: "infected", detail: "Eicar-Test-Signature"}, "application/json", "")
	got = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %q", rec.Body.String())
	}
	if retry, ok := got["retry_after"]; !ok || retry != nil || rec.Header().Get("Retry-After") != "" {
		t.Fatalf("a final refusal offers a retry: %v, Retry-After %q", got, rec.Header().Get("Retry-After"))
	}
	if !strings.Contains(got["message"].(string), "Eicar-Test-Signature") {
		t.Fatalf("the signature is not named: %v", got)
	}
}

// The page's language follows the browser, and a signature clamd names cannot add markup.
func TestAnswerAsHTMLEscapesTheSignature(t *testing.T) {
	rec := answer(&refusal{status: 422, reason: "infected", detail: `<script>alert(1)</script>`}, browserAccept, "de-CH,de;q=0.9")
	body := rec.Body.String()
	if rec.Header().Get("Content-Type") != "text/html; charset=utf-8" || !strings.Contains(body, `lang="de"`) ||
		!strings.Contains(body, messages["de"]["title"]) {
		t.Fatalf("not the German page: %v\n%s", rec.Header(), body)
	}
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("the signature was not escaped:\n%s", body)
	}
}

func TestAnswerAsTextWithItsHeaders(t *testing.T) {
	rec := answer(&refusal{status: 503, retryAfter: 30, reason: "unavailable"}, "*/*", "")
	want := messages["en"]["unavailable"] + "\nPlease try again in 30 seconds.\n"
	if rec.Code != 503 || rec.Body.String() != want {
		t.Fatalf("%d %q, want %q", rec.Code, rec.Body.String(), want)
	}
	for header, value := range map[string]string{
		"Content-Type":           "text/plain; charset=utf-8",
		"Content-Language":       "en",
		"Vary":                   "Accept, Accept-Language",
		"Cache-Control":          "no-store",
		"X-Content-Type-Options": "nosniff",
		"Retry-After":            "30",
	} {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s: %q, want %q", header, got, value)
		}
	}
}

func TestClientGoneGetsTheStatusAlone(t *testing.T) {
	rec := answer(&refusal{status: statusClientGone}, browserAccept, "")
	if rec.Code != statusClientGone || rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "" {
		t.Fatalf("%d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
}
