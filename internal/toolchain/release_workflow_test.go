package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseWorkflowRequiresCatalogSigningAndAttestationSteps(t *testing.T) {
	root := repositoryRoot(t)
	workflow := readRepositoryFile(t, root, ".github", "workflows", "release.yml")
	required := []string{
		"contents: write",
		"id-token: write",
		"attestations: write",
		"TUSK_CATALOG_SIGNING_KEY_B64: ${{ secrets.TUSK_CATALOG_SIGNING_KEY_B64 }}",
		"TUSK_CATALOG_KEY_ID: ${{ vars.TUSK_CATALOG_KEY_ID }}",
		"TUSK_CATALOG_TRUST_ANCHORS_B64: ${{ vars.TUSK_CATALOG_TRUST_ANCHORS_B64 }}",
		"release/toolchain-catalog.payload.json",
		"go run ./cmd/tusk-catalog validate --download",
		"go run ./cmd/tusk-catalog sign",
		"go run ./cmd/tusk-catalog verify",
		"actions/attest@v4",
	}
	for _, expected := range required {
		if !strings.Contains(workflow, expected) {
			t.Errorf("release workflow is missing %q", expected)
		}
	}
	if strings.Contains(strings.ToLower(workflow), "private.key") || strings.Contains(strings.ToLower(workflow), "private-key") {
		t.Fatal("release workflow references a private-key file")
	}
	if strings.Index(workflow, "go run ./cmd/tusk-catalog sign") > strings.Index(workflow, "Run GoReleaser") {
		t.Fatal("catalog signing must happen before GoReleaser")
	}
	if strings.Index(workflow, "go run ./cmd/tusk-catalog validate") > strings.Index(workflow, "go run ./cmd/tusk-catalog sign") {
		t.Fatal("catalog payload validation must happen before signing")
	}
}

func TestCommittedCatalogPayloadIsValidAndHasDeclaredReleaseMatrix(t *testing.T) {
	root := repositoryRoot(t)
	data := []byte(readRepositoryFile(t, root, "release", "toolchain-catalog.payload.json"))
	payload, err := DecodeCatalogPayload(data)
	if err != nil {
		t.Fatalf("DecodeCatalogPayload() error = %v", err)
	}
	if err := validateCatalogForSigning(payload); err != nil {
		t.Fatalf("validateCatalogForSigning() error = %v", err)
	}
	if len(payload.Artifacts) != 11 {
		t.Fatalf("payload artifact count = %d, want 11", len(payload.Artifacts))
	}
	seen := make(map[string]bool)
	for _, artifact := range payload.Artifacts {
		target, err := artifactPlatform(artifact)
		if err != nil {
			t.Fatal(err)
		}
		seen[string(artifact.Tool)+"/"+target.OS+"/"+target.Arch] = true
	}
	for _, required := range []string{"php/windows/amd64", "composer/linux/amd64", "composer/darwin/arm64", "roadrunner/windows/amd64", "roadrunner/linux/arm64", "roadrunner/darwin/amd64"} {
		if !seen[required] {
			t.Errorf("payload matrix is missing %q", required)
		}
	}
}

func TestGoReleaserPublishesCatalogAssetsAndInjectsPublicAnchors(t *testing.T) {
	root := repositoryRoot(t)
	config := readRepositoryFile(t, root, ".goreleaser.yaml")
	for _, expected := range []string{
		"officialCatalogTrustAnchorsB64",
		"TUSK_CATALOG_TRUST_ANCHORS_B64",
		"release/toolchain-catalog.json",
		"release/toolchain-catalog.provenance.json",
	} {
		if !strings.Contains(config, expected) {
			t.Errorf("GoReleaser config is missing %q", expected)
		}
	}
}

func TestReleaseDocumentationCoversSecretHandlingRotationAndRevocation(t *testing.T) {
	root := repositoryRoot(t)
	documentation := strings.ToLower(readRepositoryFile(t, root, "release", "README.md"))
	for _, expected := range []string{"tusk_catalog_signing_key_b64", "rotation", "overlap", "revocation", "private key", "payload", "trust anchor"} {
		if !strings.Contains(documentation, expected) {
			t.Errorf("release documentation is missing %q", expected)
		}
	}
	readme := readRepositoryFile(t, root, "README.md")
	if !strings.Contains(readme, "release/README.md") {
		t.Fatal("top-level README does not link to catalog release operations")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readRepositoryFile(t *testing.T, root string, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
	if err != nil {
		t.Fatalf("read repository file %s: %v", filepath.Join(parts...), err)
	}
	return string(data)
}
