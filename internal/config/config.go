// Package config turns the environment into one Config value, with defaults, and reads
// .env so a secret never has to appear on a command line.
package config

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
)

// Config is every knob this app has, resolved once at startup. A zero value in a
// display field (Window, PriceIn, PriceOut) means "unknown" and the UI prints nothing
// rather than a zero.
type Config struct {
	BindAddr   string
	Port       string
	DBPath     string
	CorpusDir  string
	BaseURL    string
	EmbedURL   string
	APIKey     string
	EmbedKey   string
	EmbedModel string
	ChatModel  string
	EmbedDim   int
	TopK       int
	Window     int     // context window of ChatModel, in tokens; 0 = unknown
	PriceIn    float64 // USD per 1M prompt tokens; 0 = don't price answers
	PriceOut   float64 // USD per 1M completion tokens
	AuthUser   string
	AuthPass   string
	BAPass     string
}

// There is no SITE_URL here, and no ASSET_BASE. The guide is published from this
// repository to its own Pages domain on its own cadence, and the app no longer links out
// to it — one product, one surface, and nothing in the binary that has to be told where
// the documentation moved. The address the *pages* use to reference themselves is
// cmd/rendocs' `-site` flag, which is where it belongs: it is a property of a render, not
// of a running server. Assets are bundled into web/dist by Vite and served from here, so
// there is no CDN to switch away from either.

// Load reads .env (if present) into the environment, then the environment into a Config.
// Existing environment variables win over .env, so a systemd unit or a one-off
// `KEY=value ./bin/server` overrides the file rather than fighting it.
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
		// The status line shows what an answer cost. None of it is guessed: a
		// window of 0 hides the percentage, and prices of 0 hide the money — an
		// invented number in either place is worse than a blank.
		Window:   envInt("CONTEXT_WINDOW", 0),
		PriceIn:  envFloat("PRICE_IN", 0),
		PriceOut: envFloat("PRICE_OUT", 0),
		AuthUser: env("AUTH_USER", "team"),
		AuthPass: env("AUTH_PASS", ""), // empty = no auth
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

// envFloat and envInt both say so when a value is set but unparseable, rather than
// returning the default in silence. The silence was the bug: `TOP_K=six` or a price with a
// comma for a decimal separator became the default, and the symptom — retrieval feels
// wrong, the status line prices nothing — points nowhere near the .env line that caused it.
// Same reasoning as loadDotEnv's own warning below.

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
		log.Printf("config: %s=%q is not a number, using %v", k, v, def)
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
		log.Printf("config: %s=%q is not a whole number, using %d", k, v, def)
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
			// Only fails on an invalid name, which means the .env line is malformed —
			// worth saying, because the setting silently not applying is the symptom.
			if err := os.Setenv(k, v); err != nil {
				log.Printf(".env: skipping %q: %v", k, err)
			}
		}
	}
}
