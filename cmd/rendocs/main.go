// Command rendocs renders the bilingual guide (web/docs.html) to a static file.
//
// The guide is a Go template — versions and SRI digests come from
// web/vendor.sha384 — so GitHub Pages, which serves only static files, needs it
// rendered first. This is that step, and it keeps the published page generated
// from the same source as the one the binary serves. No cgo, no database: it
// imports the web package and nothing else.
//
//	go run ./cmd/rendocs -d _site
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"knowledge-engine/web"
)

func main() {
	dir := flag.String("d", "_site", "output directory")
	base := flag.String("base", "https://cdn.jsdelivr.net/npm",
		"asset base URL; the published page needs a public one")
	site := flag.String("site", "https://mega-hub-studio.github.io/mega-docs",
		"where these pages are published; llms.txt needs absolute URLs")
	flag.Parse()

	// StaticNav links the pages to each other by file name and drops the "Open app"
	// button — there is no app next to a static page. web.Pages is the one list of what
	// gets published, so this command cannot publish a different set than llms.txt and
	// spec.json describe.
	nav := web.StaticNav
	pages := map[string]func() ([]byte, error){}
	for _, page := range web.Pages() {
		build := page.Build // captured per iteration, or every entry renders the last page
		pages[page.File] = func() ([]byte, error) { return build(*base, nav) }
	}
	// llms.txt rides along with the pages: it is derived from them, so publishing one
	// without the other is how an agent ends up reading a stale index.
	pages["llms.txt"] = func() ([]byte, error) { return web.LLMs(*site) }
	// spec.json is the same idea one level deeper: the machine-readable join from each
	// documented feature to the routes, environment variables and tests behind it,
	// parsed out of the pages' own markup. An agent harness reads this to decide where a
	// change belongs; web/spec_test.go is what keeps it true.
	pages["spec.json"] = func() ([]byte, error) { return web.Spec(*site) }

	// 0750/0600: the output is a build artifact, read by whoever ran the build (the
	// Pages workflow uploads it as the same user). No other account needs it.
	if err := os.MkdirAll(*dir, 0o750); err != nil {
		die(err)
	}
	for name, build := range pages {
		page, err := build()
		if err != nil {
			die(err)
		}
		path := filepath.Join(*dir, name)
		if err := os.WriteFile(path, page, 0o600); err != nil {
			die(err)
		}
		fmt.Fprintf(os.Stderr, "rendocs: wrote %s (%d bytes)\n", path, len(page))
	}
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "rendocs: %v\n", err)
	os.Exit(1)
}
