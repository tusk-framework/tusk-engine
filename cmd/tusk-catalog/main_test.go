package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/toolchain"
)

func TestRunSignUsesEnvironmentReferencesAndWritesVerifiedAssets(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := toolchain.CatalogPayload{
		SchemaVersion:  1,
		CatalogVersion: "2026.10.0",
		IssuedAt:       now.Add(-time.Minute).Format(time.RFC3339),
		ExpiresAt:      now.Add(time.Hour).Format(time.RFC3339),
		AllowedHosts:   []string{"cdn.example"},
		Artifacts: []toolchain.Artifact{{
			Tool:       toolchain.RoadRunner,
			Version:    "2025.1.0",
			GOOS:       "windows",
			GOARCH:     "amd64",
			URL:        "https://cdn.example/rr.zip",
			SHA256:     strings.Repeat("a", 64),
			Format:     "zip",
			EntryPoint: "rr.exe",
		}},
	}
	payloadPath := filepath.Join(t.TempDir(), "payload.json")
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payloadPath, payloadBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	provenancePath := filepath.Join(t.TempDir(), "catalog.provenance.json")
	anchors, err := json.Marshal(map[string]map[string]string{
		"keys": {"release-2026": base64.StdEncoding.EncodeToString(publicKey)},
	})
	if err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{
		"TUSK_CATALOG_SIGNING_KEY_B64":   base64.StdEncoding.EncodeToString(privateKey),
		"TUSK_CATALOG_KEY_ID":            "release-2026",
		"TUSK_CATALOG_TRUST_ANCHORS_B64": base64.StdEncoding.EncodeToString(anchors),
	}
	lookup := func(name string) (string, bool) {
		value, ok := environment[name]
		return value, ok
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"sign", "--payload", payloadPath, "--catalog", catalogPath, "--provenance", provenancePath}, lookup, &stdout, &stderr); code != 0 {
		t.Fatalf("run(sign) = %d, want zero; stderr = %q", code, stderr.String())
	}
	catalogBytes, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (toolchain.CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"release-2026": publicKey}, RequireValidity: true}).Verify(catalogBytes); err != nil {
		t.Fatalf("written catalog verification error = %v", err)
	}
	if _, err := os.Stat(provenancePath); err != nil {
		t.Fatalf("provenance file error = %v", err)
	}
}

func TestRunSignRejectsPrivateKeyFileOption(t *testing.T) {
	if code := run([]string{"sign", "--private-key", "secret.key"}, func(string) (string, bool) { return "", false }, io.Discard, io.Discard); code == 0 {
		t.Fatal("run(sign --private-key) = 0, want non-zero")
	}
}
