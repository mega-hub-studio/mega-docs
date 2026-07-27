package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// This file makes the guide pages the project's spec as well as its documentation.
//
// The problem it solves is the ordinary one: a feature is described in prose on a page,
// implemented behind an HTTP route, configured by an environment variable, and pinned by
// a test — four places, no link between them. Prose then drifts from code silently, and
// the moment it does the docs stop being usable as a specification, by a person or by an
// agent.
//
// So a section that maps to code carries the mapping in its own markup:
//
//	<section id="scope" data-feature="scope"
//	         data-api="POST /api/chat"
//	         data-env="TOP_K"
//	         data-test="TestScopedSearchRanksWithinTheScope">
//
// Spec collects those into spec.json, published next to the pages. `spec_test.go` then
// checks the join in both directions — every name here exists in the code, and every
// /api/ route and every environment variable the code reads is named by some section. A
// new endpoint with no section fails `make check`, which is what "spec-driven" has to
// mean if it is to mean anything: the documentation is written first because the build
// will not go green without it.
//
// Deliberately not in the spec: prose. An agent gets that from the page itself (llms.txt
// links every one). Copying it here would create the second copy this whole arrangement
// exists to avoid.

// Feature is one entry of the spec: what a section documents, and the code facts an
// agent (or a person) needs to change it without reading the whole tree.
type Feature struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	TitleVI string `json:"title_vi,omitempty"`
	// Summary is the section's own opening sentence, in English — the behaviour
	// statement a spec needs, taken from the page rather than restated.
	Summary string `json:"summary,omitempty"`
	// URL points at the section, so a reader lands on the prose behind the entry.
	URL string `json:"url"`
	// API, Env and Tests are the join to code. Each is checked to exist, and the
	// full sets of routes and variables are checked to be covered.
	API   []string `json:"api,omitempty"`
	Env   []string `json:"env,omitempty"`
	Tests []string `json:"enforced_by,omitempty"`
}

// specDoc is what spec.json holds. The prose fields are addressed to whoever reads the
// file first — usually an agent harness deciding where a change belongs.
type specDoc struct {
	Product   string    `json:"product"`
	Summary   string    `json:"summary"`
	Generated string    `json:"generated_from"`
	Workflow  []string  `json:"how_to_add_or_change_a_feature"`
	Features  []Feature `json:"features"`
}

// Spec renders spec.json: the machine-readable half of the guide, generated from the
// same markup a person reads. siteBase is where the pages are published.
func Spec(siteBase string) ([]byte, error) {
	features, err := Features(siteBase)
	if err != nil {
		return nil, err
	}
	doc := specDoc{
		Product: "mega-docs",
		Summary: "Self-hosted RAG over a team's own documents. This file is the " +
			"specification: every feature entry names the page section that defines it, " +
			"the HTTP surface it is reached through, the environment variables that " +
			"configure it, and the tests that fail when it breaks.",
		Generated: "web/*.html — the published pages are the source of truth. Each entry " +
			"comes from a <section> that declares data-feature/data-api/data-env/" +
			"data-test, and web/spec_test.go fails the build when a name here is absent " +
			"from the code, or when a route or variable in the code is named by no section.",
		Workflow: []string{
			"1. Write the section first: what the feature does, on the page for its audience (Guide = everyone, ba.html = BA, dev.html = DEV, deploy.html = whoever hosts it).",
			"2. Declare the join on that <section>: data-feature (a stable id), data-api, data-env, data-test.",
			"3. Run `make check`. It fails until the named tests, routes and variables exist — so the test name in the markup is the test you write next.",
			"4. Implement until it is green. The docs are already correct, because they were the input.",
			"5. Both languages live in the same section (EN and VI markup side by side); the toggle is CSS-only, so a missing half is a visible gap rather than a silent one.",
		},
		Features: features,
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("spec.json: %w", err)
	}
	return append(b, '\n'), nil
}

