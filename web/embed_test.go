package web

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Versions are read from the manifest, not written here: this asserts the wiring,
// so bumping a dependency stays a one-line change in web/vendor.sha384.
func TestIndexSubstitutesRemoteAssetBase(t *testing.T) {
	out, err := Index("https://cdn.jsdelivr.net/npm", "https://example.test/docs")
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
	out, err := Index("/vendor/", "https://example.test/docs") // trailing slash must not double up
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
	out, err := Index("/vendor", "https://example.test/docs")
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
	if _, err := Index("", "https://example.test/docs"); err == nil {
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
	out, err := Index("/vendor", "https://example.test/docs")
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
		out, err := Docs(base, StaticNav)
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

// The guide is documentation on a public domain; the app lives behind a tailnet or
// a tunnel. So no guide page may link to an app instance — a public page must not
// carry somebody's private address — while every asset stays pinned and SRI-verified.
func TestGuideNeverLinksAnAppInstance(t *testing.T) {
	out, err := Docs("https://cdn.jsdelivr.net/npm", StaticNav)
	if err != nil {
		t.Fatal(err)
	}
	page := string(out)

	// Asserted on markup, not on the words: the phrase also appears in the shipped
	// stylesheet's comments, so a bare substring check passes and fails for the wrong
	// reasons. What must be absent is any link to a served app.
	for _, bad := range []string{`href="/"`, "ts.net", ":8443"} {
		if strings.Contains(page, bad) {
			t.Errorf("guide page links an app instance (%q) — that address is private", bad)
		}
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
		{"dev", Dev, "Deploy", "Change it"},
		{"deploy", Deploy, "Guide", "Run it for the team"},
	} {
		out, err := tc.build("/vendor", StaticNav)
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
	// The path is built from the manifest, not written here, so bumping the design
	// system stays a one-line change in web/vendor.sha384.
	css, err := fs.ReadFile(FS, "vendor/"+pinnedSpecs(t)["8bit-nes"]+"/all.min.css")
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

// The diagram is generated output that is committed, so the failure mode is a
// .mmd edited without re-rendering: the page would keep showing the old picture
// and nothing would complain. gen-diagram.mjs stamps the source hash into the SVG,
// so that drift is detectable without needing mermaid here — which is the point,
// since neither CI nor a normal build installs it.
func TestCommittedDiagramsMatchTheirSource(t *testing.T) {
	sources, err := fs.Glob(diagramFS, "*.mmd")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no .mmd sources embedded (%v)", err)
	}
	for _, src := range sources {
		name := strings.TrimSuffix(src, ".mmd")
		mmd, err := diagramFS.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		svg, err := diagramFS.ReadFile(name + ".svg")
		if err != nil {
			t.Errorf("%s has no rendered %s.svg — run `make diagram`", src, name)
			continue
		}
		want := fmt.Sprintf("%x", sha256.Sum256(mmd))[:16]
		if !strings.Contains(string(svg), "mmd-sha256: "+want) {
			t.Errorf("%s.svg was rendered from a different %s — run `make diagram` "+
				"(want stamp %s)", name, src, want)
		}
	}
}

// The published site is file-based, so pages address each other by file name.
func TestGuidePagesLinkEachOtherByFileName(t *testing.T) {
	out, err := Deploy("https://cdn.jsdelivr.net/npm", StaticNav)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `href="./index.html"`) {
		t.Error("pages should link each other by file name on a static host")
	}
}

// Every page in Nav must be addressable and distinct on both hosts. Adding a role
// means adding a field here, and the easy mistake is leaving one of the two Navs
// unset — which renders href="" and silently links to the current page.
func TestEveryNavAddressIsSetAndDistinct(t *testing.T) {
	for host, nav := range map[string]Nav{"static": StaticNav} {
		seen := map[string]string{}
		for label, addr := range map[string]string{
			"Guide": nav.Guide, "Dev": nav.Dev, "Deploy": nav.Deploy,
		} {
			if addr == "" {
				t.Errorf("%s nav: %s has no address", host, label)
				continue
			}
			if other, dup := seen[addr]; dup {
				t.Errorf("%s nav: %s and %s both point at %q", host, label, other, addr)
			}
			seen[addr] = label
		}
	}
	// …and each rendered page must actually link the other two, not just hold the
	// addresses. A router that points nowhere is the whole failure mode here.
	for name, build := range map[string]func(string, Nav) ([]byte, error){
		"guide": Docs, "dev": Dev, "deploy": Deploy,
	} {
		out, err := build("/vendor", StaticNav)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, addr := range []string{StaticNav.Guide, StaticNav.Dev, StaticNav.Deploy} {
			if !strings.Contains(string(out), `href="`+addr+`"`) {
				t.Errorf("%s page never links %s", name, addr)
			}
		}
	}
}

// AGENTS.md tells an agent which 8bit-nes docs to read, and points at a
// version-exact URL rather than the unversioned docs site. That version is quoted
// prose, and quoted prose drifts from generated pins — which is exactly how 8bit-nes
// 0.7.0 shipped a README pinning @0.7.0 with 0.6.1's digests. Same class of bug, so
// same guard.
func TestAgentNotesPinMatchesTheManifest(t *testing.T) {
	notes, err := os.ReadFile("../AGENTS.md")
	if err != nil {
		t.Fatalf("AGENTS.md is the entry point for agents; it must exist: %v", err)
	}
	spec := pinnedSpecs(t)["8bit-nes"] // e.g. "8bit-nes@0.7.0"
	text := string(notes)

	// Every 8bit-nes version mentioned must be the pinned one — a stale URL sends the
	// reader to docs for CSS this repo does not load.
	found := regexp.MustCompile(`8bit-nes@[0-9]+\.[0-9]+\.[0-9]+`).FindAllString(text, -1)
	if len(found) == 0 {
		t.Fatal("AGENTS.md quotes no 8bit-nes version at all")
	}
	for _, got := range found {
		if got != spec {
			t.Errorf("AGENTS.md points at %s but web/vendor.sha384 pins %s — an agent would "+
				"read docs for a version this repo does not load", got, spec)
		}
	}

	// And it must name the version-exact source, not only the docs site, which is
	// always latest.
	if !strings.Contains(text, "cdn.jsdelivr.net/npm/"+spec+"/llms.txt") {
		t.Errorf("AGENTS.md does not link the pinned llms.txt for %s", spec)
	}
}

// llms.txt is what an agent reads first, so it must actually list every published
// page and be derived from them rather than hand-kept.
func TestLLMsIndexListsEveryPage(t *testing.T) {
	out, err := LLMs("https://example.test/docs/")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)

	for _, want := range []string{"index.html", "dev.html", "deploy.html"} {
		if !strings.Contains(got, "https://example.test/docs/"+want) {
			t.Errorf("llms.txt never links %s", want)
		}
	}
	// Section names come out of the pages, so a page's own heading must appear.
	for _, want := range []string{"Using it well", "The two seams", "Let the team in"} {
		if !strings.Contains(got, want) {
			t.Errorf("llms.txt is missing the %q section — is it still derived from the pages?", want)
		}
	}
	// No markup, no template actions, no trailing slash doubling.
	for _, bad := range []string{"<", "<%", "docs//"} {
		if strings.Contains(got, bad) {
			t.Errorf("llms.txt contains %q — it should be plain text", bad)
		}
	}
	if _, err := LLMs(""); err == nil {
		t.Error("want an error for an empty site base")
	}
}
