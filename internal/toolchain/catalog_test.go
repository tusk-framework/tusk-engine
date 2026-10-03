package toolchain

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogVerifierAcceptsSignedCatalog(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := validCatalogPayload()
	data := marshalSignedCatalog(t, payload, "test-key", privateKey)

	got, err := (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(data)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0].Tool != RoadRunner {
		t.Fatalf("Verify() = %#v, want one RoadRunner artifact", got)
	}
}

func TestCatalogVerifierRejectsAlteredPayload(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data := marshalSignedCatalog(t, validCatalogPayload(), "test-key", privateKey)
	var envelope SignedCatalog
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Payload.Artifacts[0].Version = "9.9.9"
	altered, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	_, err = (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(altered)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("Verify() error = %v, want signature error", err)
	}
}

func TestCatalogVerifierRejectsUnknownKeyAndMalformedSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	unknownKey := marshalSignedCatalog(t, validCatalogPayload(), "unknown", privateKey)
	_, err = (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(unknownKey)
	if err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("unknown key error = %v", err)
	}

	malformed := marshalSignedCatalog(t, validCatalogPayload(), "test-key", privateKey)
	var envelope SignedCatalog
	if err := json.Unmarshal(malformed, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Signature = "not-base64"
	malformed, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(malformed)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("malformed signature error = %v", err)
	}
}

func TestCatalogVerifierRejectsInvalidArtifactURLAndSchema(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	invalidURL := validCatalogPayload()
	invalidURL.Artifacts[0].URL = "http://insecure.example/rr.zip"
	_, err = (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(marshalSignedCatalog(t, invalidURL, "test-key", privateKey))
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("invalid URL error = %v", err)
	}

	unsupportedSchema := validCatalogPayload()
	unsupportedSchema.SchemaVersion = 99
	_, err = (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(marshalSignedCatalog(t, unsupportedSchema, "test-key", privateKey))
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("schema error = %v", err)
	}
}

func validCatalogPayload() CatalogPayload {
	return CatalogPayload{
		SchemaVersion: 1,
		Artifacts: []Artifact{{
			Tool:       RoadRunner,
			Version:    "2025.1.0",
			GOOS:       "windows",
			GOARCH:     "amd64",
			URL:        "https://cdn.example/rr.zip",
			SHA256:     strings.Repeat("a", 64),
			Signature:  "artifact-signature",
			Format:     "zip",
			EntryPoint: "rr.exe",
		}},
	}
}

func marshalSignedCatalog(t *testing.T, payload CatalogPayload, keyID string, privateKey ed25519.PrivateKey) []byte {
	t.Helper()
	canonical, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope := SignedCatalog{
		Payload:   payload,
		KeyID:     keyID,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)),
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
