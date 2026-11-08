package uploadscan

import (
	"encoding/json"
	"net/http"
	"os"
	"time"
)

type result struct {
	// Files counts the parts sent to clamd, Unanswered those it gave no answer for.
	Files      int
	Unanswered int
	Bytes      int64
	Verdict    string
	Sig        string
	Error      string
	// Why a body was scanned whole, or why scanning stopped early.
	Note string
	// Stopped is set when time ran out or the client left before all parts were scanned.
	Stopped bool
	// Why a multipart body is not the plain kind clients send (ambiguous.go).
	Ambiguous string
}

func (r *result) answered() int {
	return r.Files - r.Unanswered
}

func (r *result) note(n string) {
	if r.Note != "" {
		r.Note += "; "
	}
	r.Note += n
}

// log writes one JSON line into Traefik's log: no client address and no file names.
func (u *UploadScan) log(req *http.Request, res result, took time.Duration) {
	fields := map[string]interface{}{
		"upload_scan": "report",
		"host":        clip(req.Host),
		"path":        clip(req.URL.Path),
		"method":      clip(req.Method),
		"parts":       res.answered(),
		"bytes":       res.Bytes,
		"verdict":     res.Verdict,
		"ms":          took.Milliseconds(),
	}
	if res.Sig != "" {
		fields["sig"] = clip(res.Sig)
	}
	if res.Error != "" {
		fields["error"] = clip(res.Error)
	}
	if res.Note != "" {
		fields["note"] = clip(res.Note)
	}
	if res.Ambiguous != "" {
		fields["ambiguous"] = res.Ambiguous
	}
	line, err := json.Marshal(fields)
	if err != nil {
		return
	}
	_, _ = os.Stdout.WriteString(string(line) + "\n")
}

// clip keeps a logged text short: errors may quote what the client sent.
func clip(s string) string {
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
