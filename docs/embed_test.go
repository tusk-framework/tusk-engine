package docs

import (
	"io/fs"
	"testing"
)

func TestUserGuideContainsEveryPublishedTopic(t *testing.T) {
	want := []string{
		"user-guide/index.md",
		"user-guide/toolchain.md",
		"user-guide/php-extensions.md",
		"user-guide/composer.md",
		"user-guide/runtime.md",
		"user-guide/security.md",
		"user-guide/ci-docker.md",
		"user-guide/upgrading.md",
	}
	for _, path := range want {
		if _, err := fs.Stat(UserGuide, path); err != nil {
			t.Errorf("embedded guide %q is unavailable: %v", path, err)
		}
	}
}

func TestUserGuideDoesNotEmbedInternalPlanningDocuments(t *testing.T) {
	for _, path := range []string{
		"superpowers/specs/2026-10-09-managed-php-toolchain-and-cli-docs-design.md",
		"superpowers/plans/2026-10-09-offline-cli-documentation.md",
	} {
		if _, err := fs.Stat(UserGuide, path); err == nil {
			t.Errorf("internal planning document %q must not be embedded", path)
		}
	}
}
