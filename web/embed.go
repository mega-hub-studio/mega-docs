// Package web holds the embedded front-end: one HTML file plus, optionally, a
// vendored copy of its CDN assets.
package web

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"strings"
	"text/template"
)

// index.html is a template, not a static file — see Index.
//
//go:embed index.html
var indexTmpl string

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
func Index(assetBase string) ([]byte, error) {
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

	// text/template, not html/template: the input is operator config, never user
	// input, and html/template rewrites URL and <script> contexts.
	//
	// Delimiters are <% %> because the page is a Vue template — {{ }} belongs to
	// Vue and Go must not touch it.
	t, err := template.New("index").Delims("<%", "%>").Parse(indexTmpl)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		Base, Origin string
		Remote       bool
	}{base, origin, remote}); err != nil {
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
