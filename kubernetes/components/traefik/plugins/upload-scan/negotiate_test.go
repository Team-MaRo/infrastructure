package uploadscan

import "testing"

func TestLanguageFollowsAcceptLanguage(t *testing.T) {
	cases := map[string]string{
		"":                        "en",
		"de-CH":                   "de",
		"fr;q=0.8, de;q=0.9":      "de",
		"it":                      "it",
		"es-419,es;q=0.9":         "es",
		"ja":                      "en",
		"ja, fr;q=0.5":            "fr",
		"*":                       "en",
		"de;q=0":                  "en",
		"en-US,en;q=0.9,de;q=0.8": "en",
		"de-CH, fr-CH":            "de",
		"fr;q=nonsense, it;q=0.1": "it",
		"*;q=0.9, de;q=0.5":       "en",
		"en;q=0, *":               "de",
		"*, fr":                   "fr",
		"de;q=0.1, de-CH":         "de",
		"it;q=0.5, de;q=0.1, de":  "de",
	}
	for header, want := range cases {
		var h []string
		if header != "" {
			h = []string{header}
		}
		if got := language(h); got != want {
			t.Errorf("Accept-Language %q: %s, want %s", header, got, want)
		}
	}
}

func TestFormatFollowsAccept(t *testing.T) {
	cases := map[string]string{
		"":    asText,
		"*/*": asText,
		"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,*/*;q=0.8": asHTML,
		"application/json, text/javascript, */*; q=0.01":                             asJSON,
		"application/problem+json":                                                   asJSON,
		"text/html, application/json":                                                asHTML,
		"application/json, text/html":                                                asJSON,
		"application/json;q=0.5, */*":                                                asText,
		"text/html;q=0, */*":                                                         asText,
		"text/html;q=0":                                                              asText,
		"application/json;q=0":                                                       asText,
		"image/png, text/html;q=0":                                                   asText,
		"text/plain;q=0.5, text/html;q=0.9":                                          asHTML,
		"text/*":                                                                     asText,
		"application/*":                                                              asJSON,
		"image/png":                                                                  asText,
		"not a media range":                                                          asText,
		"text/html;level, */*;q=0.1":                                                 asHTML,
		"application/json;foo, text/plain;q=0.5":                                     asJSON,
		"application/json;q=0.1;x, text/html;q=0.9":                                  asHTML,
	}
	for header, want := range cases {
		var h []string
		if header != "" {
			h = []string{header}
		}
		if got := format(h); got != want {
			t.Errorf("Accept %q: %s, want %s", header, got, want)
		}
	}
}
