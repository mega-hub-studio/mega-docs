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

// Static tree served straight to the browser:
//
//	app/     the app shell — styles.css + the ES modules (no build step)
//	vendor/  third-party assets, laid out exactly like the CDN paths
//	         (vendor/<pkg>@<version>/<path>). Empty unless `make vendor` has run;
//	         the `all:` prefix keeps the embed valid when it holds only .gitkeep.
//
//go:embed app
//go:embed all:vendor
var FS embed.FS

// Index renders index.html for one asset base — "https://cdn.jsdelivr.net/npm"
// (the default) or "/vendor" when the assets ship inside this binary. Rendering
// happens once at startup, so serving a request is just bytes.
// Nav is where the guide's two pages live. They differ per host: the binary
// serves /docs and /deploy, while a static build uses relative file names.
type Nav struct {
	Guide, Deploy, App string
}

// ServedNav is how the running binary addresses its own pages.
var ServedNav = Nav{Guide: "/docs", Deploy: "/deploy", App: "/"}

// StaticNav is how a file-based host (GitHub Pages) addresses them. App is empty:
// there is no app to open next to a static page.
var StaticNav = Nav{Guide: "./index.html", Deploy: "./deploy.html"}

func Index(assetBase string) ([]byte, error) {
	return render(page{name: "index", tmpl: indexTmpl, base: assetBase, nav: ServedNav})
}

// Docs renders the guide page, Deploy the deployment page. Same asset plumbing as
// Index, so both load from the same pinned CDN — or from /vendor, which keeps the
// guide readable on an air-gapped box.
func Docs(assetBase string, nav Nav) ([]byte, error) {
	return render(page{name: "docs", tmpl: docsTmpl, base: assetBase, nav: nav,
		id: "docs", title: "Guide / Hướng dẫn"})
}

func Deploy(assetBase string, nav Nav) ([]byte, error) {
	return render(page{name: "deploy", tmpl: deployTmpl, base: assetBase, nav: nav,
		id: "deploy", title: "Deploy / Triển khai"})
}

type page struct {
	name, tmpl, base string
	nav              Nav
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
		Base, Origin                 string
		AppLink, GuideURL, DeployURL string
		Page, Title                  string
		Remote                       bool
	}{base, origin, pg.nav.App, pg.nav.Guide, pg.nav.Deploy, pg.id, pg.title, remote}); err != nil {
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
