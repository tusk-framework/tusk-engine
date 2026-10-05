package toolchain

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestVerifyCatalogArtifactsChecksDigestAndDeduplicates(t *testing.T) {
	data := []byte("verified-artifact")
	payload := releaseCatalogPayload()
	payload.Artifacts[0].SHA256 = digestFor(data)
	duplicateURL := payload.Artifacts[0]
	duplicateURL.GOOS = "linux"
	payload.Artifacts = append(payload.Artifacts, duplicateURL)
	downloader := &fixtureDownloader{data: data}
	if err := VerifyCatalogArtifacts(context.Background(), payload, downloader, 1024); err != nil {
		t.Fatalf("VerifyCatalogArtifacts() error = %v", err)
	}
	if downloader.calls != 1 {
		t.Fatalf("downloader calls = %d, want one for duplicate URL/digest", downloader.calls)
	}

	payload.Artifacts[0].SHA256 = strings.Repeat("f", 64)
	if err := VerifyCatalogArtifacts(context.Background(), payload, &fixtureDownloader{data: data}, 1024); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("digest mismatch error = %v", err)
	}
}

func TestSignCatalogIsDeterministicAndAuthenticatesArtifacts(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := releaseCatalogPayload()
	anchors := map[string]ed25519.PublicKey{"release-2026": publicKey}

	first, err := SignCatalog(payload, "release-2026", privateKey, anchors)
	if err != nil {
		t.Fatalf("first SignCatalog() error = %v", err)
	}
	second, err := SignCatalog(payload, "release-2026", privateKey, anchors)
	if err != nil {
		t.Fatalf("second SignCatalog() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("SignCatalog() is not deterministic:\nfirst:  %s\nsecond: %s", first, second)
	}

	verified, err := (CatalogVerifier{
		PublicKeys:      anchors,
		RequireValidity: true,
		Now:             func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) },
	}).Verify(first)
	if err != nil {
		t.Fatalf("signed catalog verification error = %v", err)
	}
	if len(verified.Artifacts) != len(payload.Artifacts) || verified.Artifacts[0].Signature == "" {
		t.Fatalf("verified catalog = %#v, want signed artifact", verified)
	}
}

func TestSignCatalogRejectsMalformedPrivateKeyAndAnchorMismatch(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := releaseCatalogPayload()
	anchors := map[string]ed25519.PublicKey{"release-2026": publicKey}

	if _, err := SignCatalog(payload, "release-2026", ed25519.PrivateKey("bad"), anchors); err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("malformed private key error = %v, want private key error", err)
	}

	_, otherPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SignCatalog(payload, "release-2026", otherPrivateKey, anchors); err == nil || !strings.Contains(err.Error(), "trust anchor") {
		t.Fatalf("anchor mismatch error = %v, want trust anchor error", err)
	}

	if _, err := SignCatalog(payload, "missing-key", privateKey, anchors); err == nil || !strings.Contains(err.Error(), "trust anchor") {
		t.Fatalf("unknown key error = %v, want trust anchor error", err)
	}
}

func TestCatalogProvenanceIsDeterministicAndContainsDigests(t *testing.T) {
	payload := []byte(`{"schema_version":1,"catalog_version":"2026.10.0"}`)
	signedCatalog := []byte(`{"key_id":"release-2026","signature":"signed"}`)

	first, err := GenerateCatalogProvenance(payload, signedCatalog, "release-2026")
	if err != nil {
		t.Fatalf("first GenerateCatalogProvenance() error = %v", err)
	}
	second, err := GenerateCatalogProvenance(payload, signedCatalog, "release-2026")
	if err != nil {
		t.Fatalf("second GenerateCatalogProvenance() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("GenerateCatalogProvenance() is not deterministic: %q != %q", first, second)
	}

	var provenance CatalogProvenance
	if err := json.Unmarshal(first, &provenance); err != nil {
		t.Fatalf("provenance is not JSON: %v", err)
	}
	payloadDigest := sha256.Sum256(payload)
	catalogDigest := sha256.Sum256(signedCatalog)
	if provenance.KeyID != "release-2026" || provenance.PayloadSHA256 != hex.EncodeToString(payloadDigest[:]) || provenance.CatalogSHA256 != hex.EncodeToString(catalogDigest[:]) {
		t.Fatalf("provenance = %#v, want key and SHA-256 digests", provenance)
	}
}

func TestParseTrustAnchorsRejectsMalformedAndTrailingDocuments(t *testing.T) {
	if _, err := ParseTrustAnchorsB64("not-base64"); err == nil || !strings.Contains(err.Error(), "trust anchor") {
		t.Fatalf("malformed anchor error = %v, want trust anchor error", err)
	}

	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	document, err := json.Marshal(map[string]map[string]string{
		"keys": {"release-2026": base64.StdEncoding.EncodeToString(publicKey)},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(append(document, []byte(" {}")...))
	if _, err := ParseTrustAnchorsB64(encoded); err == nil || !strings.Contains(err.Error(), "trust anchor") {
		t.Fatalf("trailing anchor error = %v, want trust anchor error", err)
	}
}

func releaseCatalogPayload() CatalogPayload {
	payload := validCatalogPayload()
	payload.CatalogVersion = "2026.10.0"
	payload.IssuedAt = "2026-10-04T11:00:00Z"
	payload.ExpiresAt = "2026-10-04T13:00:00Z"
	return payload
}
