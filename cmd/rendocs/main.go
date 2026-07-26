// Command rendocs renders the bilingual guide (web/docs.html) to a static file.
//
// The guide is a Go template — versions and SRI digests come from
// web/vendor.sha384 — so GitHub Pages, which serves only static files, needs it
// rendered first. This is that step, and it keeps the published page generated
// from the same source as the one the binary serves. No cgo, no database: it
// imports the web package and nothing else.
//
//	go run ./cmd/rendocs -o _site/index.html
package main

import (
	"flag"
	"fmt"
	"os"

	"knowledge-engine/web"
)

func main() {
	out := flag.String("o", "-", `output file, or "-" for stdout`)
	base := flag.String("base", "https://cdn.jsdelivr.net/npm",
		"asset base URL; the published page needs a public one")
	app := flag.String("app", "",
		`URL the "Open app" button points at; empty omits the button (there is no app on a static host)`)
	flag.Parse()

	page, err := web.Docs(*base, *app)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rendocs: %v\n", err)
		os.Exit(1)
	}
	if *out == "-" {
		os.Stdout.Write(page)
		return
	}
	if err := os.WriteFile(*out, page, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "rendocs: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "rendocs: wrote %s (%d bytes)\n", *out, len(page))
}
