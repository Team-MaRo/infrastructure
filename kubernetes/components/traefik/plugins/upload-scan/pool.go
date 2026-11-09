package uploadscan

import (
	"io"
	"sync"
	"time"
)

// How often a body tells the pool what it has received, for its pace.
const reportEvery = 64 << 10

// A body's announced size is set aside for it in proportion to its pace: what it would send within
// promiseHorizon at its rate so far - all of it during its first rateGrace, before there is a rate.
const (
	promiseHorizon = 30 * time.Second
	rateGrace      = 3 * time.Second
	recheckEvery   = 500 * time.Millisecond
	// A body waiting this long for a chunk while another body being read waits too gives up if it
	// is the one to give way (giver), so stuck bodies free memory one at a time.
	patience = time.Second
)

// memoryPool hands out memory in units (64 KiB) as bodies are read: it admits a body once its
// announced size fits next to what the bodies being read have set aside at their pace, gives freed
// units to the bodies waiting for a chunk, oldest first, and lets stuck bodies give way one at a
// time; an address holds at most perClient units (0: no limit) - the rules and their reasons in
// kubernetes/components/traefik/README.md, "Upload scan", Memory.
type memoryPool struct {
	mu        sync.Mutex
	size      int
	used      int
	perClient int
	byClient  map[string]int
	reading   []*intake // oldest first
	admitting []*admission
	ticking   bool
	gaveWay   time.Time // when a body last gave way: at most one per patience
}

// intake is a body being read: its address, the units it announced but has not taken yet, what it
// has sent since it was admitted, the time it spent waiting for the pool, and its wait for a
// chunk's units.
type intake struct {
	client    string
	need      int
	want      int
	received  int64
	admitted  time.Time
	waited    time.Duration
	waitStart time.Time
	waiting   bool
	grant     chan bool
}

type admission struct {
	client string
	need   int
	grant  chan *intake
}

// withinShare reports whether client may take units more; p.mu is held.
func (p *memoryPool) withinShare(client string, units int) bool {
	return p.perClient == 0 || p.byClient[client]+units <= p.perClient
}

// shareAdmits reports whether a body from client announcing need units fits within its share,
// next to what the address's bodies have taken and set aside; p.mu is held.
func (p *memoryPool) shareAdmits(client string, need int, now time.Time) bool {
	if p.perClient == 0 {
		return true
	}
	held := p.byClient[client] + need
	for _, in := range p.reading {
		if in.client == client {
			held += in.counted(now)
		}
	}
	return held <= p.perClient
}

// overShare reports whether a body waiting for its next chunk is held back by its address's share,
// so that only that address's own memory coming back can let it on; p.mu is held.
func (p *memoryPool) overShare(in *intake) bool {
	return !p.withinShare(in.client, in.want)
}

// capped reports whether it is its address's share alone that holds a body back, not the pool as
// well - the verdict then blames the address; p.mu is held.
func (p *memoryPool) capped(in *intake) bool {
	return p.overShare(in) && p.used+in.want <= p.size
}

// take counts units as used, also for client's share; p.mu is held.
func (p *memoryPool) take(client string, units int) {
	p.used += units
	if p.perClient > 0 {
		if p.byClient == nil {
			p.byClient = map[string]int{}
		}
		p.byClient[client] += units
	}
}

func (in *intake) took(units int) {
	in.need -= units
	if in.need < 0 {
		in.need = 0
	}
}

// stopWaiting ends a body's wait for a chunk; p.mu is held.
func (in *intake) stopWaiting(now time.Time) {
	if in.waiting {
		in.waited += now.Sub(in.waitStart)
		in.waiting = false
	}
}

// counted is how many of its announced units a body has set aside at now. Its pace leaves out the
// time it waited for the pool: a body held up there is not slow.
func (in *intake) counted(now time.Time) int {
	elapsed := now.Sub(in.admitted) - in.waited
	if in.waiting {
		elapsed -= now.Sub(in.waitStart)
	}
	if elapsed < rateGrace {
		return in.need
	}
	soon := unitsFor(int64(float64(in.received) / elapsed.Seconds() * promiseHorizon.Seconds()))
	if soon > in.need {
		return in.need
	}
	return soon
}

