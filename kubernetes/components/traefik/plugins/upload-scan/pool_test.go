package uploadscan

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// What a body sets aside of its announced size follows its pace: all of it at first, then what it
// would send in the next promiseHorizon at its rate so far.
func TestPromiseFollowsThePace(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		in   intake
		want int
	}{
		"just admitted": {intake{need: 98, admitted: now.Add(-time.Second)}, 98},
		"1 MiB in 10 s": {intake{need: 98, admitted: now.Add(-10 * time.Second), received: chunkSize}, 3},
		"stalled":       {intake{need: 98, admitted: now.Add(-10 * time.Second)}, 0},
		"50 MiB in 5 s": {intake{need: 40, admitted: now.Add(-5 * time.Second), received: 50 * chunkSize}, 40},
	}
	for name, c := range cases {
		if got := c.in.counted(now); got != c.want {
			t.Errorf("%s: %d chunks set aside, want %d", name, got, c.want)
		}
	}
}

// A body admitted because its announced size fitted keeps that room: a chunked body, which
// announced one chunk, cannot take it.
func TestSetAsideRoomIsKeptFromOthers(t *testing.T) {
	p := &memoryPool{size: 10}
	now := time.Now()
	known := p.tryAdmit(8, p.promised(now), now)
	chunked := p.tryAdmit(1, p.promised(now), now)
	if known == nil || chunked == nil {
		t.Fatal("not admitted")
	}
	if !p.free(chunked, p.promised(now), now) {
		t.Fatal("the one chunk nobody set aside was not free for the chunked body")
	}
	p.used++
	chunked.took()
	if p.free(chunked, p.promised(now), now) {
		t.Fatal("the chunked body could take room set aside for the known one")
	}
	if !p.free(known, p.promised(now), now) {
		t.Fatal("the known body could not take its own room")
	}
}

// A body that stalls stops holding others back: once its pace shows, what it set aside shrinks and
// a body that did not fit next to it is admitted.
func TestStalledBodyStopsHoldingOthersBack(t *testing.T) {
	p := &memoryPool{size: 10}
	now := time.Now()
	large := p.tryAdmit(9, p.promised(now), now)
	if large == nil {
		t.Fatal("not admitted")
	}
	if p.tryAdmit(2, p.promised(now), now) != nil {
		t.Fatal("admitted into room set aside for a body that just started")
	}
	large.admitted = now.Add(-10 * time.Second)
	if p.tryAdmit(2, p.promised(now), now) == nil {
		t.Fatal("still held back by a body that has sent nothing for 10 s")
	}
}

// The body to give way is a waiting one beyond its announcement, even if older, else the newest
// waiting one; a body not waiting for the pool is never it.
func TestGiverIsBeyondItsAnnouncementElseTheNewest(t *testing.T) {
	beyond := &intake{need: 0, waiting: true}
	older := &intake{need: 3, waiting: true}
	newer := &intake{need: 3, waiting: true}
	reading := &intake{need: 0}
	p := &memoryPool{reading: []*intake{beyond, older, newer, reading}}
	if p.giver() != beyond {
		t.Fatal("the body beyond its announcement was not the one to give way")
	}
	p.reading = []*intake{older, newer, reading}
	if p.giver() != newer {
		t.Fatal("the newest waiting body was not the one to give way")
	}
}

// More uploads than fit at once: the later ones wait for room instead of starting and running out
// halfway, so every one is scanned.
func TestManyUploadsAtOnceAreAllScanned(t *testing.T) {
	setMaxBytes(t, 8<<20)
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	sent := bytes.Repeat([]byte("u"), 7<<20)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			serve(s, http.MethodPost, "https://site.example.org/upload/"+strconv.Itoa(i), "application/zip", sent)
		}(i)
	}
	wg.Wait()

	verdicts := map[interface{}]int{}
	for _, line := range s.logLines() {
		verdicts[line["verdict"]]++
	}
	if verdicts["clean"] != 8 || pool.inUse() != 0 {
		t.Fatalf("verdicts %v, %d chunks still in use", verdicts, pool.inUse())
	}
}

// When every body being read waits for a chunk, none could finish: the newest gives up, and the
// older one gets its chunk once the newest's come back.
func TestEveryReaderWaitingGivesUpTheNewest(t *testing.T) {
	p := &memoryPool{size: 2}
	q := func() *queue { return &queue{left: 3 * time.Second, ctx: context.Background()} }
	older := p.admit(q(), 1)
	newer := p.admit(q(), 1)
	if older == nil || newer == nil {
		t.Fatal("not admitted")
	}
	got := make(chan bool, 1)
	go func() { got <- p.more(older, q()) }()
	until(t, "the older body waits", func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return older.waiting
	})

	if p.more(newer, q()) {
		t.Fatal("the newest got a chunk although none was free")
	}
	p.done(newer)
	p.give(1)
	if !<-got {
		t.Fatal("the older body did not get the chunk the newest gave back")
	}
}

