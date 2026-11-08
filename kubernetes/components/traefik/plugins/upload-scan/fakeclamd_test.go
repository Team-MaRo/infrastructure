package uploadscan

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
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
	addr    string
	delay   time.Duration
	mu      sync.Mutex
	sizes   []int
	streams [][]byte
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

func (c *fakeClamd) serve(conn net.Conn) {
	defer conn.Close()
	cmd := make([]byte, len("zINSTREAM\x00"))
	if _, err := io.ReadFull(conn, cmd); err != nil || string(cmd) != "zINSTREAM\x00" {
		return
	}
	var data bytes.Buffer
	size := make([]byte, 4)
	for {
		if _, err := io.ReadFull(conn, size); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(size)
		if n == 0 {
			break
		}
		if _, err := io.CopyN(&data, conn, int64(n)); err != nil {
			return
		}
	}
	c.mu.Lock()
	c.sizes = append(c.sizes, data.Len())
	if len(c.streams) < 1000 {
		c.streams = append(c.streams, append([]byte(nil), data.Bytes()...))
	}
	c.mu.Unlock()
	time.Sleep(c.delay)
	if bytes.Contains(data.Bytes(), []byte("BEYOND-LIMITS")) {
		_, _ = conn.Write([]byte("stream: Heuristics.Limits.Exceeded.MaxFileSize FOUND\x00"))
		return
	}
	if bytes.HasPrefix(data.Bytes(), eicar) {
		_, _ = conn.Write([]byte("stream: Eicar-Test-Signature FOUND\x00"))
		return
	}
	_, _ = conn.Write([]byte("stream: OK\x00"))
}
