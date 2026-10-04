package toolchain

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOfficialCatalogVerifierRequiresConfiguredTrustAnchor(t *testing.T) {
	original := officialCatalogTrustAnchorsB64
	officialCatalogTrustAnchorsB64 = ""
	t.Cleanup(func() { officialCatalogTrustAnchorsB64 = original })

	if _, err := OfficialCatalogVerifier(); err == nil || !strings.Contains(err.Error(), "trust anchor") {
		t.Fatalf("OfficialCatalogVerifier() error = %v, want missing trust anchor error", err)
	}
}

func TestOfficialCatalogVerifierAcceptsFreshCatalogSignedByEmbeddedAnchor(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := officialCatalogPayload(time.Now().UTC().Add(-time.Minute), time.Now().UTC().Add(time.Hour))
	data := marshalSignedCatalog(t, payload, "catalog-2026", privateKey)
	setOfficialCatalogTrustAnchors(t, "catalog-2026", publicKey)

	verifier, err := OfficialCatalogVerifier()
	if err != nil {
		t.Fatalf("OfficialCatalogVerifier() error = %v", err)
	}
	if _, err := verifier.Verify(data); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestOfficialCatalogVerifierRejectsUnsignedUnknownAndExpiredCatalogs(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	setOfficialCatalogTrustAnchors(t, "catalog-2026", publicKey)
	verifier, err := OfficialCatalogVerifier()
	if err != nil {
		t.Fatal(err)
	}

	unsigned := []byte(`{"payload":{}}`)
	if _, err := verifier.Verify(unsigned); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("unsigned catalog error = %v, want signature error", err)
	}

	unknownPublic, unknownPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = unknownPublic
	unknown := marshalSignedCatalog(t, officialCatalogPayload(time.Now().UTC().Add(-time.Minute), time.Now().UTC().Add(time.Hour)), "revoked-key", unknownPrivate)
	if _, err := verifier.Verify(unknown); err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("unknown key error = %v, want key error", err)
	}

	expired := marshalSignedCatalog(t, officialCatalogPayload(time.Now().UTC().Add(-2*time.Hour), time.Now().UTC().Add(-time.Hour)), "catalog-2026", privateKey)
	if _, err := verifier.Verify(expired); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired catalog error = %v, want expiry error", err)
	}
}

func TestOfficialCatalogVerifierRejectsNotYetValidCatalog(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	setOfficialCatalogTrustAnchors(t, "catalog-2026", publicKey)
	verifier, err := OfficialCatalogVerifier()
	if err != nil {
		t.Fatal(err)
	}

	notYetValid := marshalSignedCatalog(t, officialCatalogPayload(time.Now().UTC().Add(time.Hour), time.Now().UTC().Add(2*time.Hour)), "catalog-2026", privateKey)
	if _, err := verifier.Verify(notYetValid); err == nil || !strings.Contains(err.Error(), "not yet valid") {
		t.Fatalf("not-yet-valid catalog error = %v, want validity error", err)
	}
}

func TestLoadOfficialCatalogRejectsUnsignedLocalCatalog(t *testing.T) {
	setOfficialCatalogTrustAnchors(t, "catalog-2026", make(ed25519.PublicKey, ed25519.PublicKeySize))
	path := t.TempDir() + string(os.PathSeparator) + "catalog.json"
	if err := os.WriteFile(path, []byte(`{"payload":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadOfficialCatalog(path); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("LoadOfficialCatalog() error = %v, want signature error", err)
	}
}

func officialCatalogPayload(issuedAt, expiresAt time.Time) CatalogPayload {
	payload := validCatalogPayload()
	payload.CatalogVersion = "2026.10.0"
	payload.IssuedAt = issuedAt.UTC().Format(time.RFC3339)
	payload.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
	return payload
}

func setOfficialCatalogTrustAnchors(t *testing.T, keyID string, publicKey ed25519.PublicKey) {
	t.Helper()
	anchors, err := json.Marshal(map[string]map[string]string{
		"keys": {keyID: base64.StdEncoding.EncodeToString(publicKey)},
	})
	if err != nil {
		t.Fatal(err)
	}
	original := officialCatalogTrustAnchorsB64
	officialCatalogTrustAnchorsB64 = base64.StdEncoding.EncodeToString(anchors)
	t.Cleanup(func() { officialCatalogTrustAnchorsB64 = original })
}