func (p *memoryPool) inUse() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.used
}

// promised is what the bodies being read have set aside at now; p.mu is held.
func (p *memoryPool) promised(now time.Time) int {
	n := 0
	for _, in := range p.reading {
		n += in.counted(now)
	}
	return n
}

// tryAdmit admits a body from client announcing need units if they fit next to what is set aside,
// and within client's share, with its first piece's unit; p.mu is held.
func (p *memoryPool) tryAdmit(client string, need, promised int, now time.Time) *intake {
	if p.used+promised+need > p.size || !p.shareAdmits(client, need, now) {
		return nil
	}
	p.take(client, 1)
	in := &intake{client: client, need: need - 1, admitted: now, grant: make(chan bool, 1)}
	p.reading = append(p.reading, in)
	return in
}

// free reports whether units are free for in: not set aside for another body, and within its
// address's share; p.mu is held.
func (p *memoryPool) free(in *intake, promised int, now time.Time, units int) bool {
	return p.withinShare(in.client, units) && p.used+promised-in.counted(now)+units <= p.size
}

// serve hands free units to the bodies being read that wait, oldest first, then admits the
// waiting bodies that fit; p.mu is held.
func (p *memoryPool) serve() {
	if !p.waits() {
		return
	}
	now := time.Now()
	promised := p.promised(now)
	for _, in := range p.reading {
		if in.waiting && p.free(in, promised, now, in.want) {
			before := in.counted(now)
			p.take(in.client, in.want)
			in.took(in.want)
			in.stopWaiting(now)
			promised += in.counted(now) - before
			in.grant <- true
		}
	}
	var still []*admission
	for _, a := range p.admitting {
		if in := p.tryAdmit(a.client, a.need, promised, now); in != nil {
			promised += in.counted(now)
			a.grant <- in
		} else {
			still = append(still, a)
		}
	}
	p.admitting = still
}

// waits reports whether any body waits, to be admitted or for a chunk; p.mu is held.
func (p *memoryPool) waits() bool {
	if len(p.admitting) > 0 {
		return true
	}
	for _, in := range p.reading {
		if in.waiting {
			return true
		}
	}
	return false
}

// tick serves the pool now and then while bodies wait: what is set aside shrinks with time, not
// only when chunks come back. One for the whole pool; p.mu is held.
func (p *memoryPool) tick() {
	if p.ticking || !p.waits() {
		return
	}
	p.ticking = true
	go func() {
		ticker := time.NewTicker(recheckEvery)
		defer ticker.Stop()
		for range ticker.C {
			p.mu.Lock()
			p.serve()
			if !p.waits() {
				p.ticking = false
				p.mu.Unlock()
				return
			}
			p.mu.Unlock()
		}
	}()
}

// giver is the waiting body to give way: the newest beyond its announcement, else the newest -
// never one its address's share holds back, which waits out its time; p.mu is held.
func (p *memoryPool) giver() *intake {
	var newest *intake
	for i := len(p.reading) - 1; i >= 0; i-- {
		in := p.reading[i]
		if !in.waiting || p.overShare(in) {
			continue
		}
		if in.need == 0 {
			return in
		}
		if newest == nil {
			newest = in
		}
	}
	return newest
}

// waitingBesides reports whether a body being read other than in waits for a chunk from the pool -
// not one its address's share holds back; p.mu is held.
func (p *memoryPool) waitingBesides(in *intake) bool {
	for _, other := range p.reading {
		if other != in && other.waiting && !p.overShare(other) {
			return true
		}
	}
	return false
}

// give hands back n units held for client.
func (p *memoryPool) give(client string, n int) {
	if n <= 0 {
		return
	}
	p.mu.Lock()
	p.used -= n
	if held, ok := p.byClient[client]; ok {
		if held <= n {
			delete(p.byClient, client)
		} else {
			p.byClient[client] = held - n
		}
	}
	p.serve()
	p.mu.Unlock()
}

