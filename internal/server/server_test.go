package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"knowledge-engine/internal/db"
	"knowledge-engine/internal/rag"
)

// fakeAnswers stands in for the RAG engine — the whole point of the Answerer
// interface, so the HTTP layer is testable without SQLite or an API key.
type fakeAnswers struct {
	tokens []string
	cites  []rag.Citation
	cached bool
	err    error
	corpus db.Corpus
	cErr   error
	recall rag.Recall // how much of the thread the model read, for the `done` frame
	// retrieval is the same pair for the corpus — sections read of sections weighed. Both are
	// on the `done` frame and both are omitted when zero, so a first question does not print
	// a memory figure and an unconfigured instance does not print a grounding one.
	retrieval rag.Recall
	asked     []rag.Ask // what the handler passed down, so `fresh` can be verified
}

func (f *fakeAnswers) Answer(_ context.Context, a rag.Ask) (rag.Reply, error) {
	f.asked = append(f.asked, a)
	for _, t := range f.tokens {
		a.OnToken(t)
	}
	return rag.Reply{
		Citations: f.cites, Cached: f.cached, Recall: f.recall, Retrieval: f.retrieval,
	}, f.err
}

func (f *fakeAnswers) Corpus(int) (db.Corpus, error) { return f.corpus, f.cErr }

func newTestServer(a Answerer) http.Handler {
	return New(Deps{
		Answers: a,
		Index:   []byte("<html>index</html>"),
		// The shape Vite emits: every name carries a content hash, which is what makes
		// the immutable cache below safe.
		Assets: fstest.MapFS{
			"assets/index-A1b2C3d4.js":  {Data: []byte("export const x = 1\n")},
			"assets/index-E5f6G7h8.css": {Data: []byte(":root{}\n")},
		},
	})
}

func do(t *testing.T, h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestHealth(t *testing.T) {
	w := do(t, newTestServer(&fakeAnswers{}), "GET", "/api/health", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("health = %d %q", w.Code, w.Body.String())
	}
	// A capability the bundle cannot discover, so it has to be told: an instance with no
	// search key must say so, and the default here is off. Reported false rather than
	// omitted — the front end assigns the whole object, and an absent key lands as undefined.
	if !strings.Contains(w.Body.String(), `"search":false`) {
		t.Errorf("health = %q, want it to report search:false on an instance with no key",
			w.Body.String())
	}
}

// TestHealthReportsTheWebSupplement is the other half of the rule above, and it is here
// because a labelled outside source is something a reader has to be able to expect rather
// than discover: the guide's own promise is that an unset SEARCH_API_KEY means no external
// call is ever made, and the only way the screen can say so before an answer arrives is this
// field.
func TestHealthReportsTheWebSupplement(t *testing.T) {
	h := New(Deps{
		Answers: &fakeAnswers{},
		Index:   []byte("<html>index</html>"),
		Assets:  fstest.MapFS{},
		Runtime: Runtime{Search: true, ContextShare: 0.5},
	})
	body := do(t, h, "GET", "/api/health", "", nil).Body.String()
	for _, want := range []string{`"search":true`, `"context_share":0.5`} {
		if !strings.Contains(body, want) {
			t.Errorf("health = %q, want %s", body, want)
		}
	}
}

// The deployed revision has to reach the browser, because that is the whole point of
// stamping it: the UI shows it, so "which version is running?" is answered by looking at
// the screen instead of by reaching the host. A field the shell never sees would be a
// version nobody can read.
func TestHealthReportsTheDeployedRevision(t *testing.T) {
	h := New(Deps{
		Answers: &fakeAnswers{},
		Index:   []byte("<html>index</html>"),
		Assets:  fstest.MapFS{},
		Runtime: Runtime{Version: "9c4a34d"},
	})
	if body := do(t, h, "GET", "/api/health", "", nil).Body.String(); !strings.Contains(body, `"version":"9c4a34d"`) {
		t.Errorf("health = %q, want it to report version 9c4a34d", body)
	}
}

func TestIndexIsRevalidatedAndCachesWithETag(t *testing.T) {
	h := newTestServer(&fakeAnswers{})

	w := do(t, h, "GET", "/", "", nil)
	if w.Code != 200 || w.Body.String() != "<html>index</html>" {
		t.Fatalf("index = %d %q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache (it pins asset versions)", got)
	}
	tag := w.Header().Get("ETag")
	if tag == "" {
		t.Fatal("no ETag on index")
	}

	// A reload must cost 304 + no body, not a re-download.
	w2 := do(t, h, "GET", "/", "", map[string]string{"If-None-Match": tag})
	if w2.Code != http.StatusNotModified || w2.Body.Len() != 0 {
		t.Errorf("conditional GET = %d, %d bytes; want 304 and empty", w2.Code, w2.Body.Len())
	}
}

func TestTheBuiltBundleIsImmutable(t *testing.T) {
	h := newTestServer(&fakeAnswers{})

	js := do(t, h, "GET", "/assets/index-A1b2C3d4.js", "", nil)
	if js.Code != 200 {
		t.Fatalf("bundle = %d", js.Code)
	}
	// Every asset name carries a content hash, so a changed file is a changed URL and a
	// year of caching is safe. index.html is revalidated (the test above), which is what
	// makes the new names reachable.
	if got := js.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("bundle Cache-Control = %q, want immutable", got)
	}
	// Module scripts are rejected unless the MIME type is JavaScript.
	if ct := js.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("bundle Content-Type = %q, want a JavaScript type", ct)
	}
	if css := do(t, h, "GET", "/assets/index-E5f6G7h8.css", "", nil); css.Code != 200 {
		t.Errorf("stylesheet = %d", css.Code)
	}
	// The old app tree is gone: a stale bookmark or a cached page must 404 rather than
	// silently serve something.
	if old := do(t, h, "GET", "/app/app.js", "", nil); old.Code != 404 {
		t.Errorf("/app/app.js = %d, want 404 — that tree no longer exists", old.Code)
	}
}

