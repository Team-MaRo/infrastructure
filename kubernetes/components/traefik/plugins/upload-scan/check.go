package uploadscan

import (
	"bytes"
	"fmt"
	"net/http"
	"time"
)

func (u *UploadScan) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !u.wanted(req) {
		u.next.ServeHTTP(rw, req)
		return
	}
	held := &heldBody{original: req.Body}
	req.Body = held
	defer held.release()
	start := time.Now()
	res := u.check(req, held)
	if res.Verdict != "" {
		u.log(req, res, time.Since(start))
	}
	u.next.ServeHTTP(rw, req)
}

// check reads the body, scans it and leaves it in held for the app. An empty Verdict means there
// was nothing to scan.
func (u *UploadScan) check(req *http.Request, held *heldBody) result {
	known := req.ContentLength
	if known > maxBytes {
		return result{Bytes: known, Verdict: "skipped:too-large"}
	}

	ctx := req.Context()
	q := &queue{left: u.queueTimeout, ctx: ctx}
	// An unknown length reserves the most a body may take, twice: its chunks are joined into one
	// buffer at the end. What it does not use goes back after reading.
	need := 2 * (chunksFor(maxBytes) + 1)
	if known > 0 {
		need = chunksFor(known)
	}
	if !q.reserve(need) {
		size := known
		if size < 0 {
			size = 0
		}
		return result{Bytes: size, Verdict: "skipped:busy"}
	}
	held.setTokens(need)

	got := readBody(held.original, known, maxBytes)
	held.hold(got.data, known < 0)
	switch got.verdict {
	case "aborted":
		return result{Bytes: got.size, Verdict: got.verdict, Error: got.err.Error()}
	case "skipped:too-large":
		return result{Bytes: got.size, Verdict: got.verdict}
	}

	if got.size == 0 {
		return result{}
	}
	return u.safeScan(req, q, got)
}

// safeScan keeps report mode's promise that every request passes: once the body is held for the
// app, a fault in looking at it or scanning it is logged instead of failing the request.
func (u *UploadScan) safeScan(req *http.Request, q *queue, got read) (res result) {
	defer func() {
		if p := recover(); p != nil {
			res = result{Bytes: got.size, Verdict: "error", Error: fmt.Sprint("panic: ", p)}
		}
	}()
	return u.scanBody(req, q, got)
}

func (u *UploadScan) scanBody(req *http.Request, q *queue, got read) result {
	ctx := req.Context()
	// Before taking a clamd session: looking at the body does not need clamd.
	res := result{Bytes: got.size}
	values := req.Header.Values("Content-Type")
	multipart := isMultipart(values)
	var bounds []string
	if multipart {
		bounds = boundaries(values)
		res.Ambiguous = ambiguous(req.Header, bounds, got.data)
	}

	if !q.session() {
		res.Verdict = "skipped:busy"
		return res
	}
	defer func() { <-sessions }()

	j := newScanning(ctx, time.Now().Add(u.scanTimeout))
	switch {
	case !multipart:
		u.scanWhole(j, got.data, &res)
	case len(bounds) == 0:
		res.note("multipart without boundary, scanned whole")
		u.scanWhole(j, got.data, &res)
	case !isFormData(values):
		// What the app gets as it is, then the parts an app may still cut it into.
		res.note("multipart other than form-data, scanned whole and in parts")
		if answer := u.scanWhole(j, got.data, &res); !stop(&res, answer) {
			u.scanMultipart(j, got.data, bounds, &res)
		}
	default:
		u.scanMultipart(j, got.data, bounds, &res)
	}
	res.finish()
	return res
}

func (u *UploadScan) scanWhole(j *scanning, body []byte, res *result) scanResult {
	res.Files++
	answer := u.scan(j, bytes.NewReader(body))
	u.record(res, answer)
	return answer
}

// finish gives a scanned request its verdict from what its parts said.
func (r *result) finish() {
	switch {
	case r.Stopped && r.Verdict != "infected":
		// Time used up or client gone with parts left: what was scanned is not the whole, whatever
		// the parts before said.
		if r.Verdict != "" {
			r.note("earlier parts: " + r.Verdict)
		}
		r.Verdict = "skipped:time"
		if r.answered() > 0 {
			r.Verdict = "incomplete"
		}
	case r.Files > 0 && r.Verdict == "":
		r.Verdict = "clean"
	}
}

// stop is whether scanning a request ends here: something found, clamd out of reach, the client
// gone or the time used up.
func stop(res *result, s scanResult) bool {
	return res.Verdict == "infected" || s.unreachable || s.stopped
}
