package server

import "net/http"

// settings wires the Admin screen's one endpoint.
//
//	GET /api/settings  → [{group,name,value,source,secret}] — needs X-Admin-Pass
//
// Read-only, and that is the feature rather than a first step. Every knob is read once at
// startup, so a write path would need persistence and a reload for values nobody changes
// twice a year — while the thing an operator genuinely could not do was *see* one: the
// effective value lived in .env, in internal/config's defaults, and in whatever the shell
// already had, and which of the three won was a guess. `source` is that answer.
//
// Registered only when ADMIN_PASS is set, the same shape as the QA and import routes: a
// surface that cannot be unlocked does not exist. The route being absent is what lets the
// front end decide whether to show the screen at all, from /api/health alone.
//
// `inv` returns `any` deliberately. This package owns its own value types where it has a
// reason to — Auth, Runtime — and here it has none: the shape belongs to internal/config,
// which is where the knobs are defined, and a copy of that struct in this package would be
// a second place to add a field to. The server's whole job on this route is "gate it, encode
// it"; the payload is documented on the Deploy page's admin section.
func settings(mux *http.ServeMux, inv func() any, pass AdminPass) {
	if inv == nil || !pass.enabled() {
		return
	}
	mux.HandleFunc("GET /api/settings", pass.gate().wrap(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, inv())
	}))
}