// arrived counts what a body has sent, for its pace.
func (p *memoryPool) arrived(in *intake, n int) {
	p.mu.Lock()
	in.received += int64(n)
	p.mu.Unlock()
}

// admit admits a body from client announcing need units, within the queue's time; nil when it does
// not get in, q.capped when its address's share kept it out.
func (p *memoryPool) admit(q *queue, client string, need int) *intake {
	p.mu.Lock()
	now := time.Now()
	in := p.tryAdmit(client, need, p.promised(now), now)
	if in != nil {
		p.mu.Unlock()
		return in
	}
	a := &admission{client: client, need: need, grant: make(chan *intake, 1)}
	p.admitting = append(p.admitting, a)
	p.tick()
	p.mu.Unlock()

	start := time.Now()
	defer func() { q.left -= time.Since(start) }()
	timer := time.NewTimer(q.left)
	defer timer.Stop()
	select {
	case got := <-a.grant:
		return got
	case <-timer.C:
	case <-q.ctx.Done():
	}
	return p.withdraw(q, a)
}

// withdraw takes back a waiting admission; one granted meanwhile is kept.
func (p *memoryPool) withdraw(q *queue, a *admission) *intake {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case got := <-a.grant:
		return got
	default:
	}
	q.capped = !p.shareAdmits(a.client, a.need, time.Now()) && p.used+a.need <= p.size
	var still []*admission
	for _, other := range p.admitting {
		if other != a {
			still = append(still, other)
		}
	}
	p.admitting = still
	return nil
}

// more takes the units of a body's next chunk: at once if they are free for it, else in turn
// behind the bodies read before it, within the queue's time - or until it has waited longer than
// patience while another body being read waits too, is the one to give way and no other gave way
// within patience. False when it does not get them, q.capped when its address's share held it back.
func (p *memoryPool) more(in *intake, q *queue, units int) bool {
	p.mu.Lock()
	now := time.Now()
	if p.free(in, p.promised(now), now, units) {
		p.take(in.client, units)
		in.took(units)
		p.mu.Unlock()
		return true
	}
	in.want = units
	in.waiting = true
	in.waitStart = now
	p.tick()
	p.mu.Unlock()

	start := time.Now()
	defer func() { q.left -= time.Since(start) }()
	timer := time.NewTimer(q.left)
	defer timer.Stop()
	turns := time.NewTicker(patience)
	defer turns.Stop()
	waiting := true
	for waiting {
		select {
		case ok := <-in.grant:
			return ok
		case <-turns.C:
			p.mu.Lock()
			now := time.Now()
			if p.giver() == in && p.waitingBesides(in) && now.Sub(p.gaveWay) >= patience {
				p.gaveWay = now
				// Before unlocking: serve grants a chunk only to a body still waiting.
				in.stopWaiting(now)
				q.capped = false
				p.mu.Unlock()
				return false
			}
			p.mu.Unlock()
		case <-timer.C:
			waiting = false
		case <-q.ctx.Done():
			waiting = false
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case ok := <-in.grant:
		return ok
	default:
	}
	in.stopWaiting(time.Now())
	q.capped = p.capped(in)
	return false
}

// done ends a body's reading: what it set aside no longer counts.
func (p *memoryPool) done(in *intake) {
	p.mu.Lock()
	var still []*intake
	for _, other := range p.reading {
		if other != in {
			still = append(still, other)
		}
	}
	p.reading = still
	p.serve()
	p.mu.Unlock()
}

// paced counts what a body sends, for its pace in the pool - every reportEvery, not on each read,
// to spare the pool's lock.
type paced struct {
	r       io.Reader
	pool    *memoryPool
	in      *intake
	pending int
}

func (c *paced) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.pending += n
	if c.pending >= reportEvery || (err != nil && c.pending > 0) {
		c.pool.arrived(c.in, c.pending)
		c.pending = 0
	}
	return n, err
}
