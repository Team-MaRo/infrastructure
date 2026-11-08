package uploadscan

import (
	"context"
	"net/http"
	"testing"
)

func TestOnlyReportModeIsAccepted(t *testing.T) {
	config := CreateConfig()
	config.Clamd = "127.0.0.1:3310"
	config.Mode = "block"
	if _, err := New(context.Background(), http.NotFoundHandler(), config, "upload-scan"); err == nil {
		t.Fatal("block mode must be refused until phase 3")
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
