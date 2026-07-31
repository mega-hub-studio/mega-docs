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
	"encoding/json"
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
	Release []byte    // web/release.json verbatim; empty leaves GET /api/release unregistered
	Auth    Auth      // optional Basic credentials; zero value = open
	BAPass  BAPass    // gates the write actions; empty = no write surface
	Runtime Runtime   // what the status line reports; zero values simply hide fields
	// Settings is the Admin screen's payload, and AdminPass is what opens it. Both are
	// optional and both must be present: nil or an empty password leaves GET /api/settings
	// unregistered, so an instance with no admin secret has no admin surface to find.
	Settings  func() any
	AdminPass AdminPass
}

// Model is one chat model this instance will answer with, as the front end needs to know it:
// the name to send back, and the two numbers that let the status line report a percentage and
// a price for it. Zero in either means "unknown", which prints nothing.
//
// The server's own shape rather than config's: cmd/server maps one to the other, the same way
// it already does for every other field of Runtime, and this package stays free of the
// environment it is configured from.
type Model struct {
	Name     string  `json:"name"`
	Window   int     `json:"window"`
	PriceIn  float64 `json:"price_in"`
	PriceOut float64 `json:"price_out"`
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
	// Models is what a reader may pick between, first one the default — the same list the
	// four fields above describe the head of. Both are published: the scalars are what an
	// operator's `curl /api/health` has always answered, the list is what the front end
	// needs to offer a choice and to price whichever one it is on.
	Models []Model
	// The four numbers the engine works to, published so the settings panel can show what
	// this instance is actually tuned like without a password: the floor on how many sections
	// an answer is built from, how much of the window a thread may take, how much of it the
	// sections may take, and how many answers the cache holds. None of them is a secret and
	// all four change what a reader gets.
	TopK         int
	ThreadShare  float64
	ContextShare float64
	CacheKeep    int
	// Search says this instance can supplement a thin answer from the public web. The front
	// end cannot discover it — the bundle is static — and it has to say so before an answer
	// appears, because a labelled outside source is a thing a reader agrees to, not a
	// surprise. Like `writes` and `admin`: a capability the UI is told about, not one it
	// finds out by getting a 403.
	Search bool
	// Version is the commit the binary was built from — the one field here that says
	// nothing about an answer. It is reported so "which version is deployed?" has an
	// answer on the screen and from `curl /api/health`, rather than requiring shell access
	// to the host that serves it. Empty means the build carried no VCS stamp.
	Version string
	// Release is the tag that commit was cut from — `v0.13.0`, or empty in a tree with no
	// tags. Both are here because they answer different questions: the commit is which
	// bytes, the release is what changed. Only the label travels in /api/health; the notes
	// behind it are GET /api/release, so a payload that grows with every commit stays out
	// of the endpoint every client re-polls on reconnect.
	Release string
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
//	GET  /api/release what changed since the previous tag — open, and unregistered when no
//	                  tag has been cut, which is what leaves the version badge unclickable
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
	// `release` is the tag beside the commit, and it is a label rather than the notes: it is
	// what the badge prints, so it has to arrive in the request the app already makes. Empty
	// means no tag was ever cut, and the badge falls back to the commit instead of inventing
	// a number.
	// `models` is the picker's whole source of truth: the front end offers exactly these and
	// POST /api/chat refuses anything else, so the two can never disagree about what this
	// instance will answer with. Marshalled rather than formatted — a model name is operator
	// input, and a quote in it would otherwise write broken JSON.
	models, err := json.Marshal(d.Runtime.Models)
	if err != nil {
		models = []byte("[]")
	}
	health := fmt.Sprintf(
		`{"ok":true,"writes":%t,"admin":%t,"search":%t,"model":%q,"window":%d,`+
			`"price_in":%g,"price_out":%g,`+
			`"models":%s,"top_k":%d,"thread_share":%g,"context_share":%g,"cache_keep":%d,`+
			`"version":%q,"release":%q}`,
		d.BAPass.enabled(), d.AdminPass.enabled(), d.Runtime.Search,
		d.Runtime.Model, d.Runtime.Window,
		d.Runtime.PriceIn, d.Runtime.PriceOut, models,
		d.Runtime.TopK, d.Runtime.ThreadShare, d.Runtime.ContextShare, d.Runtime.CacheKeep,
		d.Runtime.Version, d.Runtime.Release)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The body is a constant; a failed write means the probe hung up, which is its
		// business, not ours.
		_, _ = w.Write([]byte(health))
	})

	// What changed, for the modal behind the badge. Registered only with a release to
	// describe: an untagged build has no notes, and a route answering `{"notes":[]}` would
	// have the app render an empty dialog rather than leave the badge unclickable — the same
	// rule the write routes follow, where a missing capability removes its surface.
	if len(d.Release) > 0 {
		mux.HandleFunc("GET /api/release", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(d.Release)
		})
	}

	mux.HandleFunc("POST /api/chat", chatHandler(d.Answers, d.Runtime.Models, d.Runtime.Search))
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
