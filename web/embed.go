// Package web holds the embedded front-end: one HTML file plus, optionally, a
// vendored copy of its CDN assets.
package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"text/template"
)

// Both pages are templates, not static files — see Index and Docs.
//
//go:embed index.html
var indexTmpl string

//go:embed docs.html
var docsTmpl string

//go:embed dev.html
var devTmpl string

//go:embed ba.html
var baTmpl string

//go:embed deploy.html
var deployTmpl string

// docsbase.html holds the <head>, the top bar and the closing script that both
// guide pages share — one copy of the 80-line style block, not two.
//
//go:embed docsbase.html
var docsBaseTmpl string

// Diagrams are authored as mermaid (web/*.mmd) and rendered to SVG once by
// `make diagram`. The .mmd files ride along so a test can prove the committed SVG
// still matches its source — the failure mode of any generated file in a repo.
//
//go:embed *.svg *.mmd
var diagramFS embed.FS

// diagram returns a committed SVG for inlining. An unknown name is a template
// error, which surfaces at startup rather than as a hole in the page. The result
// is written verbatim — this is text/template, which does no escaping, and the
// input is a file from this repository rather than anything user-supplied.
func diagram(name string) (string, error) {
	b, err := diagramFS.ReadFile(name + ".svg")
	if err != nil {
		return "", fmt.Errorf("diagram %q: %w (run `make diagram`)", name, err)
	}
	return string(b), nil
}

// FS is the static tree served straight to the browser:
//
//	app/     the app shell — styles.css + the ES modules (no build step)
//	vendor/  third-party assets, laid out exactly like the CDN paths
//	         (vendor/<pkg>@<version>/<path>). Empty unless `make vendor` has run;
//	         the `all:` prefix keeps the embed valid when it holds only .gitkeep.
//
//go:embed app
//go:embed all:vendor
var FS embed.FS

// Nav is where the guide's pages live, relative to each other.
//
// One page per role, because each role arrives with a different question:
//
//	Guide   "what is this, and can I trust this answer?"   — everyone, then BA
//	Dev     "where do I change X, and how do I test it?"   — DEV
//	Deploy  "how do we run this for the team?"             — whoever hosts it
//
// To add a page: a field here, an address in StaticNav, a render func, and one entry
// in Pages.
type Nav struct {
	Guide, BA, Dev, Deploy string
}

// Page is one published page: the name it is published under, and what renders it.
type Page struct {
	File  string
	Build func(assetBase string, nav Nav) ([]byte, error)
}

// Pages is every page of the guide, in reading order — the one list cmd/rendocs,
// llms.txt and spec.json all walk. Ordered rather than a map, because two of those are
// generated files and an index that shuffles between builds is a useless diff.
//
// It exists because there were three copies of it, and a fourth page would have had to be
// added to each: the risk is not the typing, it is a spec that covers three pages while
// the site publishes four.
func Pages() []Page {
	return []Page{
		{"index.html", Docs}, // the site's landing page, so index rather than docs
		{strings.TrimPrefix(StaticNav.BA, "./"), BA},
		{strings.TrimPrefix(StaticNav.Dev, "./"), Dev},
		{strings.TrimPrefix(StaticNav.Deploy, "./"), Deploy},
	}
}

// StaticNav is how the published site addresses its own pages. It is the only Nav:
// the guide is documentation, published once at a public URL, and the app binary
// does not serve it. There is deliberately no link from the guide back to an app —
// an instance lives behind a tailnet or a tunnel, and a public page must not
// hardcode somebody's private address.
var StaticNav = Nav{Guide: "./index.html", BA: "./ba.html", Dev: "./dev.html", Deploy: "./deploy.html"}

// Index renders the app shell. docsURL is the published guide, which the app links
// out to rather than hosting: the docs have their own domain, and serving a second
// copy from the app is noise plus a copy to drift.
// The asset base is "https://cdn.jsdelivr.net/npm" (the default) or "/vendor" when
// the assets ship inside this binary. Rendering happens once at startup, so serving a
// request is just bytes.
func Index(assetBase, docsURL string) ([]byte, error) {
	return render(page{name: "index", tmpl: indexTmpl, base: assetBase, docsURL: docsURL})
}

// The three guide pages, by the name that identifies each one in a template, a URL and
// a nav entry — one spelling, so a new page cannot half-exist.
const (
	pageDocs   = "docs"
	pageBA     = "ba"
	pageDev    = "dev"
	pageDeploy = "deploy"
)

