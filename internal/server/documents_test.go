package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"knowledge-engine/internal/config"
	"knowledge-engine/internal/rag"
)

type fakeImporter struct {
	got     []string // "name:content", in the order the handler called
	removed []string // the paths DELETE asked for, in order
	err     error
}

func (f *fakeImporter) Upload(_ context.Context, name, content string) (rag.Uploaded, error) {
	f.got = append(f.got, name+":"+content)
	if f.err != nil {
		return rag.Uploaded{}, f.err
	}
	return rag.Uploaded{Path: name, Chunks: 3}, nil
}

func (f *fakeImporter) Remove(_ context.Context, name string) (rag.Removed, error) {
	f.removed = append(f.removed, name)
	if f.err != nil {
		return rag.Removed{}, f.err
	}
	return rag.Removed{Path: name, Trash: rag.TrashDir + "/" + name}, nil
}

func importServer(imp Importer, pass BAPass) http.Handler {
	return New(Deps{
		Answers: &fakeAnswers{},
		Docs:    imp,
		Index:   []byte("<html>index</html>"),
		Assets:  fstest.MapFS{"assets/index-A1b2C3d4.js": {Data: []byte("export const x = 1\n")}},
		BAPass:  pass,
	})
}

// form builds a multipart body the way a browser's FormData does.
func form(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, content := range files {
		f, err := w.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(content))
	}
	_ = w.Close()
	return buf.String(), w.FormDataContentType()
}

