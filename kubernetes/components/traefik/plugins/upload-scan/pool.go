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
	// A body waiting this long for a chunk while another body stuck alike waits too - on the pool,
	// or on its address's share with all of that address's memory held by its waiting bodies -
	// gives up if it is the one to give way (giver), so stuck bodies free memory one at a time.
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
	// When a body last gave way, among those stuck on the pool and per address among those stuck
	// on its share: at most one of each per patience.
	gaveWay    time.Time
	ownGaveWay map[string]time.Time
}

// intake is a body being read: its address, the units it holds and those it announced but has not
// taken yet, what it has sent since it was admitted, the time it spent waiting for the pool, and
// its wait for a chunk's units.
type intake struct {
	client    string
	held      int
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

// setAside is what the bodies being read have set aside at now, in all and per address; p.mu is
// held.
func (p *memoryPool) setAside(now time.Time) (int, map[string]int) {
	all, sets := 0, map[string]int{}
	for _, in := range p.reading {
		n := in.counted(now)
		all += n
		sets[in.client] += n
	}
	return all, sets
}

// setAsideFor is what the bodies being read have set aside at now, in all and of client's bodies;
// p.mu is held.
func (p *memoryPool) setAsideFor(client string, now time.Time) (all, own int) {
	for _, in := range p.reading {
		n := in.counted(now)
		all += n
		if in.client == client {
			own += n
		}
	}
	return all, own
}

// shareAdmits reports whether a body from client announcing need units fits within its share,
// next to what the address holds and its bodies have set aside (own); p.mu is held.
func (p *memoryPool) shareAdmits(client string, need, own int) bool {
	return p.perClient == 0 || p.byClient[client]+own+need <= p.perClient
}

// shareFree reports whether in may take units within its address's share, next to what the
// address holds and its bodies have set aside (own) - as the pool keeps a body's set-aside room
// from others, the share keeps it from its own address's other bodies; p.mu is held.
func (p *memoryPool) shareFree(in *intake, own int, now time.Time, units int) bool {
	return p.perClient == 0 || p.byClient[in.client]+own-in.counted(now)+units <= p.perClient
}

// overShare reports whether a body waiting for its next chunk is held back by its address's share,
// so that only that address's own memory coming back can let it on; p.mu is held.
func (p *memoryPool) overShare(in *intake, own int, now time.Time) bool {
	return !p.shareFree(in, own, now, in.want)
}

// capped reports whether it is its address's share alone that holds a body back, not the pool as
// well - the verdict then blames the address; p.mu is held.
func (p *memoryPool) capped(in *intake, promised, own int, now time.Time) bool {
	return p.overShare(in, own, now) && p.used+promised-in.counted(now)+in.want <= p.size
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

// tryAdmit admits a body from client announcing need units if they fit next to what is set aside
// (promised, own from setAsideFor), and within client's share, with its first piece's unit; p.mu
// is held.
func (p *memoryPool) tryAdmit(client string, need, promised, own int, now time.Time) *intake {
	if p.used+promised+need > p.size || !p.shareAdmits(client, need, own) {
		return nil
	}
	p.take(client, 1)
	in := &intake{client: client, held: 1, need: need - 1, admitted: now, grant: make(chan bool, 1)}
	p.reading = append(p.reading, in)
	return in
}

// free reports whether units are free for in: not set aside for another body (promised), and
// within its address's share (own), both from setAsideFor; p.mu is held.
func (p *memoryPool) free(in *intake, promised, own int, now time.Time, units int) bool {
	return p.shareFree(in, own, now, units) && p.used+promised-in.counted(now)+units <= p.size
}

// hand gives in units, returning how much what it has set aside changed; p.mu is held.
func (p *memoryPool) hand(in *intake, units int, now time.Time) int {
	before := in.counted(now)
	p.take(in.client, units)
	in.held += units
	in.took(units)
	in.stopWaiting(now)
	return in.counted(now) - before
}

// serve hands free units to the bodies being read that wait, oldest first, then admits the
// waiting bodies that fit; p.mu is held.
func (p *memoryPool) serve() {
	if !p.waits() {
		return
	}
	now := time.Now()
	promised, sets := p.setAside(now)
	for _, in := range p.reading {
		if in.waiting && p.free(in, promised, sets[in.client], now, in.want) {
			changed := p.hand(in, in.want, now)
			promised += changed
			sets[in.client] += changed
			in.grant <- true
		}
	}
	var still []*admission
	for _, a := range p.admitting {
		if in := p.tryAdmit(a.client, a.need, promised, sets[a.client], now); in != nil {
			promised += in.counted(now)
			sets[a.client] += in.counted(now)
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

// alike lists the bodies waiting stuck the way in does, in among them, newest first: on the pool,
// or on the same address's share (sets, from setAside); p.mu is held.
func (p *memoryPool) alike(in *intake, sets map[string]int, now time.Time) []*intake {
	own := p.overShare(in, sets[in.client], now)
	var stuck []*intake
	for i := len(p.reading) - 1; i >= 0; i-- {
		other := p.reading[i]
		if !other.waiting || own && other.client != in.client {
			continue
		}
		if p.overShare(other, sets[other.client], now) == own {
			stuck = append(stuck, other)
		}
	}
	return stuck
}

// giver is the body to give way among bodies stuck alike, newest first: the newest beyond its
// announcement, else the newest.
func giver(stuck []*intake) *intake {
	for _, in := range stuck {
		if in.need == 0 {
			return in
		}
	}
	return stuck[0]
}

// givesWay reports, and records, whether in gives up its wait now: it waits stuck with others
// alike and nothing else able to let them on - on its address's share, all of the address's
// memory held by those bodies, since memory of a body still being read, scanned or passed on comes
// back by itself - it is the one to give way, and no other body stuck alike gave way within
// patience. capped is the verdict's, as from capped; p.mu is held.
func (p *memoryPool) givesWay(in *intake, now time.Time) (gives, capped bool) {
	if !in.waiting {
		return false, false
	}
	promised, sets := p.setAside(now)
	stuck := p.alike(in, sets, now)
	if len(stuck) < 2 || giver(stuck) != in {
		return false, false
	}
	if !p.overShare(in, sets[in.client], now) {
		if now.Sub(p.gaveWay) < patience {
			return false, false
		}
		p.gaveWay = now
		return true, false
	}
	held := 0
	for _, other := range stuck {
		held += other.held
	}
	if held != p.byClient[in.client] || now.Sub(p.ownGaveWay[in.client]) < patience {
		return false, false
	}
	if p.ownGaveWay == nil {
		p.ownGaveWay = map[string]time.Time{}
	}
	p.ownGaveWay[in.client] = now
	return true, p.capped(in, promised, sets[in.client], now)
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
			delete(p.ownGaveWay, client)
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
	promised, own := p.setAsideFor(client, now)
	in := p.tryAdmit(client, need, promised, own, now)
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
	now := time.Now()
	promised, own := p.setAsideFor(a.client, now)
	q.capped = !p.shareAdmits(a.client, a.need, own) && p.used+promised+a.need <= p.size
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
// patience and gives way (givesWay). False when it does not get them, q.capped when its address's
// share alone held it back.
func (p *memoryPool) more(in *intake, q *queue, units int) bool {
	p.mu.Lock()
	now := time.Now()
	if promised, own := p.setAsideFor(in.client, now); p.free(in, promised, own, now, units) {
		p.hand(in, units, now)
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
			if gives, capped := p.givesWay(in, now); gives {
				q.capped = capped
				// Before unlocking: serve grants a chunk only to a body still waiting.
				in.stopWaiting(now)
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
	now = time.Now()
	promised, own := p.setAsideFor(in.client, now)
	q.capped = p.capped(in, promised, own, now)
	in.stopWaiting(now)
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