// Docs renders the Guide page. It, Dev and Deploy share Index's asset plumbing, so all
// four resolve from the same pinned CDN. Only cmd/rendocs calls the three guide pages:
// the guide is published as static files, never served by the app.
func Docs(assetBase string, nav Nav) ([]byte, error) {
	return render(page{
		name: pageDocs, tmpl: docsTmpl, base: assetBase, nav: nav,
		id: pageDocs, title: "Guide / Hướng dẫn",
	})
}

// BA renders the BA page: one section per thing a BA does — see Docs.
func BA(assetBase string, nav Nav) ([]byte, error) {
	return render(page{
		name: pageBA, tmpl: baTmpl, base: assetBase, nav: nav,
		id: pageBA, title: "BA / Cho BA",
	})
}

// Dev renders the Dev page — see Docs.
func Dev(assetBase string, nav Nav) ([]byte, error) {
	return render(page{
		name: pageDev, tmpl: devTmpl, base: assetBase, nav: nav,
		id: pageDev, title: "Dev / Cho dev",
	})
}

// Deploy renders the Deploy runbook — see Docs.
func Deploy(assetBase string, nav Nav) ([]byte, error) {
	return render(page{
		name: pageDeploy, tmpl: deployTmpl, base: assetBase, nav: nav,
		id: pageDeploy, title: "Deploy / Triển khai",
	})
}

type page struct {
	name, tmpl, base string
	nav              Nav
	docsURL          string // index.html only: the outbound link to the guide
	id, title        string // guide pages only; index.html uses neither
}

func render(pg page) ([]byte, error) {
	name, tmpl, assetBase := pg.name, pg.tmpl, pg.base
	base := strings.TrimSuffix(assetBase, "/")
	if base == "" {
		return nil, errors.New("web: empty asset base")
	}

	// A remote base gets a preconnect hint; a local /vendor path needs none.
	// Origin is the scheme+host to warm, e.g. https://cdn.jsdelivr.net.
	remote := strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://")
	origin := ""
	if remote {
		scheme, rest, _ := strings.Cut(base, "//")
		host, _, _ := strings.Cut(rest, "/")
		origin = scheme + "//" + host
	}

	p, err := parsePins(manifestSrc)
	if err != nil {
		return nil, err
	}

	// text/template, not html/template: the input is operator config, never user
	// input, and html/template rewrites URL and <script> contexts.
	//
	// Delimiters are <% %> because the page is a Vue template — {{ }} belongs to
	// Vue and Go must not touch it.
	//
	// url/sri read web/vendor.sha384, so the page never spells out a version or a
	// hash. Both return an error for anything unpinned, which Execute turns into a
	// startup failure — the right time to find out.
	t, err := template.New(name).Delims("<%", "%>").Funcs(template.FuncMap{
		"url": func(pkg, file string) (string, error) {
			path, err := p.path(pkg, file)
			if err != nil {
				return "", err
			}
			return base + "/" + path, nil
		},
		"sri": p.sri,
		// diagram inlines a pre-rendered SVG. Mermaid renders it at build time
		// (`make diagram`) and never ships to the browser: it is ~800KB gzipped, and
		// this diagram is fixed, so paying that at runtime buys nothing. Inline
		// rather than <img> so it needs no request, styles from the page's tokens,
		// and its nodes stay reachable for the walkthrough's spotlight.
		"diagram": diagram,
	}).Parse(tmpl)
	if err != nil {
		return nil, err
	}
	// The guide pages share a head/bar/foot partial; index.html doesn't use it.
	if pg.id != "" {
		if _, err := t.Parse(docsBaseTmpl); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		Base, Origin                       string
		DocsURL                            string
		GuideURL, BAURL, DevURL, DeployURL string
		Page, Title                        string
		Remote                             bool
	}{
		base, origin, pg.docsURL, pg.nav.Guide, pg.nav.BA, pg.nav.Dev, pg.nav.Deploy,
		pg.id, pg.title, remote,
	}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// HasVendor reports whether `make vendor` actually populated vendor/ — so the
// server can warn when ASSET_BASE points at /vendor but nothing is there.
func HasVendor() bool {
	entries, err := fs.ReadDir(FS, "vendor")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			return true
		}
	}
	return false
}
