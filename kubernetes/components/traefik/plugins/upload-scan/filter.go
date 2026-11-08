package uploadscan

import (
	"mime"
	"net"
	"net/http"
	"strings"
)

// wanted decides on the headers alone whether a request is read and scanned: any method with a
// body - apps parse uploads on DELETE and others too.
func (u *UploadScan) wanted(req *http.Request) bool {
	if req.ContentLength == 0 || req.Body == nil || req.Body == http.NoBody {
		return false
	}
	host := strings.ToLower(req.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if u.excludeHosts[host] || u.excludePaths[host+req.URL.Path] {
		return false
	}
	// WordPress' media endpoint takes a raw body under any type, but only with this header - in
	// any of its values.
	if len(req.Header.Values("Content-Disposition")) > 0 {
		return true
	}
	// Skipped only when every Content-Type value says so: a server may take another than the first.
	values := req.Header.Values("Content-Type")
	// Some parsers take "multipart/form-data+json" for form-data.
	if len(values) == 0 || isMultipart(values) {
		return true
	}
	stream := true
	small := true
	for _, v := range values {
		mediaType, _, err := mime.ParseMediaType(v)
		if err != nil {
			return true
		}
		stream = stream && matches(mediaType, u.streamTypes)
		small = small && (matches(mediaType, u.skipTypes) || strings.HasSuffix(mediaType, "+json"))
	}
	if stream {
		return false
	}
	if small {
		return req.ContentLength < 0 || req.ContentLength > u.skipMaxBytes
	}
	return true
}

func matches(mediaType string, types []string) bool {
	for _, t := range types {
		if mediaType == t || strings.HasPrefix(mediaType, t+"+") {
			return true
		}
	}
	return false
}
