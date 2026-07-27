package web

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// These three tests are what makes spec.json a specification rather than a summary.
//
// A generated document that only ever reads the pages can say anything: name a test that
// was renamed two commits ago, or omit the endpoint somebody added this morning. So the
// join is checked in both directions, against the source:
//
//	spec → code   every test name, route and variable a section declares must exist
//	code → spec   every /api/ route and every variable internal/config reads must be
//	              named by some section
//
// The second direction is the one with teeth. It means a new endpoint cannot go green
// until it is documented, which is the only version of "documentation is the source of
// truth" that survives a deadline.
//
// They read the sibling packages' source rather than importing them: web must not depend
// on internal/server or internal/config (it is the layer they serve), and the facts
// wanted here — which routes are registered, which variables are read — are lexical.

const testSite = "https://mega-hub-studio.github.io/mega-docs"

func TestEverySpecNameExistsInTheCode(t *testing.T) {
	features, err := Features(testSite)
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	tests, routes, knobs := codeFacts(t)

	for _, f := range features {
		for _, name := range f.Tests {
			if !tests[name] {
				t.Errorf("section %q names data-test=%q, which is not a test in this tree.\n"+
					"Either write it, or fix the name: the point of the annotation is that "+
					"breaking the feature turns something red.", f.ID, name)
			}
		}
		for _, r := range f.API {
			if !routes[r] {
				t.Errorf("section %q names data-api=%q, which no mux line registers.\n"+
					"Registered: %v", f.ID, r, sorted(routes))
			}
		}
		for _, k := range f.Env {
			if !knobs[k] {
				t.Errorf("section %q names data-env=%q, which internal/config never reads.\n"+
					"A documented setting that nothing reads is worse than an undocumented "+
					"one: it gets set, and nothing happens.", f.ID, k)
			}
		}
	}
}

func TestEveryRouteAndKnobIsSpecified(t *testing.T) {
	features, err := Features(testSite)
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	_, routes, knobs := codeFacts(t)

	specced := map[string]string{}
	for _, f := range features {
		for _, r := range f.API {
			specced[r] = f.ID
		}
		for _, k := range f.Env {
			specced[k] = f.ID
		}
	}

	for r := range routes {
		// Only the API is specified this way. The three static routes (the shell, app/,
		// vendor/) are the delivery of the page itself, not a feature anyone calls, and
		// they have their own tests.
		if !strings.Contains(r, "/api/") {
			continue
		}
		if specced[r] == "" {
			t.Errorf("%s is registered but no section declares it.\n"+
				"Add data-api to the section that documents it (dev.html#http lists the "+
				"whole surface), or the endpoint exists only in the code.", r)
		}
	}
	for k := range knobs {
		if specced[k] == "" {
			t.Errorf("internal/config reads %s but no section declares it.\n"+
				"The settings table on deploy.html is where a knob lands — add a row and "+
				"the name to that section's data-env.", k)
		}
	}
}

