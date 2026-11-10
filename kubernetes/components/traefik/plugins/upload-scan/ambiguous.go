package uploadscan

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
)

const (
	// More parts than any form sends: such a body is ambiguous, and scanning stops there.
	maxParts = 1000
	// A part's headers: browsers send a few short lines. PHP reads them through a 5120-byte
	// buffer and, at some line lengths, takes the rest of the headers for the file's content
	// (2026-11-07, PHP 8.3); a block below that cannot hold such a line.
	maxHeaderBlock = 4096
)

// encoded names a request's Content-Encoding, or returns "": browsers never compress an upload, and
// the scan sees the compressed bytes - a server that inflates the body stores what clamd never saw,
// and for a multipart body cuts parts the scan did not.
func encoded(header http.Header) string {
	for _, e := range header.Values("Content-Encoding") {
		if e = strings.TrimSpace(e); e != "" && !strings.EqualFold(e, "identity") {
			return "Content-Encoding on the request"
		}
	}
	return ""
}

// ambiguous names why a multipart request is not the plain kind browsers and HTTP libraries send,
// or returns "". Parsers read such requests differently (PHP, Node, Go and Python each in their own
// way), so what the scan saw may not be what the app stores; block mode refuses them (refuse.go)
// rather than trying to read each the way every parser would.
func ambiguous(header http.Header, bounds []string, body []byte) string {
	if reason := encoded(header); reason != "" {
		return reason
	}
	values := header.Values("Content-Type")
	if len(values) != 1 {
		return "Content-Type given more than once"
	}
	mediaType, params, err := mime.ParseMediaType(values[0])
	if err != nil {
		return "Content-Type not parsable"
	}
	if mediaType != "multipart/form-data" || !isFormData(values) {
		return "multipart other than form-data"
	}
	boundary := params["boundary"]
	if len(bounds) != 1 || bounds[0] != boundary {
		return "boundary read differently by parsers"
	}
	delimiter := []byte("--" + boundary)
	if !bytes.HasPrefix(body, delimiter) {
		return "body does not start with the boundary"
	}
	ending := ""
	parts := 0
	at := 0
	// At most maxParts + 1 rounds: each ends at a delimiter, the first closing one ends the loop.
	for {
		tail := body[at+len(delimiter):]
		closing := bytes.HasPrefix(tail, []byte("--"))
		// What follows must end the delimiter line: "--" for the closing one, spaces, a line break.
		// PHP also ends a part at a line that merely starts with the delimiter; Go and Python do
		// not.
		rest := bytes.TrimLeft(bytes.TrimPrefix(tail, []byte("--")), " \t")
		after := ""
		switch {
		case len(rest) == 0:
		case bytes.HasPrefix(rest, []byte("\r\n")):
			after = "crlf"
		case rest[0] == '\n':
			after = "lf"
		default:
			return "boundary inside the content"
		}
		before := ""
		switch {
		case at == 0:
		case at >= 2 && body[at-2] == '\r' && body[at-1] == '\n':
			before = "crlf"
		case body[at-1] == '\n':
			before = "lf"
		default:
			return "boundary inside the content"
		}
		// Go reads the whole body with the line break of the first delimiter line, PHP each part
		// with its own: one kind throughout, or they cut differently.
		for _, b := range []string{after, before} {
			if b == "" {
				continue
			}
			if ending == "" {
				ending = b
			}
			if b != ending {
				return "line breaks mixed"
			}
		}
		if closing {
			// Go stops at the first closing delimiter, PHP reads on.
			if len(bytes.TrimSpace(rest)) > 0 {
				return "data after the closing boundary"
			}
			break
		}
		parts++
		if parts > maxParts {
			return "more than 1000 parts"
		}
		from := at + len(delimiter)
		next := bytes.Index(body[from:], delimiter)
		limit := len(body)
		if next >= 0 {
			limit = from + next
		}
		if longHeaders(body, from, limit) {
			return "part headers too long"
		}
		if next < 0 {
			return "no closing boundary"
		}
		at = limit
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		// Raw, so a transfer encoding stays visible: Go decodes quoted-printable before storing,
		// what the scan saw was the encoded text.
		part, err := reader.NextRawPart()
		if errors.Is(err, io.EOF) {
			return ""
		}
		if err != nil {
			return "not valid multipart"
		}
		if part.Header.Get("Content-Transfer-Encoding") != "" {
			return "transfer encoding in a part"
		}
		if _, err := io.Copy(io.Discard, part); err != nil {
			return "not valid multipart"
		}
	}
}

// longHeaders is whether a part's headers, from after its delimiter up to the empty line before
// its content (or limit), take maxHeaderBlock bytes or more.
func longHeaders(body []byte, start, limit int) bool {
	end := headerEnd(body, start, limit)
	if end < 0 {
		end = limit
	}
	return end-start >= maxHeaderBlock
}