// The `done` frame is asserted whole, and with every optional field populated, because it is
// the one frame that carries facts the client cannot re-derive: which model answered, and how
// much of the thread it read. A client reading only some of them renders a blank model badge
// and a memory meter about nothing, with no error anywhere — so the wire shape is pinned here.
func TestChatStreamsTokensThenCitationsThenDone(t *testing.T) {
	h := New(Deps{
		Answers: &fakeAnswers{
			tokens: []string{"Hybrid ", "search [1]"},
			// Both kinds, because the wire shape has to keep them apart: a document carries
			// doc/heading, a public result carries kind/title/url and its own numbering.
			cites: []rag.Citation{
				{N: 1, DocPath: "docs/a.md", Heading: "How"},
				{N: 1, Kind: "web", Title: "OAuth 2.0", URL: "https://example.test/oauth"},
			},
			recall:    rag.Recall{Kept: 3, Offered: 8},
			retrieval: rag.Recall{Kept: 18, Offered: 40},
		},
		Index: []byte("<html>index</html>"),
		// A named model only survives readQuestion when the instance offers it: pick() answers
		// "" for an empty list, whatever was asked for.
		Runtime: Runtime{Models: []Model{{Name: "gpt-4"}}},
	})

	w := do(t, h, "POST", "/api/chat", `{"question":"how?","model":"gpt-4"}`, nil)
	if w.Code != 200 {
		t.Fatalf("chat = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}

	body := w.Body.String()
	for _, want := range []string{
		"event: token\ndata: {\"t\":\"Hybrid \"}\n\n",
		"event: token\ndata: {\"t\":\"search [1]\"}\n\n",
		`event: citations`,
		`"doc":"docs/a.md"`,
		`{"n":1,"kind":"web","title":"OAuth 2.0","url":"https://example.test/oauth"}`,
		"event: done\ndata: {\"done\":true,\"cached\":false," +
			"\"model\":\"gpt-4\",\"kept\":3,\"offered\":8," +
			"\"sections\":18,\"candidates\":40}\n\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("stream missing %q\n--- got ---\n%s", want, body)
		}
	}
	// Order matters: the client links [n] markers only once citations land.
	if strings.Index(body, "event: citations") < strings.LastIndex(body, "event: token") {
		t.Error("citations were sent before the last token")
	}
}

func TestChatSendsEmptyCitationListNotNull(t *testing.T) {
	// Retrieval finding nothing returns nil citations; the client reads .length
	// off this immediately, so `null` would break the render.
	h := newTestServer(&fakeAnswers{tokens: []string{"nothing found"}, cites: nil})

	body := do(t, h, "POST", "/api/chat", `{"question":"x"}`, nil).Body.String()
	if !strings.Contains(body, "event: citations\ndata: []\n\n") {
		t.Errorf("want an empty citation array, got:\n%s", body)
	}
}

func TestChatReportsEngineFailureInStream(t *testing.T) {
	h := newTestServer(&fakeAnswers{tokens: []string{"partial"}, err: errors.New("provider down")})

	body := do(t, h, "POST", "/api/chat", `{"question":"x"}`, nil).Body.String()
	if !strings.Contains(body, `event: error`) || !strings.Contains(body, "provider down") {
		t.Errorf("want an error event, got:\n%s", body)
	}
	if strings.Contains(body, "event: done") {
		t.Error("a failed answer must not also report done")
	}
}

func TestChatRejectsBadRequests(t *testing.T) {
	h := newTestServer(&fakeAnswers{})
	for name, body := range map[string]string{
		"not json":        `{`,
		"missing field":   `{}`,
		"empty question":  `{"question":""}`,
		"blank question":  `{"question":"   "}`,
		"wrong json type": `{"question":42}`,
		// A thread longer than the client ever offers. The byte cap cannot catch this — a
		// two-character turn is legal, so thousands fit under 256 KiB — and every one of them
		// would buy two completions on a route that is open by design and never cached.
		"more turns than the client offers": `{"question":"x","history":[` +
			strings.Repeat(`{"q":"a","a":"b"},`, 13) + `{"q":"a","a":"b"}]}`,
	} {
		if code := do(t, h, "POST", "/api/chat", body, nil).Code; code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", name, code)
		}
	}
}

