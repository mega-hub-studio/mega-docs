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
	flag.Parse()

	// StaticNav links the pages to each other by file name and drops the "Open app"
	// button — there is no app next to a static page.
	pages := map[string]func() ([]byte, error){
		"index.html":  func() ([]byte, error) { return web.Docs(*base, web.StaticNav) },
		"deploy.html": func() ([]byte, error) { return web.Deploy(*base, web.StaticNav) },
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		die(err)
	}
	for name, build := range pages {
		page, err := build()
		if err != nil {
			die(err)
		}
		path := filepath.Join(*dir, name)
		if err := os.WriteFile(path, page, 0o644); err != nil {
			die(err)
		}
		fmt.Fprintf(os.Stderr, "rendocs: wrote %s (%d bytes)\n", path, len(page))
	}
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "rendocs: %v\n", err)
	os.Exit(1)
}