func postFiles(t *testing.T, h http.Handler, pass string, files map[string]string) (int, importResult) {
	t.Helper()
	body, ct := form(t, files)
	hdr := map[string]string{"Content-Type": ct}
	if pass != "" {
		hdr["X-BA-Pass"] = pass
	}
	w := do(t, h, "POST", "/api/documents", body, hdr)
	var out importResult
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// The gate is the whole difference between "anyone on the tailnet can read the
// documents" and "anyone can rewrite them", so it is checked before anything else.
func TestImportNeedsThePassword(t *testing.T) {
	imp := &fakeImporter{}
	h := importServer(imp, "s3cret")

	for _, c := range []struct {
		name, pass string
		want       int
	}{
		{"no password", "", http.StatusUnauthorized},
		{"wrong password", "nope", http.StatusUnauthorized},
	} {
		code, _ := postFiles(t, h, c.pass, map[string]string{"a.md": "# A"})
		if code != c.want {
			t.Errorf("%s = %d, want %d", c.name, code, c.want)
		}
	}
	if len(imp.got) != 0 {
		t.Fatalf("engine was called despite the gate: %v", imp.got)
	}
}

// A read-only instance must refuse with 403, not 401: there is no password that
// would work, so "try again" is the wrong thing to tell a client.
func TestImportOnAReadOnlyInstance(t *testing.T) {
	imp := &fakeImporter{}
	code, _ := postFiles(t, importServer(imp, ""), "anything", map[string]string{"a.md": "# A"})
	if code != http.StatusForbidden {
		t.Fatalf("read-only import = %d, want 403", code)
	}
	if len(imp.got) != 0 {
		t.Fatalf("engine was called on a read-only instance: %v", imp.got)
	}
}

func TestImportIndexesEachFile(t *testing.T) {
	imp := &fakeImporter{}
	code, out := postFiles(t, importServer(imp, "s3cret"), "s3cret", map[string]string{
		"spec.md": "# Spec\n\nbody",
	})
	if code != http.StatusOK {
		t.Fatalf("import = %d, want 200", code)
	}
	if len(out.Uploaded) != 1 || out.Uploaded[0].Path != "spec.md" || out.Chunks != 3 {
		t.Fatalf("uploaded = %+v, chunks = %d", out.Uploaded, out.Chunks)
	}
	if len(imp.got) != 1 || imp.got[0] != "spec.md:# Spec\n\nbody" {
		t.Fatalf("engine saw %q", imp.got)
	}
}

// Importing a folder means importing whatever is in it. Failing the batch because
// one file is a PDF would make the user find the bad one by bisection.
func TestImportReportsPartialFailure(t *testing.T) {
	imp := &fakeImporter{}
	code, out := postFiles(t, importServer(imp, "s3cret"), "s3cret", map[string]string{
		"good.md":  "# Good",
		"scan.pdf": "%PDF-1.7",
	})
	if code != http.StatusOK {
		t.Fatalf("partial import = %d, want 200", code)
	}
	if len(out.Uploaded) != 1 || out.Uploaded[0].Path != "good.md" {
		t.Fatalf("uploaded = %+v", out.Uploaded)
	}
	if len(out.Failed) != 1 || out.Failed[0].Name != "scan.pdf" {
		t.Fatalf("failed = %+v", out.Failed)
	}
	if !strings.Contains(out.Failed[0].Error, "markitdown") {
		t.Errorf("the .pdf rejection should name the fix, got %q", out.Failed[0].Error)
	}
	// The engine must never have been asked to write the PDF.
	if len(imp.got) != 1 || !strings.HasPrefix(imp.got[0], "good.md:") {
		t.Fatalf("engine saw %q", imp.got)
	}
}

// Nothing indexed is a failed request. A 200 here would let a client's "imported!"
// toast fire on a batch where every file was rejected.
func TestImportWithNothingUsableIs400(t *testing.T) {
	code, out := postFiles(t, importServer(&fakeImporter{}, "s3cret"), "s3cret", map[string]string{
		"scan.pdf": "%PDF",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("all-rejected import = %d, want 400", code)
	}
	if len(out.Failed) != 1 {
		t.Fatalf("failed = %+v", out.Failed)
	}
}

func TestImportWithNoFiles(t *testing.T) {
	body, ct := form(t, nil)
	w := do(t, importServer(&fakeImporter{}, "s3cret"), "POST", "/api/documents", body,
		map[string]string{"Content-Type": ct, "X-BA-Pass": "s3cret"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty import = %d, want 400", w.Code)
	}
}

// An engine with no importer wired must not answer the route at all, the same way
// the QA routes disappear without a Knowledge.
func TestImportRouteAbsentWithoutAnImporter(t *testing.T) {
	h := New(Deps{Answers: &fakeAnswers{}, Index: []byte("x"), Assets: fstest.MapFS{}, BAPass: "s3cret"})
	if w := do(t, h, "POST", "/api/documents", "", nil); w.Code != http.StatusNotFound {
		t.Fatalf("POST /api/documents = %d, want 404", w.Code)
	}
}

// The removal route, and the two things about it that are easy to get wrong.
//
// It is gated: an unset BA_PASS must mean no delete surface at all, not an open one — the
// same rule the import and confirm routes follow, and the one that turns "forgot to
// configure a secret" into a missing feature rather than an open door.
func TestRemovingADocumentIsGated(t *testing.T) {
	t.Run("unset BA_PASS refuses", func(t *testing.T) {
		imp := &fakeImporter{}
		srv := importServer(imp, "")
		res := do(t, srv, http.MethodDelete, "/api/documents/booking/pricing.md", "", nil)
		if res.Code != http.StatusForbidden {
			t.Errorf("want 403 with no password configured, got %d", res.Code)
		}
		if len(imp.removed) != 0 {
			t.Errorf("the engine was called anyway: %v", imp.removed)
		}
	})

	t.Run("wrong password refuses", func(t *testing.T) {
		imp := &fakeImporter{}
		srv := importServer(imp, "right")
		res := do(t, srv, http.MethodDelete, "/api/documents/booking/pricing.md", "",
			map[string]string{"X-BA-Pass": "wrong"})
		if res.Code != http.StatusUnauthorized {
			t.Errorf("want 401 with the wrong password, got %d", res.Code)
		}
		if len(imp.removed) != 0 {
			t.Errorf("the engine was called anyway: %v", imp.removed)
		}
	})

	// And the path must arrive whole. `{path...}` is what makes a nested document
	// deletable at all: without the trailing wildcard, "booking/pricing.md" matches no
	// route and a folder of documents becomes undeletable through the UI.
	t.Run("a nested path arrives whole", func(t *testing.T) {
		imp := &fakeImporter{}
		srv := importServer(imp, "pw")
		res := do(t, srv, http.MethodDelete, "/api/documents/business/pricing/2026.md", "",
			map[string]string{"X-BA-Pass": "pw"})
		if res.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", res.Code, res.Body.String())
		}
		if len(imp.removed) != 1 || imp.removed[0] != "business/pricing/2026.md" {
			t.Errorf("the engine got %v, want the full nested path", imp.removed)
		}
		if !strings.Contains(res.Body.String(), rag.TrashDir) {
			t.Errorf("the reply does not say where the file went: %s", res.Body.String())
		}
	})

	// A document name is whatever someone called their file, so it reaches this route
	// percent-encoded — `lib/upload.js` encodes each segment separately, keeping "/" as the
	// separator while a space or a "#" is escaped. This asserts the other half of that
	// contract: ServeMux unescapes the wildcard, so the engine is handed the real name. Get
	// it wrong and "Q3 pricing.md" is undeletable through the UI while every test with a
	// tidy file name passes.
	t.Run("an encoded name is unescaped before the engine sees it", func(t *testing.T) {
		imp := &fakeImporter{}
		srv := importServer(imp, "pw")
		res := do(t, srv, http.MethodDelete, "/api/documents/business/Q3%20pricing%20%232.md", "",
			map[string]string{"X-BA-Pass": "pw"})
		if res.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", res.Code, res.Body.String())
		}
		if len(imp.removed) != 1 || imp.removed[0] != "business/Q3 pricing #2.md" {
			t.Errorf("the engine got %q, want the decoded name", imp.removed)
		}
	})
}

// ── the Admin screen's one endpoint ───────────────────────────────────────────
// It lives with the document tests because it shares their shape — a gated read on a
// nil-able seam — and rule 21 says extend the file that already owns that, not add another.

func adminServer(inv func() any, pass AdminPass) http.Handler {
	return New(Deps{
		Answers:   &fakeAnswers{},
		Index:     []byte("<html>index</html>"),
		Assets:    fstest.MapFS{"assets/index-A1b2C3d4.js": {Data: []byte("export const x = 1\n")}},
		Settings:  inv,
		AdminPass: pass,
	})
}

// Rule — an unset ADMIN_PASS removes the surface rather than opening it, and the route is
// not even registered: the front end reads /api/health to decide whether the Admin tab
// exists, so a 404 here and an absent tab are the same fact.
func TestSettingsNeedTheAdminPassword(t *testing.T) {
	inv := func() any { return []map[string]string{{"name": "TOP_K", "value": "6"}} }

	t.Run("unset ADMIN_PASS leaves the route unregistered", func(t *testing.T) {
		// 404, not 403: `settings` returns before HandleFunc, so there is no handler to
		// refuse. That is the difference between "you may not" and "there is nothing here",
		// and it is what keeps an admin-less instance from advertising an admin surface.
		res := do(t, adminServer(inv, ""), http.MethodGet, "/api/settings", "", nil)
		if res.Code != http.StatusNotFound {
			t.Errorf("GET /api/settings = %d, want 404 when ADMIN_PASS is unset", res.Code)
		}
	})

	t.Run("no header refuses", func(t *testing.T) {
		res := do(t, adminServer(inv, "pw"), http.MethodGet, "/api/settings", "", nil)
		if res.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401 without the header", res.Code)
		}
	})

	t.Run("the BA password does not open it", func(t *testing.T) {
		// Separate secrets on purpose: publishing an answer and reading which passwords
		// exist on the box are different permissions. Sending the right value in the wrong
		// header must not pass either.
		res := do(t, adminServer(inv, "pw"), http.MethodGet, "/api/settings", "",
			map[string]string{"X-BA-Pass": "pw"})
		if res.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401 for a BA password in the BA header", res.Code)
		}
	})

	t.Run("the admin password opens it", func(t *testing.T) {
		res := do(t, adminServer(inv, "pw"), http.MethodGet, "/api/settings", "",
			map[string]string{"X-Admin-Pass": "pw"})
		if res.Code != http.StatusOK {
			t.Fatalf("got %d, want 200: %s", res.Code, res.Body.String())
		}
		if !strings.Contains(res.Body.String(), "TOP_K") {
			t.Errorf("the reply does not carry the inventory: %s", res.Body.String())
		}
	})
}

