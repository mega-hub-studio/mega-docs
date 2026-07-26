package main

import (
	"context"
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
func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: ingest <file-or-dir> ...")
	}
	cfg := config.Load()
	if cfg.APIKey == "" {
		log.Fatal("AI_API_KEY not set (see .env)")
	}

	store, err := db.Open(cfg.DBPath, cfg.EmbedDim)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer store.Close()

	engine := rag.New(store, ai.New(cfg.BaseURL, cfg.EmbedURL, cfg.APIKey, cfg.EmbedModel, cfg.ChatModel), cfg.TopK)
	ctx := context.Background()

	var files []string
	for _, arg := range os.Args[1:] {
		info, err := os.Stat(arg)
		if err != nil {
			log.Printf("skip %s: %v", arg, err)
			continue
		}
		if info.IsDir() {
			filepath.WalkDir(arg, func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && isDoc(p) {
					files = append(files, p)
				}
				return nil
			})
		} else if isDoc(arg) {
			files = append(files, arg)
		}
	}

	var indexed, chunks, failed int
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			log.Printf("read %s: %v", f, err)
			failed++
			continue
		}
		n, err := engine.Ingest(ctx, f, string(content))
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
		log.Printf("%d file(s) failed — the index is incomplete", failed)
		os.Exit(1)
	}
	if chunks == 0 {
		log.Print("nothing was indexed: no chunks were written")
		os.Exit(1)
	}
}

func isDoc(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".md" || ext == ".txt" || ext == ".markdown"
}
