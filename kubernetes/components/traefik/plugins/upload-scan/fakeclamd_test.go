package uploadscan

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

// eicar is the antivirus test string, built from two halves so no file holds it whole - scanners
// on a developer's machine would flag this file otherwise.
var eicar = []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$` + `EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)

// fakeClamd answers INSTREAM like clamd: FOUND when the stream starts with the EICAR string, as
// ClamAV's signature requires. A delay makes it slow; it keeps every stream and its size.
type fakeClamd struct {
	addr  string
	delay time.Duration
	// closeAfter makes it close a session after that many streams (0: never).
	closeAfter int
	mu         sync.Mutex
	conns      int
	sizes      []int
	streams    [][]byte
	ended      []string // how each session ended: its last command, or "" when it was closed
}

// endings is how the sessions so far ended.
func (c *fakeClamd) endings() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.ended...)
}

// connections is how many connections clamd accepted.
func (c *fakeClamd) connections() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conns
}

// scans is how many streams clamd got, and their sizes.
func (c *fakeClamd) scans() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.sizes...)
}

// got is whether some stream to clamd was exactly b.
func (c *fakeClamd) got(b []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.streams {
		if bytes.Equal(s, b) {
			return true
		}
	}
	return false
}

func startClamd(t *testing.T, delay time.Duration) *fakeClamd {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	c := &fakeClamd{addr: ln.Addr().String(), delay: delay}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go c.serve(conn)
		}
	}()
	return c
}

// serve speaks a single INSTREAM, or a session (IDSESSION) of INSTREAMs up to END, answering each
// with its number in front.
func (c *fakeClamd) serve(conn net.Conn) {
	defer conn.Close()
	c.mu.Lock()
	c.conns++
	closeAfter := c.closeAfter
	c.mu.Unlock()
	in := bufio.NewReader(conn)
	cmd, err := in.ReadString(0)
	if err != nil {
		return
	}
	switch cmd {
	case "zINSTREAM\x00":
		if answer, ok := c.stream(in); ok {
			_, _ = conn.Write([]byte(answer + "\x00"))
		}
	case "zIDSESSION\x00":
		for id := 1; ; id++ {
			cmd, _ := in.ReadString(0)
			if cmd != "zINSTREAM\x00" {
				c.mu.Lock()
				c.ended = append(c.ended, cmd)
				c.mu.Unlock()
				return
			}
			answer, ok := c.stream(in)
			if !ok {
				return
			}
			_, _ = conn.Write([]byte(strconv.Itoa(id) + ": " + answer + "\x00"))
			if id == closeAfter {
				return
			}
		}
	}
}

// stream reads one INSTREAM's chunks, keeps them and gives clamd's answer.
func (c *fakeClamd) stream(in *bufio.Reader) (string, bool) {
	var data bytes.Buffer
	size := make([]byte, 4)
	for {
		if _, err := io.ReadFull(in, size); err != nil {
			return "", false
		}
		n := binary.BigEndian.Uint32(size)
		if n == 0 {
			break
		}
		if _, err := io.CopyN(&data, in, int64(n)); err != nil {
			return "", false
		}
	}
	c.mu.Lock()
	c.sizes = append(c.sizes, data.Len())
	if len(c.streams) < 1000 {
		c.streams = append(c.streams, append([]byte(nil), data.Bytes()...))
	}
	c.mu.Unlock()
	time.Sleep(c.delay)
	switch {
	case bytes.Contains(data.Bytes(), []byte("BEYOND-LIMITS")):
		return "stream: Heuristics.Limits.Exceeded.MaxFileSize FOUND", true
	case bytes.HasPrefix(data.Bytes(), eicar):
		return "stream: Eicar-Test-Signature FOUND", true
	}
	return "stream: OK", true
}
