package uploadscan

import (
	"net"
	"net/http"
	"strings"
	"sync"
)

// clientOf is the address a request comes from, as the limits per address count it: the visitor's
// address Cloudflare passes on (the scan runs only behind cloudflare-only, which lets nothing else
// reach it), else the connection's. An IPv6 address counts as its /64, the size of one network.
func clientOf(req *http.Request) string {
	ip := net.ParseIP(strings.TrimSpace(req.Header.Get("CF-Connecting-IP")))
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	if ip == nil {
		ip = net.ParseIP(host)
	}
	if ip == nil {
		return host
	}
	if ip.To4() != nil {
		return ip.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// clients counts the uploads being checked per address.
type clients struct {
	mu       sync.Mutex
	inFlight map[string]int
}

// enter takes one of client's maxPerClient places; false when none is left.
func (c *clients) enter(client string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inFlight[client] >= maxPerClient {
		return false
	}
	c.inFlight[client]++
	return true
}

func (c *clients) leave(client string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inFlight[client]--
	if c.inFlight[client] <= 0 {
		delete(c.inFlight, client)
	}
}
