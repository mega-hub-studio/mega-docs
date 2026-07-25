package web

import (
	_ "embed"
	"fmt"
	"strings"
)

// vendor.sha384 is the single source of truth for third-party assets: which
// version is pinned, what its bytes hash to, and therefore what `make vendor`
// downloads. index.html carries no literal version or digest — it asks for
// `url "vue" "dist/vue.global.prod.js"` and this file answers, so a bump is one
// line in one file instead of a version here and a hash there.
//
//go:embed vendor.sha384
var manifestSrc string

// pins maps a package name to its pinned "<pkg>@<version>" spec, and a full
// "<pkg>@<version>/<path>" to its sha384 digest.
type pins struct {
	spec   map[string]string // "vue" → "vue@3.5.40"
	digest map[string]string // "vue@3.5.40/dist/vue.global.prod.js" → "sha384-…"
}

func parsePins(src string) (pins, error) {
	p := pins{spec: map[string]string{}, digest: map[string]string{}}
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, sri, ok := strings.Cut(line, " ")
		if !ok {
			return p, fmt.Errorf("vendor.sha384:%d: want '<pkg>@<version>/<path> sha384-…'", i+1)
		}
		spec, _, ok := strings.Cut(path, "/")
		if !ok {
			return p, fmt.Errorf("vendor.sha384:%d: %q has no path after the version", i+1, path)
		}
		name, _, ok := strings.Cut(spec, "@")
		if !ok || name == "" {
			return p, fmt.Errorf("vendor.sha384:%d: %q is not <pkg>@<version>", i+1, spec)
		}
		if prev, dup := p.spec[name]; dup && prev != spec {
			return p, fmt.Errorf("vendor.sha384:%d: %s pinned twice (%s and %s)", i+1, name, prev, spec)
		}
		p.spec[name] = spec
		p.digest[path] = strings.TrimSpace(sri)
	}
	if len(p.digest) == 0 {
		return p, fmt.Errorf("vendor.sha384: no pins found")
	}
	return p, nil
}

// path returns "<pkg>@<version>/<file>", erroring when the manifest doesn't pin
// it — so a typo or a half-finished bump fails at startup, not in the browser.
func (p pins) path(pkg, file string) (string, error) {
	spec, ok := p.spec[pkg]
	if !ok {
		return "", fmt.Errorf("%s is not pinned in vendor.sha384", pkg)
	}
	full := spec + "/" + file
	if _, ok := p.digest[full]; !ok {
		return "", fmt.Errorf("%s is not listed in vendor.sha384 (add it, then run `make vendor`)", full)
	}
	return full, nil
}

func (p pins) sri(pkg, file string) (string, error) {
	full, err := p.path(pkg, file)
	if err != nil {
		return "", err
	}
	return p.digest[full], nil
}

// Vendored reports the manifest's pinned files as "<pkg>@<version>/<path>",
// which is exactly the layout `make vendor` writes under web/vendor/.
func (p pins) files() []string {
	out := make([]string, 0, len(p.digest))
	for k := range p.digest {
		out = append(out, k)
	}
	return out
}
