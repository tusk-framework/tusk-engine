package toolchain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

const currentCatalogSchema = 1

// Artifact identifies one immutable tool binary for one target platform.
type Artifact struct {
	Tool       ToolName `json:"tool"`
	Version    string   `json:"version"`
	GOOS       string   `json:"goos"`
	GOARCH     string   `json:"goarch"`
	URL        string   `json:"url"`
	SHA256     string   `json:"sha256"`
	Signature  string   `json:"signature"`
	Format     string   `json:"format"`
	EntryPoint string   `json:"entrypoint"`
}

// CatalogPayload is the signed portion of a toolchain catalog.
type CatalogPayload struct {
	SchemaVersion  int        `json:"schema_version"`
	CatalogVersion string     `json:"catalog_version,omitempty"`
	IssuedAt       string     `json:"issued_at,omitempty"`
	ExpiresAt      string     `json:"expires_at,omitempty"`
	AllowedHosts   []string   `json:"allowed_hosts"`
	Artifacts      []Artifact `json:"artifacts"`
}

// SignedCatalog wraps a catalog payload with the key identifier and signature
// used to authenticate it.
type SignedCatalog struct {
	Payload   CatalogPayload `json:"payload"`
	KeyID     string         `json:"key_id"`
	Signature string         `json:"signature"`
}

// CatalogVerifier validates signed catalog documents against trusted public
// keys supplied by the Engine release.
type CatalogVerifier struct {
	PublicKeys      map[string]ed25519.PublicKey
	RequireValidity bool
	Now             func() time.Time
}

// Verify decodes and authenticates a signed catalog. The signature covers the
// deterministic JSON encoding of the typed payload, not the outer envelope.
func (v CatalogVerifier) Verify(data []byte) (CatalogPayload, error) {
	var envelope SignedCatalog
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return CatalogPayload{}, fmt.Errorf("decode signed catalog: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return CatalogPayload{}, errors.New("decode signed catalog: trailing JSON")
		}
		return CatalogPayload{}, fmt.Errorf("decode signed catalog: %w", err)
	}

	if strings.TrimSpace(envelope.Signature) == "" {
		return CatalogPayload{}, errors.New("catalog signature is required")
	}
	publicKey, ok := v.PublicKeys[envelope.KeyID]
	if !ok {
		return CatalogPayload{}, fmt.Errorf("unknown catalog key %q", envelope.KeyID)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return CatalogPayload{}, fmt.Errorf("catalog key %q has invalid length", envelope.KeyID)
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return CatalogPayload{}, errors.New("invalid catalog signature encoding")
	}
	canonical, err := json.Marshal(envelope.Payload)
	if err != nil {
		return CatalogPayload{}, fmt.Errorf("canonicalize catalog payload: %w", err)
	}
	if !ed25519.Verify(publicKey, canonical, signature) {
		return CatalogPayload{}, errors.New("catalog signature verification failed")
	}
	if err := validateCatalog(envelope.Payload); err != nil {
		return CatalogPayload{}, err
	}
	if v.RequireValidity {
		if err := validateCatalogValidity(envelope.Payload, v.now()); err != nil {
			return CatalogPayload{}, err
		}
	}
	for index, artifact := range envelope.Payload.Artifacts {
		if err := verifyArtifactSignature(publicKey, artifact); err != nil {
			return CatalogPayload{}, fmt.Errorf("artifact %d: %w", index, err)
		}
	}
	return envelope.Payload, nil
}

func (v CatalogVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now().UTC()
	}
	return time.Now().UTC()
}

// officialCatalogTrustAnchorsB64 is injected by release builds with -ldflags.
// It intentionally defaults to empty so development builds fail closed.
var officialCatalogTrustAnchorsB64 string

// OfficialCatalogVerifier constructs the fail-closed verifier used for the
// released catalog. Public keys are configuration, never private signing
// material, and can contain multiple IDs during key rotation overlap.
func OfficialCatalogVerifier() (CatalogVerifier, error) {
	return CatalogVerifierFromTrustAnchorsB64(officialCatalogTrustAnchorsB64)
}

