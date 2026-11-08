package uploadscan

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type scanResult struct {
	sig         string
	err         error
	unreachable bool
	// The client left, or the request's time ran out: scanning stops, which is not clamd's fault.
	stopped bool
}

// scanning is one request's scan: its client, its time and one buffer for all its parts.
type scanning struct {
	ctx      context.Context
	deadline time.Time
	frame    []byte
}

func newScanning(ctx context.Context, deadline time.Time) *scanning {
	return &scanning{ctx: ctx, deadline: deadline, frame: make([]byte, 4+64<<10)}
}

// record keeps the worst verdict of a request: infected over limits-exceeded over error over
// clean. Heuristics.Limits.Exceeded is clamd's answer for a file beyond its own limits.
func (u *UploadScan) record(res *result, s scanResult) {
	if s.err != nil || s.stopped {
		res.Unanswered++
	}
	switch {
	case s.stopped:
		res.Stopped = true
		res.note("time used up or client gone after " + strconv.Itoa(res.answered()) + " parts")
	case s.sig != "" && !strings.HasPrefix(s.sig, "Heuristics.Limits.Exceeded"):
		res.Verdict = "infected"
		res.Sig = s.sig
	case s.sig != "" && res.Verdict != "infected":
		res.Verdict = "limits-exceeded"
		res.Sig = s.sig
	case s.err != nil && res.Verdict != "infected" && res.Verdict != "limits-exceeded":
		res.Verdict = "error"
		res.Error = s.err.Error()
	}
}

// scan sends one file to clamd (INSTREAM) and reads its answer: "stream: OK" or
// "stream: <signature> FOUND".
func (u *UploadScan) scan(j *scanning, r io.Reader) scanResult {
	ctx := j.ctx
	deadline := j.deadline
	if ctx.Err() != nil {
		return scanResult{err: errors.New("client gone"), stopped: true}
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return scanResult{err: errors.New("scan time used up"), stopped: true}
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	if remaining < dialer.Timeout {
		dialer.Timeout = remaining
	}
	conn, err := dialer.DialContext(ctx, "tcp", u.clamd)
	if err != nil {
		if ctx.Err() != nil {
			return scanResult{err: errors.New("client gone"), stopped: true}
		}
		// The budget ran out while connecting: not clamd's fault.
		if !time.Now().Before(deadline) {
			return scanResult{err: errors.New("scan time used up"), stopped: true}
		}
		return scanResult{err: fmt.Errorf("connecting to clamd: %w", err), unreachable: true}
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return scanResult{err: err}
	}
	// A client that hangs up ends the scan: closing the connection frees clamd's session.
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-finished:
		}
	}()

	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return u.failed(j, fmt.Errorf("writing to clamd: %w", err))
	}
	// Each chunk goes out with its 4-byte length in front, in one write.
	frame := j.frame
	for {
		n, readErr := r.Read(frame[4:])
		if n > 0 {
			binary.BigEndian.PutUint32(frame, uint32(n))
			if _, err := conn.Write(frame[:4+n]); err != nil {
				return u.failed(j, fmt.Errorf("writing to clamd: %w", err))
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return scanResult{err: fmt.Errorf("reading the file: %w", readErr)}
		}
	}
	binary.BigEndian.PutUint32(frame, 0)
	if _, err := conn.Write(frame[:4]); err != nil {
		return u.failed(j, fmt.Errorf("writing to clamd: %w", err))
	}

	reply, err := bufio.NewReader(conn).ReadString(0)
	if err != nil {
		return u.failed(j, fmt.Errorf("reading clamd's answer: %w", err))
	}
	reply = strings.TrimSuffix(reply, "\x00")
	switch {
	case strings.HasSuffix(reply, " OK"):
		return scanResult{}
	case strings.HasSuffix(reply, " FOUND"):
		return scanResult{sig: strings.TrimSuffix(strings.TrimPrefix(reply, "stream: "), " FOUND")}
	default:
		return scanResult{err: fmt.Errorf("clamd: %s", reply)}
	}
}

// failed is a scan that broke off: because the client hung up (its connection was closed for it),
// because the request's time ran out, or a fault of clamd's or the connection's.
func (u *UploadScan) failed(j *scanning, err error) scanResult {
	if j.ctx.Err() != nil {
		return scanResult{err: errors.New("client gone"), stopped: true}
	}
	if !time.Now().Before(j.deadline) {
		return scanResult{err: err, stopped: true}
	}
	return scanResult{err: err}
}
