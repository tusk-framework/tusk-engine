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

func TestReleaseWorkflowUsesTrustedCallerAndProtectsPublishedReleases(t *testing.T) {
	root := repositoryRoot(t)
	workflow := readRepositoryFile(t, root, ".github", "workflows", "release.yml")
	action := readRepositoryFile(t, root, ".github", "actions", "publish-release", "action.yml")
	if strings.Contains(workflow, "workflow_dispatch") || strings.Contains(workflow, "push:") {
		t.Fatal("privileged release workflow must not be directly triggered by a selected ref or tag push")
	}
	if !strings.Contains(workflow, "workflow_call:") {
		t.Fatal("release workflow must only be callable by a trusted workflow")
	}
	for _, expected := range []string{"expected-sha:", "EXPECTED_SHA: ${{ inputs.expected-sha }}", "ref: main"} {
		if !strings.Contains(workflow, expected) {
			t.Errorf("reusable publisher is missing exact source/trusted code contract %q", expected)
		}
	}
	if strings.Contains(workflow, "/commits/main") {
		t.Fatal("reusable publisher must permit a safe retry for an already-tagged SHA after main advances")
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

func TestAutomaticReleaseWorkflowGatesMutationAndPinsDockerSource(t *testing.T) {
	root := repositoryRoot(t)
	workflow := strings.ReplaceAll(readRepositoryFile(t, root, ".github", "workflows", "automatic-release.yml"), "\r\n", "\n")
	docker := strings.ReplaceAll(readRepositoryFile(t, root, ".github", "workflows", "docker-publish.yml"), "\r\n", "\n")
	for _, expected := range []string{
		"workflow_run:",
		"branches:\n      - main",
		"- Go",
		"- Docker validation",
		"- Skeleton smoke",
		"cancel-in-progress: false",
		"HEAD_BRANCH: ${{ github.event.workflow_run.head_branch }}",
		"EVENT_NAME: ${{ github.event.workflow_run.event }}",
		"HEAD_REPOSITORY: ${{ github.event.workflow_run.head_repository.full_name }}",
		"Resolve current main as the release candidate",
		"repos/$GH_REPOSITORY/commits/main",
		"sha: ${{ steps.current-main.outputs.sha }}",
		"Verify latest required checks for the triggering SHA",
		"steps.event-required.outputs.validated",
		"EVENT_VALIDATED: ${{ needs.validate.outputs.event-validated }}",
		"candidate_release\" == \"draft\"",
		"candidate_release\" == \"published\"",
		"only the current validated main SHA may be considered next",
		"release_state_or_fail()",
		"if ! release_state \"$tag\"; then",
		"refusing to plan publication",
		"has(\"draft\") and (.draft | type == \"boolean\")",
		"404) candidate_release=\"absent\"",
		"*) rm -f \"$response_file\"; return 1 ;;",
		"target_sha=\"$EVENT_SHA\"",
		"git show \"$TARGET_SHA:release/toolchain-catalog.payload.json\"",
		"ref: main",
		"uses: ./.github/actions/verify-required-checks",
		"git tag -a",
		"tusk-release-automation:v1",
		"calculate_next_tag()",
		"previous_stable_tag()",
		"tag_is_automation_retry()",
		"tag_is_automation_retry \"$tag\" \"$EVENT_SHA\"",
		"tag_is_automation_retry \"$tag\" \"$target_sha\"",
		"if (( ${#event_tags[@]} > 1 )); then",
		"event stable tag does not match calculated SemVer",
		"event tag lacks the exact automation marker and source SHA",
		"if (( ${#tags_at_sha[@]} > 1 )); then",
		"multiple stable release tags point at the same source commit",
		"grep -Fqx \"source-sha:$target_sha\"",
		"grep -Fqx \"source-sha:$sha\"",
		"go run ./cmd/tusk-release previous-tag --candidate-tag \"$1\" --expected-sha \"$2\"",
		"previous_event_tag",
		"previous_candidate_tag",
		"expected_event_tag",
		"expected_tag",
		"if [[ -n \"$candidate_tag\" ]]; then",
		"calculated release tag is already occupied",
		"existing tag is not an automation-created annotated tag for this exact SHA",
		"concurrent tag is not an automation-created annotated tag for this exact SHA",
		"grep -Fqx 'tusk-release-automation:v1'",
		"git push origin",
		"Preflight official catalog and signing configuration",
		"uses: ./.github/workflows/release.yml",
		"uses: ./.github/workflows/docker-publish.yml",
		"source-ref: ${{ needs.release.outputs.sha }}",
	} {
		if !strings.Contains(workflow, expected) {
			t.Errorf("automatic release workflow is missing contract %q", expected)
		}
	}
	if strings.Contains(workflow, "CONCLUSION: ${{ github.event.workflow_run.conclusion }}") {
		t.Fatal("a workflow_run is a wake-up signal; exact-SHA required-check results, not its own conclusion, gate release")
	}
	if strings.Contains(workflow, "workflow_run:") && !strings.Contains(workflow, "branches:\n      - main") {
		t.Fatal("workflow_run must filter main events to prevent PR completions replacing the pending serialized wake-up")
	}
	if strings.Contains(workflow, "git push --force") || strings.Contains(workflow, "git push -f ") {
		t.Fatal("automatic release workflow must never force-update a stable tag")
	}
	preflight := strings.Index(workflow, "Preflight official catalog and signing configuration")
	tagCreation := strings.Index(workflow, "Create immutable annotated stable tag")
	if preflight < 0 || tagCreation < 0 || preflight > tagCreation {
		t.Fatal("release prerequisites must be checked before creating the immutable tag")
	}
	for _, expected := range []string{"source-ref:", "publish-main:", "ref: ${{ inputs.source-ref }}", "Recheck current main for mutable main tag", "Preserve existing immutable version image tags", "org.opencontainers.image.revision=${{ inputs.source-ref }}", "context: workflow", "steps.current-main.outputs.publish == 'true'", "type=raw,value=main"} {
		if !strings.Contains(docker, expected) {
			t.Errorf("Docker publisher is missing exact-source contract %q", expected)
		}
	}
	for _, expected := range []string{"docker buildx imagetools inspect", "docker buildx imagetools inspect --raw", "docker pull --platform", ".manifests[]?", "does not match the validated source SHA", "Could not safely inspect immutable image", "unauthorized|denied|forbidden|authentication required|permission denied"} {
		if !strings.Contains(docker, expected) {
			t.Errorf("Docker publisher is missing immutable-tag preservation contract %q", expected)
		}
	}
	if !strings.Contains(docker, `\.(0|[1-9][0-9]*))$`) {
		t.Fatal("Docker immutable version tag validation must reject empty or leading-zero SemVer components")
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
