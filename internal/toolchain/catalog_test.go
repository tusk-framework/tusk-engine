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

func TestCatalogVerifierRejectsInvalidArtifactSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := validCatalogPayload()
	data := marshalSignedCatalog(t, payload, "test-key", privateKey)
	var envelope SignedCatalog
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Payload.Artifacts[0].Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	canonical, err := json.Marshal(envelope.Payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	data, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	_, err = (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(data)
	if err == nil || !strings.Contains(err.Error(), "artifact signature") {
		t.Fatalf("artifact signature error = %v, want artifact signature error", err)
	}
}

func TestCatalogVerifierRejectsArtifactHostOutsideAllowlist(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := validCatalogPayload()
	payload.AllowedHosts = []string{"trusted.example"}

	_, err = (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(marshalSignedCatalog(t, payload, "test-key", privateKey))
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("host allowlist error = %v, want allowlist error", err)
	}
}

func TestCatalogVerifierAcceptsPlatformVariantAndRejectsDuplicateVariant(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := validCatalogPayload()
	payload.Artifacts[0].GOOS = ""
	payload.Artifacts[0].GOARCH = ""
	payload.Artifacts[0].Target = Platform{OS: "linux", Arch: "amd64", Distribution: "ubuntu-24.04", Libc: "glibc"}
	if _, err := (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(marshalSignedCatalog(t, payload, "test-key", privateKey)); err != nil {
		t.Fatalf("platform variant verification error = %v", err)
	}

	duplicate := payload
	duplicate.Artifacts = append(append([]Artifact(nil), payload.Artifacts...), payload.Artifacts[0])
	_, err = (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(marshalSignedCatalog(t, duplicate, "test-key", privateKey))
	if err == nil || !strings.Contains(err.Error(), "duplicate artifact") {
		t.Fatalf("duplicate platform variant error = %v", err)
	}
}

func TestCatalogVerifierRejectsConflictingLegacyAndPlatformFields(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := validCatalogPayload()
	payload.Artifacts[0].Target = Platform{OS: "linux", Arch: "amd64"}
	_, err = (CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}}).Verify(marshalSignedCatalog(t, payload, "test-key", privateKey))
	if err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("conflicting target error = %v, want target validation error", err)
	}
}

func TestLegacyArtifactEncodingDoesNotAddEmptyTarget(t *testing.T) {
	data, err := json.Marshal(Artifact{Tool: RoadRunner, Version: "2025.1.0", GOOS: "windows", GOARCH: "amd64", URL: "https://cdn.example/rr.zip", SHA256: strings.Repeat("a", 64), Signature: "signature", Format: "zip", EntryPoint: "rr.exe"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"target"`) {
		t.Fatalf("legacy artifact JSON unexpectedly contains target: %s", data)
	}
}

func validCatalogPayload() CatalogPayload {
	return CatalogPayload{
		SchemaVersion: 1,
		AllowedHosts:  []string{"cdn.example"},
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
	for index := range payload.Artifacts {
		canonicalArtifact, err := testArtifactSigningBytes(payload.Artifacts[index])
		if err != nil {
			t.Fatal(err)
		}
		payload.Artifacts[index].Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonicalArtifact))
	}
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

func testArtifactSigningBytes(artifact Artifact) ([]byte, error) {
	artifact.Signature = ""
	return json.Marshal(artifact)
}
