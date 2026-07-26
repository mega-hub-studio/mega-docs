package web

import (
	"io/fs"
	"regexp"
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

func TestDocsRendersForBothAssetBases(t *testing.T) {
	for _, base := range []string{"https://cdn.jsdelivr.net/npm", "/vendor"} {
		out, err := Docs(base, ServedNav)
		if err != nil {
			t.Fatalf("Docs(%q): %v", base, err)
		}
		page := string(out)
		if !strings.Contains(page, base+"/"+pinnedSpecs(t)["8bit-nes"]+"/all.min.css") {
			t.Errorf("Docs(%q) did not resolve the stylesheet from the manifest", base)
		}
		if strings.Contains(page, "<%") {
			t.Errorf("Docs(%q) left an unrendered action", base)
		}
		// Both languages must be present in the markup — the toggle is CSS-only, so
		// if one side is missing it is missing for good.
		for _, want := range []string{`lang="en"`, `lang="vi"`} {
			if !strings.Contains(page, want) {
				t.Errorf("Docs(%q) has no %s content", base, want)
			}
		}
	}
}

// The static build for GitHub Pages must omit the "Open app" button — a static
// host has no app to open — while keeping every asset pinned and SRI-verified.
func TestDocsForStaticHostingOmitsTheAppLink(t *testing.T) {
	out, err := Docs("https://cdn.jsdelivr.net/npm", StaticNav)
	if err != nil {
		t.Fatal(err)
	}
	page := string(out)

	if strings.Contains(page, "Open app") {
		t.Error(`the "Open app" button survived a build with no app link`)
	}
	if strings.Contains(page, "<%") {
		t.Error("an unrendered action was left in the output")
	}
	// Still pinned and still hash-verified: publishing it changes nothing there.
	if !strings.Contains(page, `integrity="`+pinnedDigest(t, "8bit-nes", "all.min.css")+`"`) {
		t.Error("the published page lost its integrity digest")
	}
	// The content itself must be intact — this is the copy the team reads. The
	// split moved operations onto the deploy page, so each page is checked against
	// what it now owns; asserting both here is what catches copy lost in the move.
	deploy, err := Deploy("https://cdn.jsdelivr.net/npm", StaticNav)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		page  string
		wants []string
	}{
		{"guide", page, []string{"Nothing is indexed", "make ingest"}},
		{"deploy", string(deploy), []string{"tailscale serve", "EMBED_BASE_URL"}},
	} {
		for _, want := range tc.wants {
			if !strings.Contains(tc.page, want) {
				t.Errorf("published %s page is missing %q", tc.name, want)
			}
		}
	}
}

// The guide is two pages sharing one head partial. Each must render whole, link to
// the other, and mark itself current — a broken partial would show up as a page
// with no <head> rather than as a test failure elsewhere.
func TestBothGuidePagesRenderAndCrossLink(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(string, Nav) ([]byte, error)
		other string
		owns  string
	}{
		{"guide", Docs, "Deploy", "Ask your own docs"},
		{"deploy", Deploy, "Guide", "Run it for the team"},
	} {
		out, err := tc.build("/vendor", ServedNav)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		page := string(out)

		// the shared partial actually landed
		for _, want := range []string{"<!DOCTYPE html>", "<style>", "</html>", `id="lang"`} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: missing %q — is the head/foot partial wired?", tc.name, want)
			}
		}
		if c := strings.Count(page, "<main>"); c != 1 {
			t.Errorf("%s: %d <main> elements", tc.name, c)
		}
		if !strings.Contains(page, "</main>") {
			t.Errorf("%s: <main> was never closed", tc.name)
		}
		if !strings.Contains(page, tc.owns) {
			t.Errorf("%s: lost its own content (%q)", tc.name, tc.owns)
		}
		if !strings.Contains(page, ">"+tc.other+"<") {
			t.Errorf("%s: no link to %s", tc.name, tc.other)
		}
		if strings.Contains(page, "<%") {
			t.Errorf("%s: unrendered action", tc.name)
		}
		// the deploy content must live in exactly one page
		hasDeploy := strings.Contains(page, "tailscale serve")
		if (tc.name == "deploy") != hasDeploy {
			t.Errorf("%s: deploy instructions in the wrong page", tc.name)
		}
	}
}

// <meta name="theme-color"> colours the browser's own chrome, which sits directly
// above the sticky .bar — so it has to equal the token the bar is painted with. It
// is the one value that cannot be written as var(--bg): meta content takes no CSS.
// That makes it the one value that can drift silently when the design system moves,
// which is what this checks.
//
// The token is read from the pinned 8bit-nes stylesheet, so it compares against the
// exact bytes the page loads. That file arrives via `make vendor` and is never
// committed, so the check skips rather than lies when it is absent.
func TestThemeColorMatchesTheBarToken(t *testing.T) {
	css, err := fs.ReadFile(FS, "vendor/8bit-nes@0.6.0/all.min.css")
	if err != nil {
		t.Skip("8bit-nes stylesheet not vendored — run `make vendor` to enable this check")
	}
	m := regexp.MustCompile(`--bg:\s*(#[0-9a-fA-F]{3,8})`).FindSubmatch(css)
	if m == nil {
		t.Fatal("no --bg token in the pinned stylesheet: has the design system renamed it?")
	}
	token := strings.ToLower(string(m[1]))

	// .bar is painted with --bg in docsbase.html; index.html carries its own copy of
	// the same bar, so both templates are checked.
	for name, src := range map[string]string{"docsbase.html": docsBaseTmpl, "index.html": indexTmpl} {
		got := regexp.MustCompile(`name="theme-color" content="(#[0-9a-fA-F]{3,8})"`).FindStringSubmatch(src)
		if got == nil {
			t.Errorf("%s has no theme-color meta", name)
			continue
		}
		if strings.ToLower(got[1]) != token {
			t.Errorf("%s theme-color is %s but the bar is painted --bg = %s — the browser chrome "+
				"will not match the top of the page", name, got[1], token)
		}
	}
}

// Static hosting links by file name; the served binary links by route.
func TestNavAddressesMatchTheHost(t *testing.T) {
	static, err := Deploy("https://cdn.jsdelivr.net/npm", StaticNav)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(static), `href="./index.html"`) {
		t.Error("static build should link the guide by file name")
	}
	served, err := Deploy("/vendor", ServedNav)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(served), `href="/docs"`) {
		t.Error("served build should link the guide by route")
	}
}
