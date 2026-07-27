// Command ingest indexes a folder of documents into the database the server reads.
//
// Only .md / .txt / .markdown are indexed: converting PDF and DOCX belongs in the tool
// that is good at it (Docling, MarkItDown), not in this binary.
//
// Usage: ingest <file-or-dir> [<file-or-dir> ...]
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"knowledge-engine/internal/ai"
	"knowledge-engine/internal/config"
	"knowledge-engine/internal/db"
	"knowledge-engine/internal/rag"
)

// Usage: ingest <file-or-dir> [<file-or-dir> ...]
// Only .md / .txt files are indexed. Convert PDF/DOCX to markdown first
// (Docling / MarkItDown) — keep the Go binary clean.
// main exists only to turn an error into an exit code — see run(), and the same note
// in cmd/server: os.Exit and log.Fatal skip deferred calls, including the one that
// closes the database.
func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		log.Print("usage: ingest <file-or-dir> [<file-or-dir> ...]")
		return errors.New("no paths given")
	}
	cfg := config.Load()
	if cfg.APIKey == "" {
		return errors.New("AI_API_KEY not set (see .env)")
	}

	store, err := db.Open(cfg.DBPath, cfg.EmbedDim)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer store.Close()

	engine := rag.New(store, ai.New(ai.Config{
		ChatBaseURL: cfg.BaseURL, EmbedBaseURL: cfg.EmbedURL,
		APIKey: cfg.APIKey, EmbedAPIKey: cfg.EmbedKey,
		EmbedModel: cfg.EmbedModel, ChatModel: cfg.ChatModel,
	}), rag.Options{TopK: cfg.TopK})
	ctx := context.Background()

	files := collect(args)

	var indexed, chunks, failed int
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			log.Printf("read %s: %v", f, err)
			failed++
			continue
		}
		n, err := engine.Ingest(ctx, docPath(cfg.CorpusDir, f), string(content))
		if err != nil {
			log.Printf("ingest %s: %v", f, err)
			failed++
			continue
		}
		indexed++
		chunks += n
		log.Printf("indexed %s (%d chunks)", f, n)
	}

	// Report what was *indexed*, not what was found, and fail the process if
	// anything didn't make it. Exiting 0 after indexing nothing is how you end up
	// with a server that looks ready and answers "not in the documents" forever.
	log.Printf("done: %d/%d files, %d chunks", indexed, len(files), chunks)
	if failed > 0 {
		return fmt.Errorf("%d file(s) failed — the index is incomplete", failed)
	}
	if chunks == 0 {
		return errors.New("nothing was indexed: no chunks were written")
	}
	return nil
}

// collect expands the arguments into the list of files to index: a file if it is one we
// can read, everything indexable underneath if it is a folder.
//
// Nothing here fails the run. A path that cannot be read is named and skipped, because
// one unreadable file in a corpus of hundreds is not a reason to index none of them —
// but it is a reason to say so, and the count at the end tells the rest of the story.
func collect(args []string) []string {
	var files []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			log.Printf("skip %s: %v", arg, err)
			continue
		}
		if !info.IsDir() {
			if isDoc(arg) {
				files = append(files, arg)
			}
			continue
		}
		// Walk errors are reported, not swallowed. A folder the process cannot read used
		// to produce "0 files" with no reason given, which reads as an empty corpus
		// rather than as a permission problem.
		if err := filepath.WalkDir(arg, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				log.Printf("skip %s: %v", p, err)
				return nil // one unreadable entry must not abandon the rest
			}
			if !d.IsDir() && isDoc(p) {
				files = append(files, p)
			}
			return nil
		}); err != nil {
			log.Printf("walk %s: %v", arg, err)
		}
	}
	return files
}

func isDoc(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".md" || ext == ".txt" || ext == ".markdown"
}

// docPath is the identity a document is stored under. It is the path the UI prints
// beside every citation, and the key a re-ingest updates in place — so it must not
// depend on how ingest happened to be invoked.
//
// Inside the corpus directory it is relative to it. `ingest docs`,
// `ingest /opt/knowledge/docs` and `ingest docs/spec.md` then all agree, and they
// agree with the `qa/ticket-N.md` a BA confirm writes — without this, the same file
// becomes two documents and gets cited twice. An absolute path also puts the server's
// directory layout in front of every reader.
//
// A file from outside the corpus keeps its given path: there is no honest way to
// shorten it, and pretending otherwise would collide with a real corpus entry.
func docPath(corpusDir, file string) string {
	if corpusDir == "" {
		return filepath.Clean(file)
	}
	base, err := filepath.Abs(corpusDir)
	if err != nil {
		return filepath.Clean(file)
	}
	full, err := filepath.Abs(file)
	if err != nil {
		return filepath.Clean(file)
	}
	rel, err := filepath.Rel(base, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Clean(file)
	}
	return filepath.ToSlash(rel)
}
