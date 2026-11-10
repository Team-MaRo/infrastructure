package uploadscan

import (
	"context"
	"net/http"
	"testing"
)

func TestOnlyReportAndBlockModeAreAccepted(t *testing.T) {
	for mode, accepted := range map[string]bool{"report": true, "block": true, "Block": false, "": false} {
		config := CreateConfig()
		config.Clamd = "127.0.0.1:3310"
		config.Mode = mode
		if _, err := New(context.Background(), http.NotFoundHandler(), config, "upload-scan"); (err == nil) != accepted {
			t.Errorf("mode %q: %v", mode, err)
		}
	}
}

func TestExcludeWithAPortIsRefused(t *testing.T) {
	config := CreateConfig()
	config.Clamd = "clamd:3310"
	config.Exclude = []string{"api.example.org:8443/ingest"}
	if _, err := New(context.Background(), http.NotFoundHandler(), config, "upload-scan"); err == nil {
		t.Fatal("an exclusion with a port was accepted")
	}
}
