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

// scanning is one request's scan: its client, its time, one buffer for all its parts, and one
// clamd session (IDSESSION) for all of them, opened with the first part.
type scanning struct {
	ctx      context.Context
	deadline time.Time
	frame    []byte
	conn     net.Conn
	reply    *bufio.Reader
	sent     int // commands sent in the session, the id of clamd's next answer
	stop     chan struct{}
}

func newScanning(ctx context.Context, deadline time.Time) *scanning {
	return &scanning{ctx: ctx, deadline: deadline, frame: make([]byte, 4+64<<10)}
}

// end ends a session that is still in order, if one is open.
func (j *scanning) end() {
	if j.conn != nil {
		_, _ = j.conn.Write([]byte("zEND\x00"))
	}
	j.drop()
}

// drop closes the session without a word, as after a fault: END could land in a stream cut off.
func (j *scanning) drop() {
	if j.conn == nil {
		return
	}
	j.conn.Close()
	close(j.stop)
	j.conn = nil
	j.reply = nil
}

// record keeps the worst verdict of a request: infected over limits-exceeded over error over
// clean. Heuristics.Limits.Exceeded is clamd's answer for a file beyond its own limits; of those,
// one the file exceeds is kept over its scan time running out, which a retry may not hit.
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
		if res.Sig == "" || timedOut(res.Sig) {
			res.Sig = s.sig
		}
	case s.err != nil && res.Verdict != "infected" && res.Verdict != "limits-exceeded":
		res.Verdict = "error"
		res.Error = s.err.Error()
	}
}

// scan sends one file to clamd over the request's session (INSTREAM) and reads its answer:
// "<id>: stream: OK" or "<id>: stream: <signature> FOUND". A session that breaks is closed; the
// next part opens a new one.
func (u *UploadScan) scan(j *scanning, r io.Reader) scanResult {
	if j.ctx.Err() != nil {
		return scanResult{err: errors.New("client gone"), stopped: true}
	}
	if time.Until(j.deadline) <= 0 {
		return scanResult{err: errors.New("scan time used up"), stopped: true}
	}
	if j.conn == nil {
		if failure := u.open(j); failure != nil {
			return *failure
		}
	}
	answer := u.stream(j, r)
	if answer.err != nil {
		j.drop()
	}
	return answer
}

// open starts the request's clamd session; nil when it is open.
func (u *UploadScan) open(j *scanning) *scanResult {
	// Within the request's time, also if that runs out between the check before and here.
	ctx, cancel := context.WithDeadline(j.ctx, j.deadline)
	defer cancel()
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", u.clamd)
	if err != nil {
		if j.ctx.Err() != nil {
			return &scanResult{err: errors.New("client gone"), stopped: true}
		}
		// The budget ran out while connecting: not clamd's fault.
		if !time.Now().Before(j.deadline) {
			return &scanResult{err: errors.New("scan time used up"), stopped: true}
		}
		return &scanResult{err: fmt.Errorf("connecting to clamd: %w", err), unreachable: true}
	}
	if err := conn.SetDeadline(j.deadline); err != nil {
		conn.Close()
		return &scanResult{err: err}
	}
	if _, err := conn.Write([]byte("zIDSESSION\x00")); err != nil {
		conn.Close()
		failure := u.failed(j, fmt.Errorf("writing to clamd: %w", err))
		return &failure
	}
	// A client that hangs up ends the scan: closing the connection frees clamd's session.
	stop := make(chan struct{})
	go func() {
		select {
		case <-j.ctx.Done():
			conn.Close()
		case <-stop:
		}
	}()
	j.conn = conn
	j.reply = bufio.NewReader(conn)
	j.sent = 0
	j.stop = stop
	return nil
}

// stream sends one file as an INSTREAM command of the open session and reads its answer.
func (u *UploadScan) stream(j *scanning, r io.Reader) scanResult {
	conn := j.conn
	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return u.failed(j, fmt.Errorf("writing to clamd: %w", err))
	}
	j.sent++
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

	reply, err := j.reply.ReadString(0)
	if err != nil {
		return u.failed(j, fmt.Errorf("reading clamd's answer: %w", err))
	}
	reply = strings.TrimSuffix(reply, "\x00")
	id := strconv.Itoa(j.sent) + ": "
	if !strings.HasPrefix(reply, id) {
		return scanResult{err: fmt.Errorf("clamd answered %q, expected id %d", reply, j.sent)}
	}
	reply = strings.TrimPrefix(reply, id)
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
