package uploadscan

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Waiting for memory counts against the queue time, so waiters do not line up.
func TestWaitersDoNotQueueBehindEachOther(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "300ms" })
	occupy(t, pool.size)

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", []byte("data"))
		}()
	}
	wg.Wait()
	if took := time.Since(start); took > 1200*time.Millisecond {
		t.Fatalf("five waiters took %v, each may wait 300ms at most", took)
	}
	for _, line := range s.logLines() {
		if line["verdict"] != "skipped:busy" {
			t.Fatalf("unexpected: %v", line)
		}
	}
}

func TestClientGoneEndsTheWait(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "5s" })
	for i := 0; i < cap(sessions); i++ {
		sessions <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(sessions); i++ {
			<-sessions
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", bytes.NewReader(eicar)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/zip")
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	if took := time.Since(start); took > time.Second {
		t.Fatalf("a gone client was waited for %v", took)
	}
	s.logLines()
}

func TestFreeSessionIsTakenWithTheQueueTimeUsedUp(t *testing.T) {
	clamd := startClamd(t, 0)
	newSetup(t, clamd.addr, nil)
	q := &queue{left: 0, ctx: context.Background()}
	for i := 0; i < 20; i++ {
		if !q.turn(sessions) {
			t.Fatal("a free session was not taken")
		}
		<-sessions
	}
}
