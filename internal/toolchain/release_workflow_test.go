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
	action := readRepositoryFile(t, root, ".github", "actions", "publish-release", "action.yml")
	required := []string{
		"contents: write",
		"id-token: write",
		"attestations: write",
		"TUSK_CATALOG_SIGNING_KEY_B64: ${{ secrets.TUSK_CATALOG_SIGNING_KEY_B64 }}",
		"TUSK_CATALOG_KEY_ID: ${{ vars.TUSK_CATALOG_KEY_ID }}",
		"TUSK_CATALOG_TRUST_ANCHORS_B64: ${{ vars.TUSK_CATALOG_TRUST_ANCHORS_B64 }}",
	}
	for _, expected := range required {
		if !strings.Contains(workflow, expected) {
			t.Errorf("release workflow is missing %q", expected)
		}
	}
	actionRequired := []string{
		"release/toolchain-catalog.payload.json",
		"go run ./cmd/tusk-catalog validate --download",
		"go run ./cmd/tusk-catalog sign",
		"go run ./cmd/tusk-catalog verify",
		"actions/attest@v4",
	}
	for _, expected := range actionRequired {
		if !strings.Contains(action, expected) {
			t.Errorf("publish action is missing %q", expected)
		}
	}
	if strings.Contains(strings.ToLower(workflow+action), "private.key") || strings.Contains(strings.ToLower(workflow+action), "private-key") {
		t.Fatal("release workflow references a private-key file")
	}
	if strings.Index(action, "go run ./cmd/tusk-catalog sign") > strings.Index(action, "Run GoReleaser") {
		t.Fatal("catalog signing must happen before GoReleaser")
	}
	if strings.Index(action, "go run ./cmd/tusk-catalog validate") > strings.Index(action, "go run ./cmd/tusk-catalog sign") {
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

func TestReleaseWorkflowBindsSignedCatalogDigestIntoBinaries(t *testing.T) {
	root := repositoryRoot(t)
	actionText := readRepositoryFile(t, root, ".github", "actions", "publish-release", "action.yml")
	compute := strings.Index(actionText, "sha256sum release/toolchain-catalog.json")
	release := strings.Index(actionText, "Run GoReleaser")
	if compute < 0 || release < 0 || compute > release {
		t.Fatal("release workflow must compute the signed catalog digest before GoReleaser")
	}
	if !strings.Contains(actionText, "TUSK_CATALOG_SHA256=$digest") || !strings.Contains(actionText, "[[ \"$digest\" =~ ^[a-f0-9]{64}$ ]]") {
		t.Fatal("release workflow must validate and export the lowercase SHA-256 digest")
	}

	goreleaser := readRepositoryFile(t, root, ".goreleaser.yaml")
	if !strings.Contains(string(goreleaser), "toolchain.officialCatalogSHA256={{ .Env.TUSK_CATALOG_SHA256 }}") {
		t.Fatal("GoReleaser must embed the official catalog digest in release binaries")
	}
}

func TestReleaseWorkflowUsesTrustedTagTriggerAndProtectsPublishedReleases(t *testing.T) {
	root := repositoryRoot(t)
	workflow := readRepositoryFile(t, root, ".github", "workflows", "release.yml")
	action := readRepositoryFile(t, root, ".github", "actions", "publish-release", "action.yml")
	if strings.Contains(workflow, "workflow_dispatch") {
		t.Fatal("release workflow must not run privileged publication code from a manually selected ref")
	}
	for _, expected := range []string{
		"Guard already-published release",
		"if [[ -z \"$GH_TOKEN\" ]]",
		"https://api.github.com/repos/$GH_REPOSITORY/releases/tags/$RELEASE_TAG",
		"already-published=true",
		"if: steps.published-guard.outputs.already-published != 'true'",
	} {
		if !strings.Contains(action, expected) {
			t.Errorf("publisher is missing published-release safety contract %q", expected)
		}
	}
	if strings.Count(action, "if: steps.published-guard.outputs.already-published != 'true'") < 7 {
		t.Fatal("all signing, attestation, digest, and GoReleaser steps must be skipped for a published release")
	}
	if strings.Index(action, "Guard already-published release") > strings.Index(action, "go run ./cmd/tusk-catalog sign") {
		t.Fatal("published-release guard must run before signing")
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
