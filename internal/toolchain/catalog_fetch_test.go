package toolchain

import "testing"

func TestNormalizeReleaseVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{name: "stable release", version: "0.3.2", want: "0.3.2"},
		{name: "empty", version: "", wantErr: true},
		{name: "development build", version: "dev", wantErr: true},
		{name: "tag prefix", version: "v0.3.2", wantErr: true},
		{name: "malformed", version: "three", wantErr: true},
		{name: "prerelease", version: "0.3.2-rc.1", wantErr: true},
		{name: "leading zero", version: "00.3.2", wantErr: true},
		{name: "whitespace", version: " 0.3.2", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeReleaseVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeReleaseVersion(%q) error = %v, wantErr %t", tt.version, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NormalizeReleaseVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestReleaseCatalogURL(t *testing.T) {
	got, err := ReleaseCatalogURL("0.3.2")
	if err != nil {
		t.Fatalf("ReleaseCatalogURL() error = %v", err)
	}
	const want = "https://github.com/tusk-framework/tusk-engine/releases/download/v0.3.2/toolchain-catalog.json"
	if got != want {
		t.Fatalf("ReleaseCatalogURL() = %q, want %q", got, want)
	}

	if _, err := ReleaseCatalogURL("dev"); err == nil {
		t.Fatal("ReleaseCatalogURL(\"dev\") succeeded, want invalid release version error")
	}
}