func TestCorpusReportsWhatIsIndexed(t *testing.T) {
	h := newTestServer(&fakeAnswers{corpus: db.Corpus{
		Docs: 2, Chunks: 41, Approved: 7,
		Documents: []db.Document{{Path: "docs/a.md", Title: "A", Chunks: 30, UpdatedAt: "2026-07-01 10:00:00"}},
	}})

	w := do(t, h, "GET", "/api/corpus", "", nil)
	if w.Code != 200 {
		t.Fatalf("corpus = %d", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store (an ingest changes it)", got)
	}
	var got db.Corpus
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v — %s", err, w.Body.String())
	}
	if got.Docs != 2 || got.Chunks != 41 || got.Approved != 7 || len(got.Documents) != 1 {
		t.Errorf("round-trip lost data: %+v", got)
	}
}

func TestCorpusOnEmptyIndexIsStillAJSONObject(t *testing.T) {
	// The UI branches on docs === 0; a 500 or a bare "null" would read as a
	// broken server rather than an empty index.
	h := newTestServer(&fakeAnswers{corpus: db.Corpus{Documents: []db.Document{}}})

	w := do(t, h, "GET", "/api/corpus", "", nil)
	if w.Code != 200 {
		t.Fatalf("corpus = %d", w.Code)
	}
	if body := strings.TrimSpace(w.Body.String()); !strings.Contains(body, `"documents":[]`) {
		t.Errorf("want an empty array, got %s", body)
	}
}

func TestCorpusFailureIs500(t *testing.T) {
	h := newTestServer(&fakeAnswers{cErr: errors.New("db gone")})
	if code := do(t, h, "GET", "/api/corpus", "", nil).Code; code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", code)
	}
}

func authServer(t *testing.T, a Auth) http.Handler {
	t.Helper()
	return New(Deps{
		Answers: &fakeAnswers{corpus: db.Corpus{Documents: []db.Document{}}},
		Index:   []byte("<html>index</html>"),
		Assets:  fstest.MapFS{"assets/index-A1b2C3d4.js": {Data: []byte("export const x = 1\n")}},
		Auth:    a,
	})
}

func TestNoAuthConfiguredLeavesEverythingOpen(t *testing.T) {
	h := authServer(t, Auth{})
	for _, path := range []string{"/", "/api/health", "/api/corpus", "/assets/index-A1b2C3d4.js"} {
		if code := do(t, h, "GET", path, "", nil).Code; code != 200 {
			t.Errorf("%s = %d with auth off, want 200", path, code)
		}
	}
}

func TestAuthChallengesEveryPathExceptHealth(t *testing.T) {
	h := authServer(t, Auth{User: "team", Pass: "s3cret"})

	// Health stays open so a tunnel or uptime probe needs no credential.
	if code := do(t, h, "GET", "/api/health", "", nil).Code; code != 200 {
		t.Errorf("/api/health = %d, want 200 (probes must not need a secret)", code)
	}

	for _, path := range []string{"/", "/api/corpus", "/app/app.js"} {
		w := do(t, h, "GET", path, "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s = %d without credentials, want 401", path, w.Code)
		}
		// Without this header the browser never shows a prompt.
		if ch := w.Header().Get("WWW-Authenticate"); !strings.HasPrefix(ch, "Basic ") {
			t.Errorf("%s: WWW-Authenticate = %q", path, ch)
		}
	}
	// The chat endpoint must be gated too — it spends the provider key.
	if code := do(t, h, "POST", "/api/chat", `{"question":"x"}`, nil).Code; code != http.StatusUnauthorized {
		t.Errorf("/api/chat = %d without credentials, want 401", code)
	}
}

func TestAuthAcceptsCorrectCredentialsAndRejectsWrongOnes(t *testing.T) {
	h := authServer(t, Auth{User: "team", Pass: "s3cret"})

	cases := map[string]struct {
		user, pass string
		want       int
	}{
		"correct":      {"team", "s3cret", 200},
		"wrong pass":   {"team", "nope", http.StatusUnauthorized},
		"wrong user":   {"other", "s3cret", http.StatusUnauthorized},
		"empty":        {"", "", http.StatusUnauthorized},
		"pass as user": {"s3cret", "team", http.StatusUnauthorized},
	}
	for name, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.SetBasicAuth(c.user, c.pass)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d", name, w.Code, c.want)
		}
	}
}

// The guide is documentation on its own public domain, deliberately not served by
// the app: one surface, one job. So these must be 404s, not a second copy of the
// docs — and this is the test that notices if someone wires them back in.
func TestGuideRoutesAreNotServed(t *testing.T) {
	h := newTestServer(&fakeAnswers{})
	for _, path := range []string{"/docs", "/dev", "/deploy", "/llms.txt"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s returned %d — the app should not serve the guide", path, w.Code)
		}
	}
	// The app itself still answers, so this is not just a broken mux.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != 200 {
		t.Errorf("the app root returned %d", w.Code)
	}
}
