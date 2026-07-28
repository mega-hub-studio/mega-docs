package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinsParseRealManifest(t *testing.T) {
	p, err := parsePins(manifestSrc)
	if err != nil {
		t.Fatalf("web/vendor.sha384 does not parse: %v", err)
	}
	// Only the design system: the manifest is the *docs pages'* CDN pins now. Vue, marked,
	// DOMPurify and mermaid are npm dependencies of web/ui, bundled into web/dist — pinning
	// them here as well would be two sources of truth for one version.
	if _, ok := p.spec["8bit-nes"]; !ok {
		t.Error("8bit-nes is not pinned")
	}
	for _, pkg := range []string{"vue", "marked", "dompurify", "mermaid"} {
		if _, ok := p.spec[pkg]; ok {
			t.Errorf("%s is pinned here as well as in web/ui/package.json — the app's "+
				"dependencies are bundled, so this manifest is not where their version lives", pkg)
		}
	}
	for _, d := range p.digest {
		if !strings.HasPrefix(d, "sha384-") {
			t.Errorf("digest %q is not a sha384 SRI value", d)
		}
	}
}

// Every file a docs page asks for must be pinned: url/sri return an error for anything
// the manifest does not list, and Execute turns that into a render failure. So rendering
// all four pages *is* the check. (The app has no pins — it is bundled.)
func TestEveryAssetThePagesUseIsPinned(t *testing.T) {
	for _, page := range Pages() {
		if _, err := page.Build("/vendor", StaticNav); err != nil {
			t.Errorf("%s: rendering failed, so some asset is unpinned: %v", page.File, err)
		}
	}
}

func TestPathAndSRIRejectUnpinnedAssets(t *testing.T) {
	p, err := parsePins(manifestSrc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.path("not-a-package", "x.js"); err == nil {
		t.Error("want an error for an unknown package")
	}
	if _, err := p.path("vue", "dist/nope.js"); err == nil {
		t.Error("want an error for a file the manifest doesn't list")
	}
	if _, err := p.sri("vue", "dist/nope.js"); err == nil {
		t.Error("sri must fail for an unlisted file, not return an empty hash")
	}
}

func TestPinsRejectMalformedManifests(t *testing.T) {
	for name, src := range map[string]string{
		"no digest":     "vue@3.5.40/dist/vue.js\n",
		"no path":       "vue@3.5.40 sha384-x\n",
		"no version":    "vue/dist/vue.js sha384-x\n",
		"empty":         "# only a comment\n",
		"double pinned": "vue@3.5.40/a.js sha384-x\nvue@3.6.0/b.js sha384-y\n",
	} {
		if _, err := parsePins(src); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// A populated web/vendor/ must hold exactly what the manifest pins, or a docs page
// rendered with `-base /vendor` (which is how `make check-ui` measures them, and how a
// no-egress preview works) links files that are not there.
//
// Read from disk, not from an embed: the vendor tree is gitignored, it is only ever used
// by tooling, and embedding it would put a second copy of every asset in the binary.
func TestVendorTreeMatchesTheManifest(t *testing.T) {
	if entries, err := os.ReadDir("vendor"); err != nil || len(entries) < 2 {
		t.Skip("vendor/ is empty — run `make vendor`")
	}
	p, err := parsePins(manifestSrc)
	if err != nil {
		t.Fatal(err)
	}
	// keys are "<pkg>@<version>/<path>", which is exactly the layout
	// `make vendor` writes under web/vendor/
	for f := range p.digest {
		if _, err := os.Stat(filepath.Join("vendor", filepath.FromSlash(f))); err != nil {
			t.Errorf("pinned but not vendored: %s", f)
		}
	}
}