// Features renders every page and parses the annotated sections out of it. Exported so
// the spec and the test that guards it read exactly the same thing — a checker with its
// own parser eventually checks something else.
func Features(siteBase string) ([]Feature, error) {
	siteBase = strings.TrimSuffix(siteBase, "/")
	var out []Feature
	seen := map[string]string{}
	for _, p := range Pages() {
		page, err := p.Build("https://cdn.jsdelivr.net/npm", StaticNav)
		if err != nil {
			return nil, fmt.Errorf("spec: %s: %w", p.File, err)
		}
		fs, err := sectionsOf(string(page), p.File, siteBase)
		if err != nil {
			return nil, err
		}
		for _, f := range fs {
			if where, dup := seen[f.ID]; dup {
				return nil, fmt.Errorf("spec: two sections claim data-feature=%q (%s and %s): "+
					"an id is how a change is addressed, so it has to be unique", f.ID, where, p.File)
			}
			seen[f.ID] = p.File
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("spec: no section declares data-feature — the spec would be empty")
	}
	return out, nil
}

// sectionsOf reads one rendered page. It works on opening tags rather than matching
// <section>…</section>, because <nes-tabs> nests sections and a non-greedy match would
// end a parent at its first child's closing tag.
func sectionsOf(html, file, siteBase string) ([]Feature, error) {
	opens := reSectionOpen.FindAllStringSubmatchIndex(html, -1)
	var out []Feature
	for i, m := range opens {
		attrs := html[m[2]:m[3]]
		id := attr(attrs, "data-feature")
		if id == "" {
			continue // an ordinary section: prose only, and llms.txt already lists it
		}
		// Up to the next section's opening tag: enough to hold this section's own
		// headings and lede, and it cannot run into a sibling's.
		end := len(html)
		if i+1 < len(opens) {
			end = opens[i+1][0]
		}
		body := html[m[1]:end]

		anchor := attr(attrs, "id")
		if anchor == "" {
			return nil, fmt.Errorf("spec: %s: section data-feature=%q has no id — "+
				"the spec links to the prose, so the section needs an anchor", file, id)
		}
		title := firstMatch(reH2, body)
		if title == "" {
			return nil, fmt.Errorf("spec: %s: section %q has no English h2", file, id)
		}
		f := Feature{
			ID:      id,
			Title:   title,
			TitleVI: firstMatch(reH2VI, body),
			Summary: summaryOf(body),
			URL:     fmt.Sprintf("%s/%s#%s", siteBase, file, anchor),
			API:     list(attr(attrs, "data-api")),
			Env:     list(attr(attrs, "data-env")),
			Tests:   list(attr(attrs, "data-test")),
		}
		if len(f.API) == 0 && len(f.Env) == 0 && len(f.Tests) == 0 {
			return nil, fmt.Errorf("spec: %s: section %q declares data-feature but no "+
				"data-api, data-env or data-test — a feature entry with no join to code "+
				"is prose, and prose needs no annotation", file, id)
		}
		out = append(out, f)
	}
	return out, nil
}

// summaryOf takes the section's opening sentence: its lede if it has one, otherwise its
// first paragraph. Both are the author's own words about what the feature does.
func summaryOf(body string) string {
	if s := firstMatch(reLede, body); s != "" {
		return s
	}
	return firstMatch(reFirstP, body)
}

// list splits a comma-separated attribute and sorts it, so spec.json is stable: an
// entry that reorders between builds is noise in every diff.
func list(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(clean(part)); p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// attr pulls one double-quoted attribute out of an opening tag. The markup is generated
// by this package, so this is reading a known shape rather than parsing HTML.
func attr(tag, name string) string {
	re, ok := attrRE[name]
	if !ok {
		re = regexp.MustCompile(name + `="([^"]*)"`)
		attrRE[name] = re
	}
	m := re.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return m[1]
}

var attrRE = map[string]*regexp.Regexp{}

var (
	reSectionOpen = regexp.MustCompile(`<section([^>]*)>`)
	reH2VI        = regexp.MustCompile(`<h2 lang="vi">(.*?)</h2>`)
	reFirstP      = regexp.MustCompile(`(?s)<p lang="en">\s*(.*?)</p>`)
)
