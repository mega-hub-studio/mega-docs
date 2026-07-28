package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ── the built app ─────────────────────────────────────────────────────────────
// web/dist is produced by Vite (`make ui`) and committed, so the binary embeds a build
// artefact. That buys `go build` with no Node, and it costs the one failure mode of any
// committed generated file: going stale. These four tests are that trade, paid.

// TestBuiltAppIsEmbedded: a missing embed would otherwise show up as a blank page at
// runtime, on the deployed instance, with nothing in the log.
func TestBuiltAppIsEmbedded(t *testing.T) {
	index, err := Index()
	if err != nil {
		t.Fatalf("no built app embedded: %v", err)
	}
	page := string(index)
	if !strings.Contains(page, `<div id="app">`) {
		t.Error("index.html has no mount point")
	}
	// Vite writes the entry as a module script with a hashed name. Both halves matter:
	// module because the app is ESM, hashed because /assets/ is served immutable.
	m := regexp.MustCompile(`<script type="module"[^>]*src="(/assets/[^"]+\.js)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("index.html does not load a hashed module from /assets/")
	}
	for _, want := range []string{m[1], "build.json"} {
		if _, err := fs.Stat(FS, strings.TrimPrefix(want, "/")); err != nil {
			t.Errorf("index.html names %s but it is not in the embedded build: %v", want, err)
		}
	}
	// Nothing may load from a CDN any more: the bundle is same-origin, and an
	// integrity attribute on a same-origin file pins bytes to themselves.
	if strings.Contains(page, "cdn.jsdelivr.net") || strings.Contains(page, "integrity=") {
		t.Error("the built page reaches for a CDN — the app is bundled and self-hosted")
	}
}

// TestBuiltUIMatchesItsSources is the freshness check. Vite stamps dist/build.json with a
// hash of everything it built from; this recomputes it from the tree. It fails when
// somebody edits an SFC and forgets `make ui` — which is exactly how a committed bundle
// rots, silently, while the binary keeps serving last week's app.
func TestBuiltUIMatchesItsSources(t *testing.T) {
	build, err := BuildInfo()
	if err != nil {
		t.Fatal(err)
	}
	got := uiSourceHash(t)
	if got != build.Sources {
		t.Errorf("web/dist was built from %s but web/ui is now %s — run `make ui`",
			build.Sources[:8], got[:8])
	}
}

// uiSourceHash mirrors web/ui/scripts/stamp.js exactly: the same files, the same order,
// path then bytes, each NUL-terminated. Two implementations of one hash is the cost of
// checking a JS build from Go without running Node — and the reason the algorithm is
// three lines rather than clever.
func uiSourceHash(t *testing.T) string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(filepath.Join("ui", "src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking web/ui/src: %v", err)
	}
	for _, extra := range []string{"index.html", "vite.config.js", "package-lock.json"} {
		files = append(files, filepath.Join("ui", extra))
	}
	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		rel, err := filepath.Rel("ui", f)
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// The pins are the *docs pages* now — the app has no CDN. Versions are read from the
// manifest rather than written here, so bumping one stays a one-line change.
func TestDocsPagesLoadPinnedAssetsFromTheCDN(t *testing.T) {
	out, err := Docs("https://cdn.jsdelivr.net/npm", StaticNav)
	if err != nil {
		t.Fatal(err)
	}
	page := string(out)
	pin := pinnedSpecs(t)
	for _, want := range []string{
		`href="https://cdn.jsdelivr.net/npm/` + pin["8bit-nes"] + `/all.min.css"`,
		// a remote base is worth one warmed connection
		`<link rel="preconnect" href="https://cdn.jsdelivr.net" crossorigin>`,
		`integrity="` + pinnedDigest(t, "8bit-nes", "all.min.css") + `"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
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
		{"ba", BA, "Deploy", "BA mode"},
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
	css, err := os.ReadFile(filepath.Join("vendor", pinnedSpecs(t)["8bit-nes"], "all.min.css"))
	if err != nil {
		t.Skip("8bit-nes stylesheet not vendored — run `make vendor` to enable this check")
	}
	m := regexp.MustCompile(`--bg:\s*(#[0-9a-fA-F]{3,8})`).FindSubmatch(css)
	if m == nil {
		t.Fatal("no --bg token in the pinned stylesheet: has the design system renamed it?")
	}
	token := strings.ToLower(string(m[1]))

	// .bar is painted with --bg in docsbase.html (the docs pages) and in the app's own
	// index.html, which Vite copies through verbatim — so both are checked, one from a
	// template and one from the built output.
	app, err := Index()
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{"docsbase.html": docsBaseTmpl, "dist/index.html": string(app)} {
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
			"Guide": nav.Guide, "BA": nav.BA, "Dev": nav.Dev, "Deploy": nav.Deploy,
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
		"guide": Docs, "ba": BA, "dev": Dev, "deploy": Deploy,
	} {
		out, err := build("/vendor", StaticNav)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, addr := range []string{StaticNav.Guide, StaticNav.BA, StaticNav.Dev, StaticNav.Deploy} {
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

	for _, want := range []string{"index.html", "ba.html", "dev.html", "deploy.html"} {
		if !strings.Contains(got, "https://example.test/docs/"+want) {
			t.Errorf("llms.txt never links %s", want)
		}
	}
	// Section names come out of the pages, so a page's own heading must appear. One per
	// page, so a page dropping out of the index is caught rather than averaged away.
	for _, want := range []string{"Start in 60 seconds", "Narrow to one folder", "The two seams", "Let the team in"} {
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

// ── the documentation set ─────────────────────────────────────────────────────

// TestRootDocsAreTheFourWeKnowAbout guards critical rules 17–19: four root documents,
// one job each, and the vision doc joined to reality in exactly one place.
//
// A fifth root .md is how a parallel truth starts — somebody writes NOTES.md or
// ROADMAP.md, it disagrees with one of these four within a week, and every agent that
// reads the tree now loads two answers to the same question. Adding one is allowed;
// doing it silently is not, and this is the conversation.
func TestRootDocsAreTheFourWeKnowAbout(t *testing.T) {
	t.Parallel()

	found, err := filepath.Glob("../*.md")
	if err != nil {
		t.Fatalf("globbing root docs: %v", err)
	}
	var got []string
	for _, p := range found {
		got = append(got, filepath.Base(p))
	}
	sort.Strings(got)

	// README.md reference · CLAUDE.md rules + architecture · AGENTS.md the pins agents
	// get wrong · README-MEGA-DOCS.md the vNext brief. Anything else belongs in
	// changelog/ (a decision) or a guide page (a feature).
	want := []string{"AGENTS.md", "CLAUDE.md", "README-MEGA-DOCS.md", "README.md"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("root docs are %v, want exactly %v — a fifth root document is a second\n"+
			"place the truth can live. Put a decision in changelog/ and a feature in its\n"+
			"guide section; if this file really is a fifth pillar, give it a job in\n"+
			"CLAUDE.md rule 18 and add it here in the same commit.", got, want)
	}

	// The brief describes a product this code is not yet, so README.md carries the join:
	// which lines already hold, which are next, and which are blocked and on what. Delete
	// the join and README-MEGA-DOCS.md silently reads as a description of the tree.
	readme, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatalf("README.md is the file-by-file reference; it must exist: %v", err)
	}
	for _, marker := range []string{"Now vs vNext", "README-MEGA-DOCS.md"} {
		if !strings.Contains(string(readme), marker) {
			t.Errorf("README.md no longer contains %q — the vNext brief is then an\n"+
				"unjoined wish list, and the next agent reads it as the spec.", marker)
		}
	}
}
