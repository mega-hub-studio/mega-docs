package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"knowledge-engine/internal/rag"
)

// fakeAnswers stands in for the RAG engine — the whole point of the Answerer
// interface, so the HTTP layer is testable without SQLite or an API key.
type fakeAnswers struct {
	tokens []string
	cites  []rag.Citation
	err    error
}

func (f fakeAnswers) Answer(_ context.Context, _ string, onToken func(string)) ([]rag.Citation, error) {
	for _, t := range f.tokens {
		onToken(t)
	}
	return f.cites, f.err
}

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
