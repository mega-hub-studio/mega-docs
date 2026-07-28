// Package server is the HTTP layer: routing, cache policy, and the SSE chat
// endpoint. It knows nothing about SQLite, embeddings or templates — it is handed
// narrow interfaces and the already-rendered page, so it can be tested with neither.
// The read side (Answerer) is required; the write sides (Knowledge, Importer) are
// nil-able, and their routes disappear rather than half-work.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"knowledge-engine/internal/db"
	"knowledge-engine/internal/rag"
)

// Answerer is the read side of the RAG engine — all the HTTP layer needs to answer
// a question. *rag.Engine satisfies it; tests pass a fake.
type Answerer interface {
	Answer(ctx context.Context, a rag.Ask) (rag.Reply, error)
	// Corpus answers "what does this engine know?" — without it, an empty index
	// is indistinguishable from a broken retriever.
	Corpus(limit int) (db.Corpus, error)
}

// Deps is everything the handler set needs.
type Deps struct {
	Answers Answerer  // the RAG engine, read side
	Know    Knowledge // the QA loop; nil leaves those routes unregistered
	Docs    Importer  // document import; nil leaves that route unregistered
	Index   []byte    // the built app's index.html, straight from web/dist
	Assets  fs.FS     // the rest of that build: assets/… every name content-hashed
	Auth    Auth      // optional Basic credentials; zero value = open
	BAPass  BAPass    // gates the write actions; empty = no write surface
	Runtime Runtime   // what the status line reports; zero values simply hide fields
	// Settings is the Admin screen's payload, and AdminPass is what opens it. Both are
	// optional and both must be present: nil or an empty password leaves GET /api/settings
	// unregistered, so an instance with no admin secret has no admin surface to find.
	Settings  func() any
	AdminPass AdminPass
}

// Runtime is what an answer costs and what produced it — everything the status line
// shows that the engine cannot infer per request. Zero means "unknown", and the UI
// prints nothing rather than a zero, because a cost of $0.00 and an unmeasured cost
// are different facts.
type Runtime struct {
	Model    string
	Window   int
	PriceIn  float64
	PriceOut float64
	// Version is the commit the binary was built from — the one field here that says
	// nothing about an answer. It is reported so "which version is deployed?" has an
	// answer on the screen and from `curl /api/health`, rather than requiring shell access
	// to the host that serves it. Empty means the build carried no VCS stamp.
	Version string
}

// New wires the routes and returns the whole app as one handler.
//
// This is the app and nothing else. The guide is documentation with its own public
// domain, so it is not served here — one surface, one job.
//
//	GET  /            index.html          revalidated (it names hashed assets)
//	GET  /api/health  {"ok","writes","admin","model","window","price_in","price_out"} — open,
//	                  so probes need no secret. No "site": the app does not link to the guide
//	POST /api/chat    SSE: cached · token · citations · done · error
//	GET  /api/corpus  {"docs":n,"chunks":n,"approved":n,"documents":[…]}
//	GET  /api/tickets · POST /api/tickets · POST /api/tickets/{id}/{action}
//	POST /api/documents  import .md/.txt into the corpus — same gate as a confirm
//	DELETE /api/documents/{path…}  remove a document and its chunks — same gate
//	GET  /api/history answers still free to replay
//	GET  /api/settings  every knob with the provenance of its value — needs X-Admin-Pass,
//	                  and unregistered entirely when ADMIN_PASS is unset
//	GET  /assets/…    the built bundle      immutable (every name has a content hash)
func New(d Deps) http.Handler {
	mux := http.NewServeMux()

	// What the status line needs before the first question, plus the one field BA
	// mode needs: whether it can do anything here. That this instance *has* a
	// password configured is not a secret — that it is read-only is exactly what a
	// BA needs to be told before typing an answer.
	//
	// The model name and prices are deliberate disclosure, not a leak: an operator
	// asked for them on screen. The engine itself still refuses to discuss them —
	// that rule is about what a *document* answer may contain.
	// `admin` is here for the same reason `writes` is: the front end is a static bundle and
	// cannot discover which routes exist. Without it the Admin tab would have to render and
	// then fail on 403, which teaches a reader that the app is broken rather than that this
	// instance has no admin secret.
	// `version` is the deployed commit. It is disclosure of a public repository's revision,
	// not a secret, and it is what makes a deploy verifiable from the UI: the alternative was
	// reading journalctl on the host, which the person asking usually cannot reach.
	health := fmt.Sprintf(
		`{"ok":true,"writes":%t,"admin":%t,"model":%q,"window":%d,"price_in":%g,"price_out":%g,"version":%q}`,
		d.BAPass.enabled(), d.AdminPass.enabled(), d.Runtime.Model, d.Runtime.Window,
		d.Runtime.PriceIn, d.Runtime.PriceOut, d.Runtime.Version)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The body is a constant; a failed write means the probe hung up, which is its
		// business, not ours.
		_, _ = w.Write([]byte(health))
	})

	mux.HandleFunc("POST /api/chat", chatHandler(d.Answers))
	mux.HandleFunc("GET /api/corpus", corpusHandler(d.Answers))
	if d.Know != nil {
		tickets(mux, d.Know, d.BAPass)
	}
	if d.Docs != nil {
		documents(mux, d.Docs, d.BAPass)
	}
	settings(mux, d.Settings, d.AdminPass)

	// "/{$}" matches only the root, so the file servers below never see it.
	mux.Handle("GET /{$}", revalidate(etag(d.Index), serveBytes(d.Index, "text/html; charset=utf-8")))
	// Every file under assets/ carries a content hash in its name, which is the only
	// thing that makes a year-long immutable cache safe: a changed file is a changed
	// URL, and index.html above is revalidated so the new names are always found.
	mux.Handle("GET /assets/", immutable(http.FileServerFS(d.Assets)))

	return guard(d.Auth, mux)
}

func serveBytes(b []byte, contentType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b))
	})
}

// revalidate lets the browser cache a response but check before reusing it. With
// a strong ETag that check is a 304 — no bytes, and never a stale asset after a
// deploy.
func revalidate(tag string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", tag)
		w.Header().Set("Cache-Control", "no-cache")
		if match(r.Header.Get("If-None-Match"), tag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// immutable is safe only where the URL carries a version (vendor/vue@3.5.40/…),
// which is exactly the contract that makes a pinned CDN URL cacheable for a year.
func immutable(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}

func etag(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf(`"%x"`, sum[:16])
}

// match handles the one If-None-Match form browsers actually send back, plus "*".
func match(header, tag string) bool {
	return header != "" && (header == "*" || header == tag || header == "W/"+tag)
}
