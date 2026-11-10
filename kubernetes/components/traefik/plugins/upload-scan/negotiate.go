package uploadscan

import (
	"strconv"
	"strings"
)

const (
	asText = "text"
	asHTML = "html"
	asJSON = "json"
)

// The answer's formats and languages, each in the order a tie between two that only a wildcard
// covers goes; the first is the one when the client takes none.
var (
	formats   = []string{asText, asHTML, asJSON}
	languages = []string{"en", "de", "fr", "it", "es"}
)

type weighted struct {
	value string
	q     float64
}

// weigh splits a header of weighted values (Accept, Accept-Language) into its values, in lower
// case and in order, each with its q; an empty value, or one whose q does not parse, is left out.
func weigh(header []string) []weighted {
	var out []weighted
	for _, h := range header {
		for _, item := range strings.Split(h, ",") {
			params := strings.Split(item, ";")
			value := strings.ToLower(strings.TrimSpace(params[0]))
			q, ok := qOf(params[1:])
			if value != "" && ok {
				out = append(out, weighted{value: value, q: q})
			}
		}
	}
	return out
}

func qOf(params []string) (float64, bool) {
	for _, p := range params {
		name, value, _ := strings.Cut(p, "=")
		if strings.EqualFold(strings.TrimSpace(name), "q") {
			q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			return q, err == nil
		}
	}
	return 1, true
}

// pick is the candidate the client weighs highest by score - its q, and where the client named
// it - on a tie the one named first, then the earlier candidate.
func pick(candidates []string, score func(string) (float64, int)) string {
	best := candidates[0]
	bestQ := 0.0
	bestAt := 0
	for _, c := range candidates {
		q, at := score(c)
		if q > bestQ || q > 0 && q == bestQ && at < bestAt {
			best = c
			bestQ = q
			bestAt = at
		}
	}
	return best
}

// language is the answer's language, each weighed by its own tag's primary subtag (de-CH is de)
// or else by *.
func language(header []string) string {
	tags := weigh(header)
	return pick(languages, func(lang string) (float64, int) { return spoken(lang, tags) })
}

// spoken is the q the client gives a language - the highest of the tags naming it, else that of
// * - and where it named it; len(tags) when only * covers it.
func spoken(lang string, tags []weighted) (float64, int) {
	q := 0.0
	at := len(tags)
	named := false
	for i, t := range tags {
		primary, _, _ := strings.Cut(t.value, "-")
		switch {
		case primary == lang && (!named || t.q > q):
			q = t.q
			at = i
			named = true
		case primary == "*" && !named:
			q = t.q
		}
	}
	return q, at
}

// format is the answer's format, each weighed by the most specific media range that covers it.
func format(header []string) string {
	ranges := weigh(header)
	return pick(formats, func(f string) (float64, int) { return weight(f, ranges) })
}

// weight is the q the client gives a format - that of the most specific range covering it - and
// where it named the format; len(ranges) when only a wildcard covers it.
func weight(f string, ranges []weighted) (float64, int) {
	q := 0.0
	at := len(ranges)
	specific := 0
	for i, r := range ranges {
		s := covers(f, r.value)
		if s > specific {
			specific = s
			q = r.q
			at = len(ranges)
			if s == 3 {
				at = i
			}
		}
	}
	return q, at
}

// covers is how specifically a media range names a format: 3 by its own type, 2 by type/*, 1 by
// */*, 0 not at all.
func covers(f, mediaRange string) int {
	_, sub, _ := strings.Cut(mediaRange, "/")
	switch {
	case mediaRange == "*/*":
		return 1
	case f == asJSON:
		if mediaRange == "application/json" || strings.HasSuffix(sub, "+json") {
			return 3
		}
		if mediaRange == "application/*" {
			return 2
		}
	case f == asHTML && (mediaRange == "text/html" || mediaRange == "application/xhtml+xml"),
		f == asText && mediaRange == "text/plain":
		return 3
	case mediaRange == "text/*":
		return 2
	}
	return 0
}
