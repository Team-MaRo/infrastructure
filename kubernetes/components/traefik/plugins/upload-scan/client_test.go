package uploadscan

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The address Cloudflare passes on counts, else the connection's; IPv6 by its /64.
func TestClientOf(t *testing.T) {
	cases := []struct{ header, remote, want string }{
		{"203.0.113.7", "10.0.0.1:1234", "203.0.113.7"},
		{" 198.51.100.4 ", "10.0.0.1:1234", "198.51.100.4"},
		{"not an address", "10.0.0.1:1234", "10.0.0.1"},
		{"", "[2001:db8:1:2:aaaa::1]:443", "2001:db8:1:2::/64"},
		{"2001:db8:1:2:bbbb::9", "10.0.0.1:1234", "2001:db8:1:2::/64"},
		{"2001:db8:1:3::1", "10.0.0.1:1234", "2001:db8:1:3::/64"},
		{"", "[fe80::1%eth0]:51234", "fe80::1%eth0"}, // unparsable: its host, not each connection
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", nil)
		req.RemoteAddr = c.remote
		if c.header != "" {
			req.Header.Set("CF-Connecting-IP", c.header)
		}
		if got := clientOf(req); got != c.want {
			t.Errorf("header %q, remote %q: %q, want %q", c.header, c.remote, got, c.want)
		}
	}
}

func setMaxPerClient(t *testing.T, n int) {
	old := maxPerClient
	maxPerClient = n
	t.Cleanup(func() { maxPerClient = old })
}

func inFlight(client string) int {
	uploads.mu.Lock()
	defer uploads.mu.Unlock()
	return uploads.inFlight[client]
}

// One upload more than an address may have at once is not read and passes as it came; another
// address is not held back.
func TestUploadsBeyondTheAddressLimitAreTooMany(t *testing.T) {
	setMaxPerClient(t, 2)
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	done := make(chan struct{}, 2)
	var releases []func()
	for i := 0; i < 2; i++ {
		body := &stallingBody{first: []byte("x"), stalled: make(chan struct{})}
		releases = append(releases, stall(t, body))
		req := httptest.NewRequest(http.MethodPost, "https://site.example.org/stalled", body)
		req.ContentLength = maxBytes
		req.Header.Set("Content-Type", "application/zip")
		go func() {
			s.handler.ServeHTTP(httptest.NewRecorder(), req)
			done <- struct{}{}
		}()
	}
	until(t, "two uploads from the address are being checked", func() bool { return inFlight("192.0.2.1") == 2 })

	sent := []byte("third upload")
	serve(s, http.MethodPost, "https://site.example.org/third", "application/zip", sent)
	third := *s.got
	other := httptest.NewRequest(http.MethodPost, "https://site.example.org/other", bytes.NewReader([]byte("data")))
	other.Header.Set("Content-Type", "application/zip")
	other.Header.Set("CF-Connecting-IP", "198.51.100.9")
	s.handler.ServeHTTP(httptest.NewRecorder(), other)
	for _, release := range releases {
		release()
	}
	<-done
	<-done

	verdicts := map[string]interface{}{}
	for _, line := range s.logLines() {
		verdicts[line["path"].(string)] = line["verdict"]
	}
	if verdicts["/third"] != "skipped:too-many" || !bytes.Equal(third, sent) || verdicts["/other"] != "clean" {
		t.Fatalf("verdicts %v, the app got %q", verdicts, third)
	}
}

// An address's places come back after every request, also one that broke off.
func TestAddressPlacesComeBack(t *testing.T) {
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", []byte("data"))
	req := httptest.NewRequest(http.MethodPost, "https://site.example.org/upload", bytes.NewReader([]byte("short")))
	req.ContentLength = 100
	req.Header.Set("Content-Type", "application/zip")
	s.handler.ServeHTTP(httptest.NewRecorder(), req)

	s.logLines()
	if n := inFlight("192.0.2.1"); n != 0 {
		t.Fatalf("%d places still taken", n)
	}
	if n := addressesHolding(); n != 0 || pool.inUse() != 0 {
		t.Fatalf("%d addresses still hold memory, %d units in use", n, pool.inUse())
	}
}

