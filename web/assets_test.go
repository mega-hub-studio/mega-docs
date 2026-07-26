package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestPinsParseRealManifest(t *testing.T) {
	p, err := parsePins(manifestSrc)
	if err != nil {
		t.Fatalf("web/vendor.sha384 does not parse: %v", err)
	}
	for _, pkg := range []string{"8bit-nes", "vue", "marked", "dompurify"} {
		if _, ok := p.spec[pkg]; !ok {
			t.Errorf("%s is not pinned", pkg)
		}
	}
	for _, d := range p.digest {
		if !strings.HasPrefix(d, "sha384-") {
			t.Errorf("digest %q is not a sha384 SRI value", d)
		}
	}
}

// Every file index.html asks for must be pinned, and every pinned file must be
// something `make vendor` can place — the two lists cannot drift apart.
func TestEveryAssetThePageUsesIsPinned(t *testing.T) {
	if _, err := Index("/vendor"); err != nil {
		t.Fatalf("rendering failed, so some asset is unpinned: %v", err)
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

// A populated web/vendor/ must hold exactly what the manifest pins, or
// ASSET_BASE=/vendor serves 404s for assets the page believes are local.
func TestVendorTreeMatchesTheManifest(t *testing.T) {
	if !HasVendor() {
		t.Skip("vendor/ is empty — run `make vendor`")
	}
	p, err := parsePins(manifestSrc)
	if err != nil {
		t.Fatal(err)
	}
	// keys are "<pkg>@<version>/<path>", which is exactly the layout
	// `make vendor` writes under web/vendor/
	for f := range p.digest {
		if _, err := fs.Stat(FS, "vendor/"+f); err != nil {
			t.Errorf("pinned but not vendored: %s", f)
		}
	}
}
