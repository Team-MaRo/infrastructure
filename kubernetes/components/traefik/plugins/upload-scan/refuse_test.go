package uploadscan

import "testing"

// refusals is the table of refuse.go as the tests expect it.
var refusals = []struct {
	res    result
	status int
	retry  int
	reason string
}{
	{result{Verdict: "infected", Sig: "Eicar-Test-Signature"}, 422, 0, "infected"},
	{result{Verdict: "limits-exceeded", Sig: "Heuristics.Limits.Exceeded.MaxScanTime"}, 503, 30, "scan-time"},
	{result{Verdict: "limits-exceeded", Sig: "Heuristics.Limits.Exceeded.MaxFileSize"}, 422, 0, "limits-exceeded"},
	{result{Verdict: "clean", Ambiguous: "Content-Encoding gzip"}, 422, 0, "ambiguous"},
	{result{Verdict: "skipped:too-large"}, 413, 0, "too-large"},
	{result{Verdict: "skipped:busy"}, 503, 10, "busy"},
	{result{Verdict: "skipped:too-many"}, 429, 10, "too-many"},
	{result{Verdict: "incomplete"}, 503, 30, "scan-time"},
	{result{Verdict: "skipped:time"}, 503, 30, "scan-time"},
	{result{Verdict: "error"}, 503, 30, "unavailable"},
	{result{Verdict: "aborted"}, 400, 0, "incomplete-body"},
	{result{Verdict: "incomplete", Ambiguous: "more than 1000 parts"}, 422, 0, "ambiguous"},
	{result{Verdict: "error", Ambiguous: "Content-Encoding gzip"}, 422, 0, "ambiguous"},
	{result{Verdict: "infected", Sig: "Eicar-Test-Signature", Ambiguous: "Content-Encoding gzip"}, 422, 0, "infected"},
}

// Everything the scan looked at and did not call clean is refused, with its status, Retry-After
// and reason; a client that left gets Traefik's own 499 whatever the verdict.
func TestEveryVerdictButCleanIsRefused(t *testing.T) {
	for _, c := range refusals {
		r := refuse(c.res, false)
		if r == nil || r.status != c.status || r.retryAfter != c.retry || r.reason != c.reason {
			t.Errorf("%s (%s): %+v, want %d, Retry-After %d, %s", c.res.Verdict, c.res.Sig, r, c.status, c.retry, c.reason)
		}
		if gone := refuse(c.res, true); gone == nil || gone.status != statusClientGone {
			t.Errorf("%s with the client gone: %+v, want %d", c.res.Verdict, gone, statusClientGone)
		}
	}
	for _, res := range []result{{Verdict: "clean"}, {}} {
		if r := refuse(res, true); r != nil {
			t.Errorf("verdict %q refused: %+v", res.Verdict, r)
		}
	}
}

// The text names what the refusal is about: the signature clamd found, the size limit.
func TestRefusalNamesItsDetail(t *testing.T) {
	if r := refuse(result{Verdict: "infected", Sig: "Eicar-Test-Signature"}, false); r.detail != "Eicar-Test-Signature" {
		t.Errorf("infected names %q", r.detail)
	}
	setMaxBytes(t, 150000000)
	if r := refuse(result{Verdict: "skipped:too-large"}, false); r.detail != "150" {
		t.Errorf("too-large names %q", r.detail)
	}
}
