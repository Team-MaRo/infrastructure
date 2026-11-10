package uploadscan

import (
	"encoding/json"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// respond writes the answer to a refused upload, in the format (Accept) and the language
// (Accept-Language) the client prefers. A client that left gets the status alone.
func respond(rw http.ResponseWriter, req *http.Request, r *refusal) {
	if r.status == statusClientGone {
		rw.WriteHeader(r.status)
		return
	}
	h := rw.Header()
	lang := language(req.Header.Values("Accept-Language"))
	text := r.text(lang)
	retry := r.retry(lang)
	if r.retryAfter > 0 {
		h.Set("Retry-After", strconv.Itoa(r.retryAfter))
	}
	var body string
	switch format(req.Header.Values("Accept")) {
	case asJSON:
		h.Set("Content-Type", "application/json")
		body = answerJSON(r, text)
	case asHTML:
		h.Set("Content-Type", "text/html; charset=utf-8")
		body = answerHTML(lang, text, retry)
	default:
		h.Set("Content-Type", "text/plain; charset=utf-8")
		body = strings.TrimSpace(text+"\n"+retry) + "\n"
	}
	h.Set("Content-Language", lang)
	h.Add("Vary", "Accept, Accept-Language")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	rw.WriteHeader(r.status)
	_, _ = io.WriteString(rw, body)
}

func answerJSON(r *refusal, text string) string {
	answer := map[string]interface{}{"code": r.reason, "message": text, "retry_after": nil}
	if r.retryAfter > 0 {
		answer["retry_after"] = r.retryAfter
	}
	b, _ := json.Marshal(answer)
	return string(b)
}

// answerHTML is a page of its own for every site: no script, no branding, every text escaped.
func answerHTML(lang, text, retry string) string {
	title := html.EscapeString(messages[lang]["title"])
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="` + lang + `"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	b.WriteString(`<meta name="color-scheme" content="light dark"><title>` + title + `</title>`)
	b.WriteString(`<style>body{font:1rem/1.5 system-ui,sans-serif;`)
	b.WriteString(`max-width:36rem;margin:4rem auto;padding:0 1rem}</style>`)
	b.WriteString(`</head><body><h1>` + title + `</h1><p>` + html.EscapeString(text) + `</p>`)
	if retry != "" {
		b.WriteString(`<p>` + html.EscapeString(retry) + `</p>`)
	}
	b.WriteString("</body></html>\n")
	return b.String()
}
