package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Nur-Adnan/duster/lib/fs"
)

// scanArtifacts must flag real build output and keep look-alikes that are not
// disposable: global npm/Maven/Gradle stores, installed apps under AppData, and
// ordinary folders that merely share an artifact's name.
func TestScanArtifactsRequiresProjectMarkers(t *testing.T) {
	root := t.TempDir()
	mk := func(p string) {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	touch := func(p string) {
		if err := os.WriteFile(filepath.Join(root, p), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Flagged: artifacts next to their project manifest.
	mk("web/node_modules/react")
	touch("web/package.json")
	mk("rs/target/debug")
	touch("rs/Cargo.toml")
	mk("java/.gradle/caches")
	touch("java/build.gradle")

	// Kept: no manifest, or inside AppData.
	mk("npm-global/node_modules/pkg")
	mk("home/.m2/repository")
	mk("home/.gradle/caches")
	mk("docs/build/assets")
	mk("AppData/Local/Programs/app/resources/app/node_modules/x")
	touch("AppData/Local/Programs/app/resources/app/package.json")

	calls := 0
	got, err := scanArtifacts(root, func(DiscoveredArtifact, int) { calls++ })
	if err != nil {
		t.Fatalf("scanArtifacts: %v", err)
	}
	// The walk expands an 8.3 root (the Windows runner's RUNNER~1) to its long
	// form, so artifact paths are relative to that.
	base := fs.LongPath(root)
	var rel []string
	for _, a := range got {
		r, _ := filepath.Rel(base, a.Path)
		rel = append(rel, filepath.ToSlash(r))
	}
	sort.Strings(rel)
	want := []string{"java/.gradle", "rs/target", "web/node_modules"}
	if len(rel) != len(want) {
		t.Fatalf("got %v, want %v", rel, want)
	}
	for i := range want {
		if rel[i] != want[i] {
			t.Fatalf("got %v, want %v", rel, want)
		}
	}
	if calls != len(want) {
		t.Errorf("onFound called %d times, want %d", calls, len(want))
	}
}

// An explicitly targeted AppData root is still scanned.
func TestScanArtifactsScansExplicitAppDataRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AppData")
	if err := os.MkdirAll(filepath.Join(root, "proj", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "proj", "package.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := scanArtifacts(root, nil)
	if len(got) != 1 {
		t.Fatalf("expected the node_modules under an explicit AppData root, got %v", got)
	}
}