func LoadOfficialCatalog(path string) (CatalogPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CatalogPayload{}, fmt.Errorf("read official catalog: %w", err)
	}
	verifier, err := OfficialCatalogVerifier()
	if err != nil {
		return CatalogPayload{}, err
	}
	return verifier.Verify(data)
}

func validateCatalogValidity(payload CatalogPayload, now time.Time) error {
	if payload.CatalogVersion == "" {
		return errors.New("catalog version is required")
	}
	issuedAt, err := time.Parse(time.RFC3339, payload.IssuedAt)
	if err != nil {
		return fmt.Errorf("catalog issued_at is invalid: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, payload.ExpiresAt)
	if err != nil {
		return fmt.Errorf("catalog expires_at is invalid: %w", err)
	}
	if !expiresAt.After(issuedAt) {
		return errors.New("catalog expiry must be after issued_at")
	}
	if now.Before(issuedAt) {
		return errors.New("catalog is not yet valid")
	}
	if !now.Before(expiresAt) {
		return errors.New("catalog is expired")
	}
	return nil
}

func verifyArtifactSignature(publicKey ed25519.PublicKey, artifact Artifact) error {
	signature, err := base64.StdEncoding.DecodeString(artifact.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("invalid artifact signature encoding")
	}
	canonical, err := artifactSigningBytes(artifact)
	if err != nil {
		return fmt.Errorf("canonicalize artifact signature: %w", err)
	}
	if !ed25519.Verify(publicKey, canonical, signature) {
		return errors.New("artifact signature verification failed")
	}
	return nil
}

func artifactSigningBytes(artifact Artifact) ([]byte, error) {
	artifact.Signature = ""
	return json.Marshal(artifact)
}

func validateCatalog(payload CatalogPayload) error {
	if payload.SchemaVersion != currentCatalogSchema {
		return fmt.Errorf("unsupported catalog schema %d", payload.SchemaVersion)
	}
	if len(payload.Artifacts) == 0 {
		return errors.New("catalog contains no artifacts")
	}
	if len(payload.AllowedHosts) == 0 {
		return errors.New("catalog host allowlist is empty")
	}
	allowedHosts := make(map[string]struct{}, len(payload.AllowedHosts))
	for _, host := range payload.AllowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			return errors.New("catalog host allowlist contains an empty host")
		}
		allowedHosts[host] = struct{}{}
	}
	seen := make(map[string]struct{}, len(payload.Artifacts))
	for index, artifact := range payload.Artifacts {
		if err := validateArtifact(artifact); err != nil {
			return fmt.Errorf("artifact %d: %w", index, err)
		}
		parsedURL, _ := url.Parse(artifact.URL)
		if _, ok := allowedHosts[strings.ToLower(parsedURL.Hostname())]; !ok {
			return fmt.Errorf("artifact %d host %q is outside the catalog allowlist", index, parsedURL.Hostname())
		}
		key := strings.Join([]string{string(artifact.Tool), artifact.Version, artifact.GOOS, artifact.GOARCH}, "/")
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate artifact %s", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateArtifact(artifact Artifact) error {
	if artifact.Tool != PHP && artifact.Tool != Composer && artifact.Tool != RoadRunner {
		return fmt.Errorf("unsupported tool %q", artifact.Tool)
	}
	if err := safePathSegment("version", artifact.Version); err != nil {
		return err
	}
	if err := safePathSegment("operating system", artifact.GOOS); err != nil {
		return err
	}
	if err := safePathSegment("architecture", artifact.GOARCH); err != nil {
		return err
	}
	parsedURL, err := url.Parse(artifact.URL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return errors.New("artifact URL must use HTTPS and include a host")
	}
	if len(artifact.SHA256) != sha256.Size*2 {
		return errors.New("artifact SHA-256 must be 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(strings.ToLower(artifact.SHA256)); err != nil {
		return errors.New("artifact SHA-256 must be hexadecimal")
	}
	if strings.TrimSpace(artifact.Signature) == "" {
		return errors.New("artifact signature is required")
	}
	if artifact.Format != "raw" && artifact.Format != "zip" && artifact.Format != "tar.gz" {
		return fmt.Errorf("unsupported artifact format %q", artifact.Format)
	}
	if strings.TrimSpace(artifact.EntryPoint) == "" {
		return errors.New("artifact entrypoint is required")
	}
	return nil
}
