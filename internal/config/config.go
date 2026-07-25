package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port       string
	DBPath     string
	AssetBase  string
	BaseURL    string
	APIKey     string
	EmbedModel string
	ChatModel  string
	EmbedDim   int
	TopK       int
}

// DefaultAssetBase is the CDN the frontend loads Vue / marked / DOMPurify /
// 8bit-nes from. Set ASSET_BASE=/vendor to serve them out of the binary instead
// (run `make vendor` first) — required on a network without egress.
const DefaultAssetBase = "https://cdn.jsdelivr.net/npm"

func Load() Config {
	loadDotEnv(".env")
	return Config{
		Port:       env("PORT", "8080"),
		DBPath:     env("DB_PATH", "knowledge.db"),
		AssetBase:  env("ASSET_BASE", DefaultAssetBase),
		BaseURL:    env("AI_BASE_URL", "https://api.openai.com/v1"),
		APIKey:     env("AI_API_KEY", ""),
		EmbedModel: env("EMBED_MODEL", "text-embedding-3-small"),
		ChatModel:  env("CHAT_MODEL", "gpt-4o-mini"),
		EmbedDim:   envInt("EMBED_DIM", 1536),
		TopK:       envInt("TOP_K", 6),
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
