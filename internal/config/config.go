package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	BindAddr   string
	Port       string
	DBPath     string
	CorpusDir  string
	AssetBase  string
	SiteURL    string
	BaseURL    string
	EmbedURL   string
	APIKey     string
	EmbedKey   string
	EmbedModel string
	ChatModel  string
	EmbedDim   int
	TopK       int
	AuthUser   string
	AuthPass   string
	BAPass     string
}

// DefaultAssetBase is the CDN the frontend loads Vue / marked / DOMPurify /
// 8bit-nes from. Set ASSET_BASE=/vendor to serve them out of the binary instead
// (run `make vendor` first) — required on a network without egress.
const (
	DefaultAssetBase = "https://cdn.jsdelivr.net/npm"
	VendorAssetBase  = "/vendor"

	// DefaultSiteURL is where the guide is published. It appears in the absolute
	// URLs of /llms.txt, which indexes the documentation rather than this host, so a
	// fork should point it at its own Pages site.
	DefaultSiteURL = "https://mega-hub-studio.github.io/mega-docs"
)

func Load() Config {
	loadDotEnv(".env")
	return Config{
		// Loopback by default: this app has no authentication of its own, so
		// binding every interface has to be a deliberate choice, not the default
		// you get by forgetting. Set BIND_ADDR=0.0.0.0 for LAN/Tailscale.
		BindAddr: env("BIND_ADDR", "127.0.0.1"),
		Port:     env("PORT", "8080"),
		DBPath:   env("DB_PATH", "knowledge.db"),
		// The folder `ingest` reads, and where a BA-confirmed answer is written as
		// a markdown file. Keeping both on one path is what makes the database
		// derived: this directory is the source of truth, so put it in git.
		CorpusDir: env("CORPUS_DIR", "docs"),
		AssetBase: env("ASSET_BASE", DefaultAssetBase),
		SiteURL:   env("SITE_URL", DefaultSiteURL),
		BaseURL:   env("AI_BASE_URL", "https://api.openai.com/v1"),
		// Empty means "same as chat". Split it when a gateway serves
		// /chat/completions but not /embeddings — a RAG index needs both.
		EmbedURL: env("EMBED_BASE_URL", ""),
		APIKey:   env("AI_API_KEY", ""),
		// Only needed when EMBED_BASE_URL is a different provider: one key is not
		// valid at two vendors.
		EmbedKey:   env("EMBED_API_KEY", ""),
		EmbedModel: env("EMBED_MODEL", "text-embedding-3-small"),
		ChatModel:  env("CHAT_MODEL", "gpt-4o-mini"),
		EmbedDim:   envInt("EMBED_DIM", 1536),
		TopK:       envInt("TOP_K", 6),
		AuthUser:   env("AUTH_USER", "team"),
		AuthPass:   env("AUTH_PASS", ""), // empty = no auth
		// Gates the two actions that change what the engine will say. Empty means
		// the instance has no write surface at all — reads still work, BA mode says
		// it is read-only. Deliberately separate from AUTH_PASS: everyone who can
		// read shares that one, and confirming into the corpus is not everyone's.
		BAPass: env("BA_PASS", ""),
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// loadDotEnv loads KEY=VALUE lines without overriding existing OS env.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