func TestSpecJSONIsGeneratedFromThePages(t *testing.T) {
	b, err := Spec(testSite)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	var doc struct {
		Product  string `json:"product"`
		Workflow []string
		Features []Feature `json:"features"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("spec.json is not valid JSON: %v", err)
	}
	if doc.Product != "mega-docs" {
		t.Errorf("product = %q", doc.Product)
	}
	if len(doc.Features) < 15 {
		t.Errorf("spec.json holds %d features — the pages annotate more than that, so "+
			"something stopped being parsed", len(doc.Features))
	}

	seen := map[string]bool{}
	for _, f := range doc.Features {
		switch {
		case f.ID == "":
			t.Error("a feature has no id")
		case seen[f.ID]:
			t.Errorf("feature %q appears twice", f.ID)
		case f.Title == "":
			t.Errorf("feature %q has no English title", f.ID)
		case f.TitleVI == "":
			t.Errorf("feature %q has no Vietnamese title — both languages are the "+
				"deliverable, and a spec entry missing one hides that", f.ID)
		case f.Summary == "":
			t.Errorf("feature %q has no summary: the section has neither a lede nor a "+
				"first paragraph, so the spec says nothing about what it does", f.ID)
		case !strings.HasPrefix(f.URL, testSite+"/"), !strings.Contains(f.URL, "#"):
			t.Errorf("feature %q has URL %q — it must point at its own section", f.ID, f.URL)
		}
		seen[f.ID] = true
	}

	// Every page must contribute: a page with no annotated section is a page whose
	// features live only in prose, which is how the drift starts.
	for _, page := range Pages() {
		found := false
		for _, f := range doc.Features {
			if strings.Contains(f.URL, "/"+page.File+"#") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s declares no feature", page.File)
		}
	}
}

// TestAnUnannotatedFeatureIsRefused covers the parser's own guards — the errors that stop
// a half-written annotation from silently producing a spec with a hole in it.
func TestAnUnannotatedFeatureIsRefused(t *testing.T) {
	cases := []struct {
		name, html, want string
	}{
		{
			"no anchor",
			`<section data-feature="x" data-test="TestFoo"><h2 lang="en">X</h2></section>`,
			"has no id",
		},
		{
			"no join to code",
			`<section id="x" data-feature="x"><h2 lang="en">X</h2></section>`,
			"no join to code",
		},
		{
			"no English heading",
			`<section id="x" data-feature="x" data-test="TestFoo"><h2 lang="vi">X</h2></section>`,
			"no English h2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := sectionsOf(c.html, "test.html", testSite)
			if err == nil {
				t.Fatalf("accepted %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error was %q, wanted it to mention %q", err, c.want)
			}
		})
	}
}

// codeFacts reads the three things the spec joins to, out of the source: test names
// anywhere in the tree, routes registered in internal/server, and variables read by
// internal/config.
func codeFacts(t *testing.T) (tests, routes, knobs map[string]bool) {
	t.Helper()
	tests, routes, knobs = map[string]bool{}, map[string]bool{}, map[string]bool{}

	// The walk only collects paths; the files are read afterwards. Reading inside a
	// WalkDir callback is a symlink race (gosec G122) — and here it would also mean the
	// three extractors run interleaved with directory traversal for no reason.
	var files []string
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// These hold third-party or generated Go that would add thousands of
			// irrelevant test names.
			switch d.Name() {
			case ".git", ".cache", "vendor", "node_modules", "bin":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		text := string(src)
		for _, m := range reTestFunc.FindAllStringSubmatch(text, -1) {
			tests[m[1]] = true
		}
		if strings.Contains(path, filepath.Join("internal", "server")) {
			for _, m := range reRoute.FindAllStringSubmatch(text, -1) {
				routes[m[1]] = true
			}
		}
		if strings.Contains(path, filepath.Join("internal", "config")) &&
			!strings.HasSuffix(path, "_test.go") {
			for _, m := range reKnob.FindAllStringSubmatch(text, -1) {
				knobs[m[1]] = true
			}
		}
	}
	// A silent empty set would make every check above pass for the wrong reason.
	if len(tests) < 100 || len(routes) < 8 || len(knobs) < 15 {
		t.Fatalf("read %d tests, %d routes, %d knobs — the extractor stopped matching, "+
			"so nothing below is being checked", len(tests), len(routes), len(knobs))
	}
	return tests, routes, knobs
}

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var (
	reTestFunc = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]*)\(`)
	// Both forms the server uses, and only the pattern string: "METHOD /path".
	reRoute = regexp.MustCompile(`mux\.Handle(?:Func)?\("([A-Z]+ /[^"]*)"`)
	// env / envInt / envFloat — the only three ways a variable is read.
	reKnob = regexp.MustCompile(`\benv(?:Int|Float)?\("([A-Z][A-Z0-9_]*)"`)
)

// Guard the guard: the extractor's own patterns are the kind of thing that keeps
// matching one line after the code moved on.
func TestTheExtractorFindsWhatItClaims(t *testing.T) {
	_, routes, knobs := codeFacts(t)
	for _, want := range []string{
		"POST /api/chat", "GET /api/health", "POST /api/tickets/{id}/{action}",
		"POST /api/documents", "GET /{$}",
	} {
		if !routes[want] {
			t.Errorf("route extractor missed %q; found %v", want, sorted(routes))
		}
	}
	for _, want := range []string{"BA_PASS", "TOP_K", "EMBED_DIM", "PRICE_IN", "CORPUS_DIR"} {
		if !knobs[want] {
			t.Errorf("knob extractor missed %q; found %v", want, sorted(knobs))
		}
	}
}

// TestNoDiagramIdCollidesWithASectionId exists because one did, and the failure is
// invisible in a diff: mermaid scopes the SVG's own <style> to the svg id, so a diagram
// called "spec" inlined into <section id="spec"> restyles the whole section — mermaid's
// font, size and colours applied to a page of prose. The two id spaces live in separate
// files (web/*.mmd and the page markup), so nothing else would notice.
//
// Only these two sets are compared. Mermaid reuses ids inside its own output (an edge id
// appears on the path and on its label), which is its business and not a collision with
// the page.
func TestNoDiagramIdCollidesWithASectionId(t *testing.T) {
	for _, page := range Pages() {
		rendered, err := page.Build("https://cdn.jsdelivr.net/npm", StaticNav)
		if err != nil {
			t.Fatalf("%s: %v", page.File, err)
		}
		html := string(rendered)
		sections := map[string]bool{}
		for _, m := range reSectionOpen.FindAllStringSubmatch(html, -1) {
			if id := attr(m[1], "id"); id != "" {
				sections[id] = true
			}
		}
		for _, m := range reSVGRoot.FindAllStringSubmatch(html, -1) {
			if sections[m[1]] {
				t.Errorf("%s: the inlined diagram and a section both use id %q. Rename "+
					"web/%s.mmd: mermaid's <style> is scoped to the svg id, so its rules "+
					"apply to the section too.", page.File, m[1], m[1])
			}
		}
	}
}

var reSVGRoot = regexp.MustCompile(`<svg id="([A-Za-z][A-Za-z0-9_-]*)"`)
