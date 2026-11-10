package uploadscan

import (
	"strings"
	"testing"
)

// The messages are in the languages negotiate chooses from; every language has every text, with the
// placeholders English has; every reason refuse gives has a text; German keeps to Swiss spelling.
func TestMessagesAreComplete(t *testing.T) {
	placeholders := func(s string) string {
		var found []string
		for _, p := range []string{"{detail}", "{seconds}"} {
			if strings.Contains(s, p) {
				found = append(found, p)
			}
		}
		return strings.Join(found, " ") + "/" + strings.Repeat("{", strings.Count(s, "{"))
	}
	if len(languages) != len(messages) {
		t.Errorf("languages lists %v, the messages are in %d", languages, len(messages))
	}
	for _, lang := range languages {
		if messages[lang] == nil {
			t.Errorf("no messages in %s", lang)
		}
	}
	en := messages["en"]
	for lang, texts := range messages {
		if len(texts) != len(en) {
			t.Errorf("%s has %d texts, English %d", lang, len(texts), len(en))
		}
		for key, english := range en {
			if text := texts[key]; text == "" {
				t.Errorf("%s has no text for %s", lang, key)
			} else if placeholders(text) != placeholders(english) {
				t.Errorf("%s %s: placeholders %q, English %q", lang, key, placeholders(text), placeholders(english))
			}
		}
	}
	for _, c := range refusals {
		if en[c.reason] == "" {
			t.Errorf("no text for the reason %s", c.reason)
		}
	}
	for key, text := range messages["de"] {
		if strings.Contains(text, "ß") {
			t.Errorf("de %s: ß instead of ss", key)
		}
	}
}
