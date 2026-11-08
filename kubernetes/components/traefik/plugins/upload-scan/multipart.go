package uploadscan

import (
	"bytes"
	"strconv"
	"strings"
	"time"
)

// maxHeaderValues bounds how many Content-Type values are searched for boundaries.
const maxHeaderValues = 4

// isFormData is whether a value is multipart/form-data as PHP reads it - in any case, up to the
// first ";", "," or space - the only multipart type parsers turn into files; the app gets any
// other as it is ("multipart/form-data\t; boundary=b" included).
func isFormData(values []string) bool {
	for _, v := range values {
		t := strings.ToLower(strings.TrimSpace(v))
		if end := strings.IndexAny(t, ";, "); end >= 0 {
			t = t[:end]
		}
		if t == "multipart/form-data" {
			return true
		}
	}
	return false
}

func isMultipart(values []string) bool {
	for _, v := range values {
		if hasPrefixFold(strings.TrimSpace(v), "multipart/") {
			return true
		}
	}
	return false
}

// boundaries are the boundaries parsers may split the body by, from the first header values:
// PHP's readings - after "boundary" found case-sensitively first, else in any case, up to the first
// "=" anywhere after it, the value kept as is and trimmed - and the first and the last parameter
// named boundary, which other parsers take.
func boundaries(values []string) []string {
	var out []string
	add := func(b string) {
		if b == "" {
			return
		}
		for _, seen := range out {
			if seen == b {
				return
			}
		}
		out = append(out, b)
	}
	if len(values) > maxHeaderValues {
		values = values[:maxHeaderValues]
	}
	for _, v := range values {
		for _, i := range []int{strings.Index(v, "boundary"), indexFold(v, "boundary")} {
			if i < 0 {
				continue
			}
			raw := phpValue(v[i+len("boundary"):])
			add(raw)
			add(strings.TrimSpace(raw))
		}
		var first, last string
		for _, param := range strings.Split(v, ";") {
			name, value, ok := strings.Cut(param, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(name), "boundary") {
				continue
			}
			b := strings.Trim(strings.TrimSpace(value), `"`)
			if first == "" {
				first = b
			}
			last = b
		}
		add(first)
		add(last)
	}
	return out
}

// phpValue is the boundary as PHP takes it after the word: from the first "=" on, up to a closing
// quote, or else up to "," or ";", spaces kept.
func phpValue(rest string) string {
	eq := strings.Index(rest, "=")
	if eq < 0 {
		return ""
	}
	value := rest[eq+1:]
	if strings.HasPrefix(value, `"`) {
		value = value[1:]
		if end := strings.Index(value, `"`); end >= 0 {
			return value[:end]
		}
		return value
	}
	if end := strings.IndexAny(value, ",;"); end >= 0 {
		return value[:end]
	}
	return value
}

// indexFold finds an ASCII word in s ignoring case, without changing s's length the way
// strings.ToLower can.
func indexFold(s, word string) int {
	for i := 0; i+len(word) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(word)], word) {
			return i
		}
	}
	return -1
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// nextDelimiter is the next "--boundary" at a line start (the body's start or after "\n") from
// position from on, or -1. Parsers split only there. Searched for together with its "\n", so
// matches in the middle of a line are skipped by the standard library, not step by step in Yaegi.
func nextDelimiter(body, delimiter []byte, from int) int {
	if from > len(body) {
		return -1
	}
	if from == 0 && bytes.HasPrefix(body, delimiter) {
		return 0
	}
	search := from - 1
	if search < 0 {
		search = 0
	}
	i := bytes.Index(body[search:], append([]byte{'\n'}, delimiter...))
	if i < 0 {
		return -1
	}
	return search + i + 1
}

// headerEnd is where a part's content starts: after the first empty line between start and limit
// (the next delimiter), "\n\n" or "\n\r\n", or -1. Bounded by limit, so cutting a body stays
// linear in its size.
func headerEnd(body []byte, start, limit int) int {
	lf := bytes.Index(body[start:limit], []byte("\n\n"))
	crlf := bytes.Index(body[start:limit], []byte("\n\r\n"))
	switch {
	case crlf >= 0 && (lf < 0 || crlf < lf):
		return start + crlf + 3
	case lf >= 0:
		return start + lf + 2
	}
	return -1
}

// trimBreak drops the line break that belongs to the next delimiter.
func trimBreak(b []byte) []byte {
	if bytes.HasSuffix(b, []byte("\r\n")) {
		return b[:len(b)-2]
	}
	return bytes.TrimSuffix(b, []byte("\n"))
}

// scanMultipart cuts the body into parts at every boundary's delimiters and scans each part's
// content right away, within the request's time; a body without any delimiter is scanned whole.
// Every delimiter at a line start begins a part. A part's content - what a parser stores - runs
// from the empty line after its headers to the next delimiter whose line break lies within the
// content. Whether a "--boundary" line directly after the headers ends the part differs between
// parsers (PHP: not when the part has a Content-Disposition), so both readings are scanned: that
// line also begins a part of its own. A part without that empty line is scanned whole.
func (u *UploadScan) scanMultipart(j *scanning, body []byte, bounds []string, res *result) {
	found := false
	// Boundaries that are prefixes of each other ("b " and "b") cut at the same lines.
	scanned := map[span]bool{}
	for _, b := range bounds {
		delimiter := []byte("--" + b)
		at := nextDelimiter(body, delimiter, 0)
		if at < 0 {
			continue
		}
		found = true
		for at >= 0 {
			if time.Now().After(j.deadline) || j.ctx.Err() != nil {
				res.note("time used up or client gone after " + strconv.Itoa(res.answered()) + " parts")
				res.Stopped = true
				return
			}
			start := at + len(delimiter)
			next := nextDelimiter(body, delimiter, start)
			limit := len(body)
			if next >= 0 {
				limit = next
			}
			var c []byte
			from := start
			if h := headerEnd(body, start, limit); h >= 0 {
				// The next delimiter, unless the content starts right at it.
				end := next
				if h == next {
					end = nextDelimiter(body, delimiter, h+1)
				}
				if end < 0 {
					end = len(body)
				}
				from = h
				c = trimBreak(body[h:end])
			} else {
				piece := body[start:limit]
				// The closing "--boundary--" itself.
				if bytes.Equal(bytes.TrimSpace(piece), []byte("--")) {
					at = next
					continue
				}
				c = trimBreak(piece)
			}
			at = next
			s := span{from: from, to: from + len(c)}
			if len(c) == 0 || scanned[s] {
				continue
			}
			if res.Files >= maxParts {
				res.note("more than 1000 parts, the rest not scanned")
				res.Stopped = true
				return
			}
			scanned[s] = true
			res.Files++
			answer := u.scan(j, bytes.NewReader(c))
			u.record(res, answer)
			if stop(res, answer) {
				return
			}
		}
	}
	// Something was scanned, here or whole before.
	if res.Files > 0 {
		return
	}
	if found {
		res.note("no part with content found, scanned whole")
	} else {
		res.note("boundary not in the body, scanned whole")
	}
	u.scanWhole(j, body, res)
}

type span struct {
	from, to int
}
