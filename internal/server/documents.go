package server

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"

	"knowledge-engine/internal/rag"
)

// Importer is the import side of the engine: a document arriving over HTTP instead
// of from the host's disk. Separate from Knowledge so the QA loop and the import
// surface can be faked — and refused — independently.
type Importer interface {
	Upload(ctx context.Context, name, content string) (rag.Uploaded, error)
	// Remove takes a document out of the index and moves its file to the corpus's
	// trash. On the same interface as Upload because they are one capability — a BA
	// who may add what everyone reads may also take it back — and nil-ing the seam
	// must remove both or the surface half-works.
	Remove(ctx context.Context, name string) (rag.Removed, error)
}

const (
	// maxDoc caps one document. Markdown that a person wrote and a person will
	// read; well past a long spec, well short of something that belongs in object
	// storage.
	maxDoc = 2 << 20 // 2 MiB
	// maxImport caps the whole request, so "select all" in a folder of the wrong
	// kind of file cannot spend the process's memory before the per-file check runs.
	maxImport = 16 << 20 // 16 MiB
)

// documents wires the import route. One endpoint, behind the same password as a
// confirm, because both change what every reader is told.
//
//	POST /api/documents   multipart/form-data, field "files" (repeatable)
//	                   → {"uploaded":[{"path","chunks"}],"failed":[{"name","error"}]}
//
// Partial success is reported, not hidden: importing eight files where one is a PDF
// should index the seven and name the eighth, rather than failing the batch and
// leaving the user to guess which one was wrong.
func documents(mux *http.ServeMux, imp Importer, pass BAPass) {
	// DELETE /api/documents/{path...}
	//
	// The path is in the URL rather than a body, because it is the document's identity
	// and DELETE has no body worth parsing. `{path...}` is Go 1.22's trailing wildcard,
	// so "booking/pricing.md" arrives whole — and it is passed to rag.Remove unvalidated
	// on purpose: SafePath is the one place that decides what a document path may be, and
	// a second opinion here would be a second thing to keep in agreement with it.
	mux.HandleFunc("DELETE /api/documents/{path...}", pass.gate(func(w http.ResponseWriter, r *http.Request) {
		removed, err := imp.Remove(r.Context(), r.PathValue("path"))
		if err != nil {
			// The engine's errors here are all "this path is not usable" or "it is not
			// in the corpus" — a client mistake, not a server fault.
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, removed)
	}))

	mux.HandleFunc("POST /api/documents", pass.gate(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxImport)
		if err := r.ParseMultipartForm(maxDoc); err != nil {
			http.Error(w, "the upload was too large or malformed", http.StatusBadRequest)
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }() // temp files; a failed cleanup is the OS's problem

		files := r.MultipartForm.File["files"]
		if len(files) == 0 {
			http.Error(w, `no files: send them as multipart field "files"`, http.StatusBadRequest)
			return
		}

		// The folder the batch lands in. Sent separately from the file name because
		// a browser only reveals a relative path when a whole directory was picked —
		// for loose files the folder is a choice the person makes, and it is the one
		// thing that decides what a reader can later scope a question to.
		dir := r.FormValue("dir")

		out := importResult{Uploaded: []rag.Uploaded{}, Failed: []importFailure{}}
		for _, fh := range files {
			doc, err := readUpload(r.Context(), imp, dir, fh)
			if err != nil {
				out.Failed = append(out.Failed, importFailure{Name: fh.Filename, Error: err.Error()})
				continue
			}
			out.Uploaded = append(out.Uploaded, doc)
			out.Chunks += doc.Chunks
		}
		// Nothing landed: this was a failed request, not a successful one that
		// happens to report failures — a client should be able to tell by status.
		if len(out.Uploaded) == 0 {
			w.WriteHeader(http.StatusBadRequest)
		}
		writeJSON(w, out)
	}))
}

type importResult struct {
	Uploaded []rag.Uploaded  `json:"uploaded"`
	Failed   []importFailure `json:"failed"`
	Chunks   int             `json:"chunks"`
}

type importFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// readUpload validates before it reads: the name decides whether the bytes are
// worth pulling into memory at all, and a 40 MB PDF should be refused on its
// extension rather than on its size.
func readUpload(ctx context.Context, imp Importer, dir string, fh *multipart.FileHeader) (rag.Uploaded, error) {
	// The chosen folder is prefixed here, so the engine still receives one path and
	// applies one rule to it. Joining before validation is what makes "../" in the
	// folder box as harmless as "../" in a file name.
	name := path.Join(dir, fh.Filename)
	if _, err := rag.SafePath(name); err != nil {
		return rag.Uploaded{}, err
	}
	if fh.Size > maxDoc {
		return rag.Uploaded{}, fmt.Errorf("%s is %d KB — the limit is %d KB", fh.Filename, fh.Size>>10, maxDoc>>10)
	}
	f, err := fh.Open()
	if err != nil {
		return rag.Uploaded{}, fmt.Errorf("%s could not be read", fh.Filename)
	}
	defer f.Close()

	body, err := io.ReadAll(io.LimitReader(f, maxDoc))
	if err != nil {
		return rag.Uploaded{}, fmt.Errorf("%s could not be read", fh.Filename)
	}
	return imp.Upload(ctx, name, string(body))
}
