package web

import (
	"io/fs"
	"strings"
	"testing"
)

// Versions are read from the manifest, not written here: this asserts the wiring,
// so bumping a dependency stays a one-line change in web/vendor.sha384.
func TestIndexSubstitutesRemoteAssetBase(t *testing.T) {
	out, err := Index("https://cdn.jsdelivr.net/npm")
	if err != nil {
		t.Fatal(err)
	}
	page := string(out)
	pin := pinnedSpecs(t)

	for _, want := range []string{
		`href="https://cdn.jsdelivr.net/npm/` + pin["8bit-nes"] + `/all.min.css"`,
		`src="https://cdn.jsdelivr.net/npm/` + pin["vue"] + `/dist/vue.global.prod.js"`,
		// a remote base is worth one warmed connection
		`<link rel="preconnect" href="https://cdn.jsdelivr.net" crossorigin>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	// and the digest must travel with it
	if !strings.Contains(page, `integrity="`+pinnedDigest(t, "8bit-nes", "all.min.css")+`"`) {
		t.Error("the stylesheet went out without its manifest digest")
	}
}

func pinnedSpecs(t *testing.T) map[string]string {
	t.Helper()
	p, err := parsePins(manifestSrc)
	if err != nil {
		t.Fatal(err)
	}
	return p.spec
}

func pinnedDigest(t *testing.T, pkg, file string) string {
	t.Helper()
	p, err := parsePins(manifestSrc)
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.sri(pkg, file)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestIndexSubstitutesVendorBaseAndDropsPreconnect(t *testing.T) {
	out, err := Index("/vendor/") // trailing slash must not double up
	if err != nil {
		t.Fatal(err)
	}
	page := string(out)

	if !strings.Contains(page, `href="/vendor/`+pinnedSpecs(t)["8bit-nes"]+`/all.min.css"`) {
		t.Error("vendor path not substituted (or slash doubled)")
	}
	if strings.Contains(page, "preconnect") {
		t.Error("preconnect emitted for a same-origin base")
	}
	if strings.Contains(page, "cdn.jsdelivr.net") {
		t.Error("a CDN URL survived into the vendored page")
	}
}

// The page is a Vue template. Go's delimiters are <% %> precisely so that Vue's
// {{ }} passes through untouched — if this ever regresses, the UI renders literal
// mustaches or the template fails to parse.
func TestIndexLeavesVueInterpolationAlone(t *testing.T) {
	out, err := Index("/vendor")
	if err != nil {
		t.Fatal(err)
	}
	page := string(out)

	for _, want := range []string{"{{ t.q }}", "{{ c.n }}"} {
		if !strings.Contains(page, want) {
			t.Errorf("Vue interpolation %q did not survive rendering", want)
		}
	}
	if strings.Contains(page, "<%") || strings.Contains(page, "%>") {
		t.Error("an unrendered Go action was left in the output")
	}
}

func TestIndexRejectsEmptyBase(t *testing.T) {
	if _, err := Index(""); err == nil {
		t.Error("want an error for an empty asset base")
	}
}

// The app shell must actually be in the binary — a missing embed would only show
// up as a blank page at runtime.
func TestAppShellIsEmbedded(t *testing.T) {
	for _, name := range []string{
		"app/styles.css", "app/app.js", "app/chat.js", "app/answer.js", "app/viewport.js",
	} {
		b, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Errorf("%s not embedded: %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}

// index.html references the shell by path; keep the two in step.
func TestIndexReferencesTheEmbeddedShell(t *testing.T) {
	out, err := Index("/vendor")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="/app/styles.css"`, `"/app/app.js"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("index.html does not reference %s", want)
		}
	}
}

func TestHasVendorReportsEmptyTree(t *testing.T) {
	// A clean checkout has only web/vendor/.gitkeep, so this must be false —
	// that's what makes the startup warning meaningful.
	if HasVendor() {
		t.Skip("vendor/ is populated in this working tree (make vendor has run)")
	}
}