func addressesHolding() int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return len(pool.byClient)
}

// Two uploads from one address that together exceed its share are both scanned: the second waits
// for the first's memory.
func TestTwoLargeUploadsFromOneAddressAreBothScanned(t *testing.T) {
	setMaxBytes(t, 8<<20) // a share of 9 MiB
	clamd := startClamd(t, 0)
	s := newSetup(t, clamd.addr, nil)
	sent := bytes.Repeat([]byte("v"), 7<<20)
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			serve(s, http.MethodPost, "https://site.example.org/upload", "application/zip", sent)
			done <- struct{}{}
		}()
	}
	<-done
	<-done

	verdicts := map[interface{}]int{}
	for _, line := range s.logLines() {
		verdicts[line["verdict"]]++
	}
	if verdicts["clean"] != 2 || addressesHolding() != 0 {
		t.Fatalf("verdicts %v, %d addresses still hold memory", verdicts, addressesHolding())
	}
}

// An address holds at most its share of the memory: a body over it waits and ends too-many, while
// a body from another address still gets in.
func TestAddressShareOfMemory(t *testing.T) {
	p := &memoryPool{size: 40, perClient: 10}
	q := func(left time.Duration) *queue { return &queue{left: left, ctx: context.Background()} }
	a := p.admit(q(time.Second), "a", 10)
	if a == nil || !p.more(a, q(time.Second), 9) {
		t.Fatal("the first address did not get its share")
	}
	over := q(100 * time.Millisecond)
	if p.more(a, over, 1) || over.busy() != "skipped:too-many" {
		t.Fatalf("the first address got more than its share, or was not told so (%s)", over.busy())
	}
	if p.admit(q(time.Second), "b", 5) == nil {
		t.Fatal("another address was held back by the first one's share")
	}
}

// What an address's bodies set aside counts toward its share too: it cannot announce more than its
// share, then hold the pool's room with bodies that send nothing.
func TestAddressCannotAnnounceMoreThanItsShare(t *testing.T) {
	p := &memoryPool{size: 40, perClient: 10}
	q := func(left time.Duration) *queue { return &queue{left: left, ctx: context.Background()} }
	if p.admit(q(time.Second), "a", 10) == nil {
		t.Fatal("the first body of an address was not admitted")
	}
	second := q(100 * time.Millisecond)
	if p.admit(second, "a", 10) != nil || second.busy() != "skipped:too-many" {
		t.Fatalf("an address set aside more than its share, or was not told so (%s)", second.busy())
	}
	if p.admit(q(time.Second), "b", 10) == nil {
		t.Fatal("another address was held back")
	}
}

// A body its address's share holds back waits out its own time; it does not make another address's
// body give way, which then waits alone and gets its chunk.
func TestCappedBodyDoesNotMakeOthersGiveWay(t *testing.T) {
	p := &memoryPool{size: 20, perClient: 10}
	q := func(left time.Duration) *queue { return &queue{left: left, ctx: context.Background()} }
	a := p.admit(q(time.Second), "a", 1)
	if a == nil || !p.more(a, q(time.Second), 9) {
		t.Fatal("the first address did not get its share")
	}
	aGot := make(chan bool, 1)
	aQueue := q(2 * time.Second)
	go func() { aGot <- p.more(a, aQueue, 1) }()
	b := p.admit(q(time.Second), "b", 1)
	if b == nil {
		t.Fatal("the second address was not admitted")
	}
	p.mu.Lock()
	p.used = p.size // the pool full: b waits for the pool, a for its share
	p.mu.Unlock()
	bGot := make(chan bool, 1)
	go func() { bGot <- p.more(b, q(5*time.Second), 1) }()

	time.Sleep(patience + patience/2)
	p.give("", 1)
	if !<-bGot {
		t.Fatal("the body waiting for the pool gave way because of a body over its address's share")
	}
	p.give("", 5) // room in the pool: only its share holds a back
	if <-aGot || aQueue.busy() != "skipped:too-many" {
		t.Fatalf("the body over its share got memory, or was not told so (%s)", aQueue.busy())
	}
}
