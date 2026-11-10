package uploadscan

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// A wait that ended because the client left is logged as aborted, wherever it waited.
const clientGone = "client gone while waiting"

func (u *UploadScan) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !u.wanted(req) {
		u.next.ServeHTTP(rw, req)
		return
	}
	held := &heldBody{original: req.Body, pool: pool, client: clientOf(req)}
	req.Body = held
	defer held.release()
	start := time.Now()
	res := u.check(req, held)
	var refused *refusal
	if u.mode == "block" {
		refused = refuse(res, req.Context().Err() != nil)
	}
	if res.Verdict != "" {
		u.log(req, res, refused, time.Since(start))
	}
	if refused != nil {
		respond(rw, req, refused)
		return
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
	if !uploads.enter(held.client) {
		res := result{Verdict: "skipped:too-many"}
		if known > 0 {
			res.Bytes = known
		}
		res.note(fmt.Sprintf("%d uploads from this address at once", maxPerClient))
		return res
	}
	defer uploads.leave(held.client)

	q := &queue{left: u.queueTimeout, ctx: req.Context()}
	got := readBody(held.original, known, q, held)
	held.hold(got.chunks, got.units)
	switch got.verdict {
	case "aborted":
		return result{Bytes: got.size, Verdict: got.verdict, Error: got.err.Error()}
	case "skipped:too-large":
		return result{Bytes: got.size, Verdict: got.verdict}
	case "skipped:busy", "skipped:too-many":
		if req.Context().Err() != nil {
			return result{Bytes: got.size, Verdict: "aborted", Error: clientGone}
		}
		res := result{Bytes: known, Verdict: got.verdict}
		if got.verdict == "skipped:too-many" {
			res.note("this address's uploads hold its share of the memory")
		}
		if known < 0 {
			res.Bytes = got.size
			res.note("length unknown: bytes read before memory ran out")
		}
		return res
	}

	if got.size == 0 {
		return result{}
	}
	return u.safeScan(req, q, got, held)
}

// safeScan turns a fault in looking at a held body or scanning it into the verdict error - passed
// in report mode, refused as unavailable in block mode - instead of failing the request.
func (u *UploadScan) safeScan(req *http.Request, q *queue, got read, held *heldBody) (res result) {
	defer func() {
		if p := recover(); p != nil {
			res = result{Bytes: got.size, Verdict: "error", Error: fmt.Sprint("panic: ", p)}
		}
	}()
	return u.scanBody(req, q, got, held)
}

// scanBody scans a body read in full: a multipart one joined into one buffer, any other straight
// from its chunks.
func (u *UploadScan) scanBody(req *http.Request, q *queue, got read, held *heldBody) result {
	ctx := req.Context()
	// Before taking a clamd slot: looking at the body does not need clamd.
	res := result{Bytes: got.size}
	values := req.Header.Values("Content-Type")
	multipart := isMultipart(values)
	var body []byte
	var bounds []string
	if multipart {
		bounds = boundaries(values)
		// Joined only to be cut: without a boundary the body is scanned whole, from its chunks.
		if len(bounds) > 0 {
			joined, ok := join(q, got.chunks, held)
			if !ok {
				if ctx.Err() != nil {
					res.Verdict = "aborted"
					res.Error = clientGone
					return res
				}
				res.note("no turn to be joined in time")
				res.Verdict = "skipped:busy"
				return res
			}
			body = joined
			got.chunks = [][]byte{joined}
		}
		res.Ambiguous = ambiguous(req.Header, bounds, body)
	} else {
		res.Ambiguous = encoded(req.Header)
	}

	if !q.turn(sessions) {
		res.Verdict = "skipped:busy"
		if ctx.Err() != nil {
			res.Verdict = "aborted"
			res.Error = clientGone
		}
		return res
	}
	defer func() { <-sessions }()

	j := newScanning(ctx, time.Now().Add(u.scanTimeout))
	defer j.end()
	switch {
	case !multipart:
		u.scanWhole(j, &chunkReader{chunks: got.chunks}, &res)
	case len(bounds) == 0:
		res.note("multipart without boundary, scanned whole")
		u.scanWhole(j, &chunkReader{chunks: got.chunks}, &res)
	case !isFormData(values):
		// What the app gets as it is, then the parts an app may still cut it into.
		res.note("multipart other than form-data, scanned whole and in parts")
		if answer := u.scanWhole(j, bytes.NewReader(body), &res); !stop(&res, answer) {
			u.scanMultipart(j, body, bounds, &res)
		}
	default:
		u.scanMultipart(j, body, bounds, &res)
	}
	res.finish()
	return res
}

func (u *UploadScan) scanWhole(j *scanning, body io.Reader, res *result) scanResult {
	res.Files++
	answer := u.scan(j, body)
	u.record(res, answer)
	return answer
}

// finish gives a scanned request its verdict from what its parts said.
func (r *result) finish() {
	decided := r.Verdict == "infected" || r.Verdict == "limits-exceeded" && !timedOut(r.Sig)
	switch {
	case r.Stopped && !decided:
		// Time used up or client gone with parts left: what was scanned is not the whole, whatever
		// the parts before said - unless one of them already decides the request.
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
