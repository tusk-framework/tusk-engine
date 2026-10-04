package toolchain

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const catalogProvenanceSchema = 1

// CatalogProvenance records the immutable inputs and signing identity used to
// produce an official catalog. It intentionally contains no signing material.
type CatalogProvenance struct {
	SchemaVersion int    `json:"schema_version"`
	KeyID         string `json:"key_id"`
	PayloadSHA256 string `json:"payload_sha256"`
	CatalogSHA256 string `json:"catalog_sha256"`
}

// DecodeCatalogPayload decodes a release payload while rejecting unknown or
// trailing JSON so the signed bytes are unambiguous.
func DecodeCatalogPayload(data []byte) (CatalogPayload, error) {
	var payload CatalogPayload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return CatalogPayload{}, fmt.Errorf("decode catalog payload: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return CatalogPayload{}, errors.New("decode catalog payload: trailing JSON")
		}
		return CatalogPayload{}, fmt.Errorf("decode catalog payload: %w", err)
	}
	return payload, nil
}

// ParseTrustAnchorsB64 decodes the public trust-anchor document used by
// release tooling and official Engine builds.
func ParseTrustAnchorsB64(encoded string) (map[string]ed25519.PublicKey, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, errors.New("official catalog trust anchor is not configured")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode official catalog trust anchor: %w", err)
	}
	var document struct {
		Keys map[string]string `json:"keys"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode official catalog trust anchor: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("decode official catalog trust anchor: trailing JSON")
		}
		return nil, fmt.Errorf("decode official catalog trust anchor: %w", err)
	}
	if len(document.Keys) == 0 {
		return nil, errors.New("official catalog trust anchor contains no keys")
	}
	keys := make(map[string]ed25519.PublicKey, len(document.Keys))
	for keyID, encodedKey := range document.Keys {
		if strings.TrimSpace(keyID) == "" {
			return nil, errors.New("official catalog trust anchor contains an empty key ID")
		}
		publicKey, err := base64.StdEncoding.DecodeString(encodedKey)
		if err != nil || len(publicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("official catalog trust anchor key %q is invalid", keyID)
		}
		keys[keyID] = append(ed25519.PublicKey(nil), publicKey...)
	}
	return keys, nil
}

// CatalogVerifierFromTrustAnchorsB64 constructs the official fail-closed
// verifier from a base64-encoded public trust-anchor document.
func CatalogVerifierFromTrustAnchorsB64(encoded string) (CatalogVerifier, error) {
	keys, err := ParseTrustAnchorsB64(encoded)
	if err != nil {
		return CatalogVerifier{}, err
	}
	return CatalogVerifier{PublicKeys: keys, RequireValidity: true}, nil
}

// SignCatalog signs the canonical payload and every artifact with the same
// Ed25519 key. The key must be present under keyID in trustAnchors, preventing
// a release from producing a catalog the shipped Engine cannot verify.
func SignCatalog(payload CatalogPayload, keyID string, privateKey ed25519.PrivateKey, trustAnchors map[string]ed25519.PublicKey) ([]byte, error) {
	if strings.TrimSpace(keyID) == "" {
		return nil, errors.New("catalog key ID is required")
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("catalog signing private key has invalid length")
	}
	publicKey, ok := trustAnchors[keyID]
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("catalog signing key %q is not present in the trust anchor", keyID)
	}
	derivedPublicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok || !bytes.Equal(derivedPublicKey, publicKey) {
		return nil, fmt.Errorf("catalog signing key %q does not match the trust anchor", keyID)
	}
	if err := validateCatalogForSigning(payload); err != nil {
		return nil, err
	}

	signedPayload := payload
	signedPayload.Artifacts = append([]Artifact(nil), payload.Artifacts...)
	for index := range signedPayload.Artifacts {
		canonical, err := artifactSigningBytes(signedPayload.Artifacts[index])
		if err != nil {
			return nil, fmt.Errorf("canonicalize artifact %d: %w", index, err)
		}
		signedPayload.Artifacts[index].Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	}
	canonicalPayload, err := json.Marshal(signedPayload)
	if err != nil {
		return nil, fmt.Errorf("canonicalize catalog payload: %w", err)
	}
	envelope := SignedCatalog{
		Payload:   signedPayload,
		KeyID:     keyID,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonicalPayload)),
	}
	return json.Marshal(envelope)
}

// GenerateCatalogProvenance emits deterministic JSON for the exact payload
// and signed catalog bytes supplied by the release process.
func GenerateCatalogProvenance(payloadBytes, catalogBytes []byte, keyID string) ([]byte, error) {
	if strings.TrimSpace(keyID) == "" {
		return nil, errors.New("catalog key ID is required")
	}
	payloadDigest := sha256.Sum256(payloadBytes)
	catalogDigest := sha256.Sum256(catalogBytes)
	return json.Marshal(CatalogProvenance{
		SchemaVersion: catalogProvenanceSchema,
		KeyID:         keyID,
		PayloadSHA256: hex.EncodeToString(payloadDigest[:]),
		CatalogSHA256: hex.EncodeToString(catalogDigest[:]),
	})
}

func validateCatalogForSigning(payload CatalogPayload) error {
	if err := validateCatalogReleaseMetadata(payload); err != nil {
		return err
	}
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
		unsigned := artifact
		unsigned.Signature = "release-signature-placeholder"
		if err := validateArtifact(unsigned); err != nil {
			return fmt.Errorf("artifact %d: %w", index, err)
		}
		parsedURL, err := url.Parse(artifact.URL)
		if err != nil {
			return fmt.Errorf("artifact %d: parse URL: %w", index, err)
		}
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

func validateCatalogReleaseMetadata(payload CatalogPayload) error {
	if payload.CatalogVersion == "" {
		return errors.New("catalog version is required")
	}
	issuedAt, err := parseRFC3339("issued_at", payload.IssuedAt)
	if err != nil {
		return err
	}
	expiresAt, err := parseRFC3339("expires_at", payload.ExpiresAt)
	if err != nil {
		return err
	}
	if !expiresAt.After(issuedAt) {
		return errors.New("catalog expiry must be after issued_at")
	}
	return nil
}

func parseRFC3339(field, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("catalog %s is invalid: %w", field, err)
	}
	return parsed, nil
}
