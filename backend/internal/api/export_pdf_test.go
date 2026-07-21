package api

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteArtifactsPDF(t *testing.T) {
	latest := "1.2.0"
	behind := 3
	items := []artifactDTO{
		{
			ID: "a1", ClusterID: "c1", ClusterName: "prod-eu", Kind: "image",
			Namespace: "platform", Identity: "ghcr.io/example/api", Installed: "1.0.0",
			Latest: &latest, DriftClass: "major", DriftScore: 20000, ReleasesBehind: &behind,
			LastSeen: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
		},
		{
			ID: "a2", ClusterID: "c1", ClusterName: "prod-eu", Kind: "helm",
			Namespace: "platform", Identity: "ingress-nginx", Installed: "4.8.0",
			Latest: &latest, DriftClass: "patch", DriftScore: 100,
			LastSeen: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	if err := writeArtifactsPDF(&buf, items, time.Date(2026, 7, 21, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("writeArtifactsPDF: %v", err)
	}
	raw := buf.Bytes()
	if len(raw) < 5 || string(raw[:5]) != "%PDF-" {
		t.Fatalf("expected PDF magic, got %q", string(raw[:min(20, len(raw))]))
	}
	if !bytes.Contains(raw, []byte("%%EOF")) {
		t.Error("pdf missing end-of-file marker")
	}
	// Core fonts encode Latin text as literal strings in the content stream.
	body := string(raw)
	if !strings.Contains(body, "Omastx") {
		t.Error("pdf missing brand title")
	}
}

func TestWriteArtifactsPDFEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeArtifactsPDF(&buf, nil, time.Now().UTC()); err != nil {
		t.Fatalf("writeArtifactsPDF empty: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatal("expected PDF for empty ledger")
	}
}
