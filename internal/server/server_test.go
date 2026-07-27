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
	err    error
	corpus db.Corpus
	cErr   error
}

func (f fakeAnswers) Answer(_ context.Context, _ string, onToken func(string)) ([]rag.Citation, error) {
	for _, t := range f.tokens {
		onToken(t)
	}
	return f.cites, f.err
}

func (f fakeAnswers) Corpus(int) (db.Corpus, error) { return f.corpus, f.cErr }

func newTestServer(a Answerer) http.Handler {
	return New(Deps{
		Answers: a,
		Index:   []byte("<html>index</html>"),
		Assets: fstest.MapFS{
			"app/app.js":                    {Data: []byte("export const x = 1\n")},
			"vendor/vue@3.5.40/dist/vue.js": {Data: []byte("/* vue */\n")},
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
	w := do(t, newTestServer(fakeAnswers{}), "GET", "/api/health", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("health = %d %q", w.Code, w.Body.String())
	}
}

func TestIndexIsRevalidatedAndCachesWithETag(t *testing.T) {
	h := newTestServer(fakeAnswers{})

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

func TestAppTreeRevalidatesAndVendorIsImmutable(t *testing.T) {
	h := newTestServer(fakeAnswers{})

	app := do(t, h, "GET", "/app/app.js", "", nil)
	if app.Code != 200 {
		t.Fatalf("app.js = %d", app.Code)
	}
	if got := app.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("app Cache-Control = %q, want no-cache", got)
	}
	if app.Header().Get("ETag") == "" {
		t.Error("app tree served without an ETag — every load would re-download it")
	}
	// Module scripts are rejected unless the MIME type is JavaScript.
	if ct := app.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("app.js Content-Type = %q, want a JavaScript type", ct)
	}

	// Vendored paths carry their version, so they can be cached forever.
	v := do(t, h, "GET", "/vendor/vue@3.5.40/dist/vue.js", "", nil)
	if v.Code != 200 {
		t.Fatalf("vendor = %d", v.Code)
	}
	if got := v.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("vendor Cache-Control = %q, want immutable", got)
	}
}

func TestChatStreamsTokensThenCitationsThenDone(t *testing.T) {
	h := newTestServer(fakeAnswers{
		tokens: []string{"Hybrid ", "search [1]"},
		cites:  []rag.Citation{{N: 1, DocPath: "docs/a.md", Heading: "How"}},
	})

	w := do(t, h, "POST", "/api/chat", `{"question":"how?"}`, nil)
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
		"event: done\ndata: {\"done\":true}\n\n",
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
	h := newTestServer(fakeAnswers{tokens: []string{"nothing found"}, cites: nil})

	body := do(t, h, "POST", "/api/chat", `{"question":"x"}`, nil).Body.String()
	if !strings.Contains(body, "event: citations\ndata: []\n\n") {
		t.Errorf("want an empty citation array, got:\n%s", body)
	}
}

func TestChatReportsEngineFailureInStream(t *testing.T) {
	h := newTestServer(fakeAnswers{tokens: []string{"partial"}, err: errors.New("provider down")})

	body := do(t, h, "POST", "/api/chat", `{"question":"x"}`, nil).Body.String()
	if !strings.Contains(body, `event: error`) || !strings.Contains(body, "provider down") {
		t.Errorf("want an error event, got:\n%s", body)
	}
	if strings.Contains(body, "event: done") {
		t.Error("a failed answer must not also report done")
	}
}

func TestChatRejectsBadRequests(t *testing.T) {
	h := newTestServer(fakeAnswers{})
	for name, body := range map[string]string{
		"not json":        `{`,
		"missing field":   `{}`,
		"empty question":  `{"question":""}`,
		"blank question":  `{"question":"   "}`,
		"wrong json type": `{"question":42}`,
	} {
		if code := do(t, h, "POST", "/api/chat", body, nil).Code; code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", name, code)
		}
	}
}

func TestCorpusReportsWhatIsIndexed(t *testing.T) {
	h := newTestServer(fakeAnswers{corpus: db.Corpus{
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
	h := newTestServer(fakeAnswers{corpus: db.Corpus{Documents: []db.Document{}}})

	w := do(t, h, "GET", "/api/corpus", "", nil)
	if w.Code != 200 {
		t.Fatalf("corpus = %d", w.Code)
	}
	if body := strings.TrimSpace(w.Body.String()); !strings.Contains(body, `"documents":[]`) {
		t.Errorf("want an empty array, got %s", body)
	}
}

func TestCorpusFailureIs500(t *testing.T) {
	h := newTestServer(fakeAnswers{cErr: errors.New("db gone")})
	if code := do(t, h, "GET", "/api/corpus", "", nil).Code; code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", code)
	}
}

func authServer(t *testing.T, a Auth) http.Handler {
	t.Helper()
	return New(Deps{
		Answers: fakeAnswers{corpus: db.Corpus{Documents: []db.Document{}}},
		Index:   []byte("<html>index</html>"),
		Assets:  fstest.MapFS{"app/app.js": {Data: []byte("export const x = 1\n")}},
		Auth:    a,
	})
}

func TestNoAuthConfiguredLeavesEverythingOpen(t *testing.T) {
	h := authServer(t, Auth{})
	for _, path := range []string{"/", "/api/health", "/api/corpus", "/app/app.js"} {
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
		r := httptest.NewRequest("GET", "/", nil)
		r.SetBasicAuth(c.user, c.pass)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d", name, w.Code, c.want)
		}
	}
}

// Guide pages are routed from a map keyed by address, so adding a role needs no
// change in this package. What must hold: every entry is served, an entry that is
// nil or absent is a 404 rather than a blank 200, and the pages are ETag'd like the
// index (a doc page changes only when the binary does).
func TestPagesMapIsRoutedByAddress(t *testing.T) {
	h := New(Deps{
		Answers: fakeAnswers{},
		Index:   []byte("<html>index</html>"),
		Pages: map[string][]byte{
			"/docs":     []byte("<html>guide</html>"),
			"/dev":      []byte("<html>dev</html>"),
			"/deploy":   []byte("<html>deploy</html>"),
			"/llms.txt": []byte("# index\n"), // plain text, not HTML
			"/nope":     nil,                 // a page that failed to render must not become a route
		},
		Assets: fstest.MapFS{"app/app.js": {Data: []byte("export const x = 1\n")}},
	})

	for path, want := range map[string]string{
		"/docs": "guide", "/dev": "dev", "/deploy": "deploy",
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Errorf("%s: got %d, want 200", path, w.Code)
			continue
		}
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("%s served the wrong page: %q", path, w.Body.String())
		}
		if w.Header().Get("ETag") == "" {
			t.Errorf("%s went out without an ETag", path)
		}
	}

	// llms.txt must not go out as text/html: a browser would try to render it and an
	// agent would have to guess.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/llms.txt", nil))
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("/llms.txt served as %q, want text/plain", ct)
	}

	for _, path := range []string{"/nope", "/unknown"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, w.Code)
		}
	}
}
