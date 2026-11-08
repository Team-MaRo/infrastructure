// Package uploadscan is a Traefik middleware that hands every uploaded file to clamd before the
// request reaches the app (kubernetes/components/traefik/README.md, "Upload scan"). It runs in
// Traefik's Go interpreter (Yaegi 0.16), so: standard library only, no built-in min/max, and one
// assignment per statement - Yaegi drops the first target of `a, b = "x", f()`.
package uploadscan

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Config is the Middleware's plugin configuration, filled by field name (Traefik decodes it with
// mapstructure).
type Config struct {
	// Only "report" for now: every request passes, the verdict is logged.
	Mode string
	// clamd's host:port.
	Clamd string
	// The whole scan of one request, all its files included - also what bounds a flood of parts.
	ScanTimeout string
	// The longest a request waits for memory or a clamd session, all waits together.
	QueueTimeout string
	// "host" excludes a host, "host/path" exactly that path on it (the host in any case, the path
	// as written).
	Exclude []string
	// Never read: requests that may stream, read to the end they would hang. Matched as the type
	// itself or "type+anything".
	StreamContentTypes []string
	// Not read when at most SkipMaxBytes and of known length: the type is the client's word.
	// Matched like StreamContentTypes; any "+json" counts as well.
	SkipContentTypes []string
	SkipMaxBytes     int64
}

func CreateConfig() *Config {
	return &Config{
		Mode:         "report",
		ScanTimeout:  "45s",
		QueueTimeout: "10s",
		StreamContentTypes: []string{
			"application/grpc",
			"application/grpc-web",
			"application/grpc-web-text",
			"application/connect",
			"application/proto",
		},
		SkipContentTypes: []string{
			"application/x-www-form-urlencoded",
			"application/json",
		},
		SkipMaxBytes: 64 << 10, // 64 KiB: << 10 multiplies by 2^10, a KiB
	}
}

type UploadScan struct {
	next         http.Handler
	clamd        string
	scanTimeout  time.Duration
	queueTimeout time.Duration
	excludeHosts map[string]bool
	excludePaths map[string]bool
	streamTypes  []string
	skipTypes    []string
	skipMaxBytes int64
}

func New(_ context.Context, next http.Handler, config *Config, _ string) (http.Handler, error) {
	if config.Mode != "report" {
		return nil, fmt.Errorf("upload-scan: mode %q is not supported, only report", config.Mode)
	}
	if config.Clamd == "" {
		return nil, errors.New("upload-scan: clamd is required")
	}
	scanTimeout, err := time.ParseDuration(config.ScanTimeout)
	if err != nil {
		return nil, fmt.Errorf("upload-scan: scanTimeout: %w", err)
	}
	queueTimeout, err := time.ParseDuration(config.QueueTimeout)
	if err != nil {
		return nil, fmt.Errorf("upload-scan: queueTimeout: %w", err)
	}
	initLimits()

	u := &UploadScan{
		next:         next,
		clamd:        config.Clamd,
		scanTimeout:  scanTimeout,
		queueTimeout: queueTimeout,
		excludeHosts: map[string]bool{},
		excludePaths: map[string]bool{},
		streamTypes:  lower(config.StreamContentTypes),
		skipTypes:    lower(config.SkipContentTypes),
		skipMaxBytes: config.SkipMaxBytes,
	}
	for _, e := range config.Exclude {
		host, path, isPath := strings.Cut(e, "/")
		host = strings.ToLower(host)
		if strings.Contains(host, ":") {
			return nil, fmt.Errorf("upload-scan: exclude %q: hosts are matched without a port", e)
		}
		if isPath {
			u.excludePaths[host+"/"+path] = true
		} else {
			u.excludeHosts[host] = true
		}
	}
	return u, nil
}

func lower(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.ToLower(s))
	}
	return out
}