// A small upload is admitted while a large one waits for room for its announced size.
func TestSmallUploadPassesWhileALargeOneWaits(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, func(c *Config) { c.QueueTimeout = "2s" })
	occupy(t, pool.size-5) // room for a small upload, not for 100 MiB
	large := &stallingBody{first: []byte("x"), stalled: make(chan struct{})}
	release := stall(t, large)
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/large", large)
	req.ContentLength = maxBytes
	req.Header.Set("Content-Type", "application/zip")
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handler.ServeHTTP(httptest.NewRecorder(), req)
	}()
	until(t, "the large upload waits to be admitted", func() bool {
		pool.mu.Lock()
		defer pool.mu.Unlock()
		return len(pool.admitting) == 1
	})

	serve(s, http.MethodPost, "https://site.example.org/small", "application/zip", []byte("data"))
	pool.mu.Lock()
	largeWaits := len(pool.admitting) == 1
	pool.mu.Unlock()
	release()
	<-done

	verdicts := map[string]interface{}{}
	for _, line := range s.logLines() {
		verdicts[line["path"].(string)] = line["verdict"]
	}
	if verdicts["/small"] != "clean" || verdicts["/large"] != "skipped:busy" || !largeWaits {
		t.Fatalf("large upload still waiting when the small one was done: %v; verdicts %v", largeWaits, verdicts)
	}
}

// A body waiting alone for a chunk does not give way after patience: it gets the chunk once memory
// comes back within its queue time.
func TestLoneWaiterWaitsForMemory(t *testing.T) {
	p := &memoryPool{size: 2}
	q := &queue{left: 5 * time.Second, ctx: context.Background()}
	in := p.admit(q, 2)
	if in == nil {
		t.Fatal("not admitted")
	}
	p.mu.Lock()
	p.used++ // a finished body's chunk, not passed on yet
	p.mu.Unlock()
	got := make(chan bool, 1)
	go func() { got <- p.more(in, q) }()

	time.Sleep(2 * patience)
	p.give(1)
	if !<-got {
		t.Fatal("the body waiting alone gave way instead of waiting for memory")
	}
}

// Stuck bodies give way one at a time after patience, also while others hold memory - a finished
// body, a slow sender not waiting for the pool - and the older one gets its chunk once the newer
// one's come back.
func TestStuckBodiesGiveWayWhoeverElseHoldsMemory(t *testing.T) {
	now := time.Now()
	slow := &intake{need: 4, admitted: now.Add(-10 * time.Second), grant: make(chan bool, 1)} // sent nothing for 10 s
	older := &intake{need: 4, admitted: now, grant: make(chan bool, 1)}
	newer := &intake{need: 4, admitted: now, grant: make(chan bool, 1)}
	p := &memoryPool{size: 12, used: 12, reading: []*intake{slow, older, newer}}
	q := func() *queue { return &queue{left: 5 * time.Second, ctx: context.Background()} }
	olderGot := make(chan bool, 1)
	newerGot := make(chan bool, 1)
	start := time.Now()
	go func() { olderGot <- p.more(older, q()) }()
	go func() { newerGot <- p.more(newer, q()) }()

	if <-newerGot {
		t.Fatal("the newer body got a chunk although none was free")
	}
	if took := time.Since(start); took < patience/2 || took > 3*patience {
		t.Fatalf("the newer body gave way after %v, want about %v", took, patience)
	}
	p.done(newer)
	p.give(3)
	if !<-olderGot {
		t.Fatal("the older body did not get a chunk once the newer one's came back")
	}
}

// A waiting admission is let in once a body's reservation shrinks with time, without any memory
// coming back: the pool's timer looks again.
func TestAdmissionFollowsAShrinkingReservation(t *testing.T) {
	p := &memoryPool{size: 10}
	now := time.Now()
	large := p.tryAdmit(9, p.promised(now), now)
	if large == nil {
		t.Fatal("not admitted")
	}
	admitted := make(chan *intake, 1)
	go func() { admitted <- p.admit(&queue{left: 3 * time.Second, ctx: context.Background()}, 2) }()
	until(t, "the small body waits to be admitted", func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.admitting) == 1
	})

	p.mu.Lock()
	large.admitted = time.Now().Add(-10 * time.Second) // it has sent nothing for 10 s
	p.mu.Unlock()
	if <-admitted == nil {
		t.Fatal("not admitted by the pool's timer within the queue time")
	}
}