// Rule — a secret's value never leaves the process. This screen is what an operator
// screenshots when asking for help, so a key on it is a key in a chat thread. The inventory
// is built in internal/config, and this asserts the property end to end over the real one.
func TestSettingsRedactEverySecret(t *testing.T) {
	const leak = "sk-do-not-print-me"
	t.Setenv("AI_API_KEY", leak)
	t.Setenv("BA_PASS", leak)
	t.Setenv("ADMIN_PASS", leak)
	t.Setenv("AUTH_PASS", leak)
	t.Setenv("EMBED_API_KEY", leak)

	cfg := config.Load()
	srv := adminServer(func() any { return cfg.Inventory() }, AdminPass(cfg.AdminPass))
	res := do(t, srv, http.MethodGet, "/api/settings", "",
		map[string]string{"X-Admin-Pass": leak})
	if res.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if strings.Contains(body, leak) {
		t.Errorf("a secret's value reached the response: %s", body)
	}
	// Not vacuous: the five secrets must be *present* as rows, reporting that they are set.
	// A response that simply omitted them would pass the check above for the wrong reason.
	for _, name := range []string{"AI_API_KEY", "BA_PASS", "ADMIN_PASS", "AUTH_PASS", "EMBED_API_KEY"} {
		if !strings.Contains(body, name) {
			t.Errorf("%s is not in the inventory at all", name)
		}
	}
	if n := strings.Count(body, `"value":"set"`); n != 5 {
		t.Errorf("got %d secrets reported as set, want 5: %s", n, body)
	}
}
