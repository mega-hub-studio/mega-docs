// Package config turns the environment into one Config value, with defaults, and reads
// .env so a secret never has to appear on a command line.
package config

import (
	"bufio"
	"errors"
	"fmt"
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
	// ThreadShare is how much of a model's context window a conversation may take, and
	// CacheKeep how many answers the cache holds. Both were constants inside the engine, and
	// both are here for the same reason: a number an operator would want to move after
	// watching their own corpus behave has no business being a recompile.
	ThreadShare float64
	CacheKeep   int
	Window      int     // context window of ChatModel, in tokens; 0 = unknown
	PriceIn     float64 // USD per 1M prompt tokens; 0 = don't price answers
	PriceOut    float64 // USD per 1M completion tokens
	// Models is what a reader may pick between, first one the default. It is always at
	// least one entry: with CHAT_MODELS unset it is the single model the four knobs above
	// describe, which is exactly the instance that never wanted a picker.
	Models    []Model
	AuthUser  string
	AuthPass  string
	BAPass    string
	AdminPass string
	// fromFile is the set of keys .env supplied, which is the one thing os.Getenv cannot
	// answer afterwards: loadDotEnv copies them into the environment, so by the time
	// anything reads them a file value and a shell value look identical. Unexported — it is
	// provenance for the Admin screen, not a knob.
	fromFile map[string]bool
}

// Model is one chat model a reader may answer with, and the two numbers the status line
// cannot guess about it. Zero in either display field means "unknown", which prints
// nothing — the same rule the single-model knobs already follow, per model now.
type Model struct {
	Name     string  `json:"name"`
	Window   int     `json:"window"`    // context window in tokens; 0 = unknown
	PriceIn  float64 `json:"price_in"`  // USD per 1M prompt tokens; 0 = don't price
	PriceOut float64 `json:"price_out"` // USD per 1M completion tokens
}

// parseModels reads CHAT_MODELS: `name:window:$in:$out`, comma-separated, first is the
// default. Everything after the name is optional, so `CHAT_MODELS=gpt-4o-mini,gpt-4o` is a
// legal two-model instance that simply prints no percentage and no cost.
//
// It returns an error rather than skipping a bad entry, and Load turns that into a refusal
// to start. A typo here is not a cosmetic fault: the list is the allowlist the server
// validates against and the price table the status line bills from, so a silently dropped
// entry is a model a reader cannot pick and a wrong number is money reported wrong. The
// same trade `web/vendor.sha384` already makes — fail at startup, not in a browser.
func parseModels(spec string) ([]Model, error) {
	var out []Model
	for entry := range strings.SplitSeq(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, rest, _ := strings.Cut(entry, ":")
		m := Model{Name: strings.TrimSpace(name)}
		if m.Name == "" {
			return nil, fmt.Errorf("entry %q has no model name", entry)
		}
		fields := []any{&m.Window, &m.PriceIn, &m.PriceOut}
		for i, field := range fields {
			var have string
			have, rest, _ = strings.Cut(rest, ":")
			have = strings.TrimSpace(have)
			if have == "" {
				break
			}
			if err := assign(field, have); err != nil {
				return nil, fmt.Errorf("%s: field %d (%q): %w", m.Name, i+2, have, err)
			}
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, errors.New("no model named")
	}
	return out, nil
}

// assign parses one numeric field of a CHAT_MODELS entry into whichever of the two kinds
// it is. Two-line type switch rather than three near-identical parse blocks above.
func assign(field any, text string) error {
	switch f := field.(type) {
	case *int:
		n, err := strconv.Atoi(text)
		*f = n
		return err
	case *float64:
		n, err := strconv.ParseFloat(text, 64)
		*f = n
		return err
	}
	return nil
}

// Setting is one knob as an operator sees it on the Admin screen: what it is called, what
// it resolved to, and — the part that used to be a guess — where that value came from.
//
// Value is the *effective* value out of Config rather than the raw environment, so a
// default reads as the number actually in use. A secret never carries its value; Value is
// "set" or "unset" and Secret says to style it as a state rather than a number.
type Setting struct {
	Group  string `json:"group"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source string `json:"source"` // ".env" · "env" · "default"
	Secret bool   `json:"secret"`
}

// No SITE_URL, and no ASSET_BASE: both were deleted, and
// changelog/2026-07-28-drop-guide-link.md is why.

// Load reads .env (if present) into the environment, then the environment into a Config.
// Existing environment variables win over .env, so a systemd unit or a one-off
// `KEY=value ./bin/server` overrides the file rather than fighting it.
func Load() Config {
	fromFile := loadDotEnv(".env")
	return Config{
		fromFile: fromFile,
		// Loopback by default: this app has no authentication of its own, so
		// binding every interface has to be a deliberate choice, not the default
		// you get by forgetting. Set BIND_ADDR=0.0.0.0 for LAN/Tailscale.
		BindAddr: env("BIND_ADDR", "127.0.0.1"),
		Port:     env("PORT", "8080"),
		DBPath:   env("DB_PATH", "knowledge.db"),
		// Only the folder `ingest` reads when an operator imports from disk. Nothing
		// writes there, so read access is enough — the documents themselves live in
		// DB_PATH, which is the source of truth (invariant 1).
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
		// 0.35 leaves two thirds of the window for the retrieved sections and the completion.
		// Raising it buys memory by making answers less grounded, which is why it is a knob
		// rather than a preference: the trade has to be made on purpose.
		ThreadShare: envFloat("THREAD_SHARE", 0.35),
		// 200 answers is more than a team asks between re-indexes, and it keeps the one-file
		// story honest — a database small enough to copy in a second is one nobody plans around.
		CacheKeep: envInt("CACHE_KEEP", 200),
		// The status line shows what an answer cost. None of it is guessed: a
		// window of 0 hides the percentage, and prices of 0 hide the money — an
		// invented number in either place is worse than a blank.
		Window:   envInt("CONTEXT_WINDOW", 0),
		PriceIn:  envFloat("PRICE_IN", 0),
		PriceOut: envFloat("PRICE_OUT", 0),
		Models:   models(),
		AuthUser: env("AUTH_USER", "team"),
		AuthPass: env("AUTH_PASS", ""), // empty = no auth
		// Gates the two actions that change what the engine will say. Empty means
		// the instance has no write surface at all — reads still work, BA mode says
		// it is read-only. Deliberately separate from AUTH_PASS: everyone who can
		// read shares that one, and confirming into the corpus is not everyone's.
		BAPass: env("BA_PASS", ""),
		// Gates the Admin screen, which lists every setting below with the provenance of
		// each — including which passwords exist. Unset means that screen and its route do
		// not exist, the same shape as BA_PASS: a missing secret removes a surface rather
		// than opening one.
		AdminPass: env("ADMIN_PASS", ""),
	}
}

// Inventory is every knob, grouped, for the Admin screen to render. One list, next to the
// definitions it describes — a second copy of "what the knobs are" is how a new one ends up
// invisible. This is also the order .env.example follows, and the direction matters: that
// file names the keys worth a decision and points here for the rest, because a default
// written down twice is a default that goes stale in the copy nobody compiles.
func (c Config) Inventory() []Setting {
	num := strconv.Itoa
	return []Setting{
		c.set("provider", "AI_BASE_URL", c.BaseURL),
		c.secret("provider", "AI_API_KEY", c.APIKey),
		c.set("provider", "EMBED_BASE_URL", embedURL(c.EmbedURL)),
		c.secret("provider", "EMBED_API_KEY", c.EmbedKey),
		c.set("models", "CHAT_MODEL", c.ChatModel),
		c.set("models", "CHAT_MODELS", modelList(c.Models)),
		c.set("models", "EMBED_MODEL", c.EmbedModel),
		c.set("models", "EMBED_DIM", num(c.EmbedDim)),
		c.set("models", "TOP_K", num(c.TopK)),
		c.set("models", "THREAD_SHARE", strconv.FormatFloat(c.ThreadShare, 'g', -1, 64)),
		c.set("models", "CACHE_KEEP", num(c.CacheKeep)),
		c.set("hosting", "BIND_ADDR", c.BindAddr),
		c.set("hosting", "PORT", c.Port),
		c.set("hosting", "AUTH_USER", c.AuthUser),
		c.secret("hosting", "AUTH_PASS", c.AuthPass),
		c.secret("hosting", "BA_PASS", c.BAPass),
		c.secret("hosting", "ADMIN_PASS", c.AdminPass),
		c.set("storage", "CORPUS_DIR", c.CorpusDir),
		c.set("storage", "DB_PATH", c.DBPath),
		c.set("status line", "CONTEXT_WINDOW", num(c.Window)),
		c.set("status line", "PRICE_IN", strconv.FormatFloat(c.PriceIn, 'g', -1, 64)),
		c.set("status line", "PRICE_OUT", strconv.FormatFloat(c.PriceOut, 'g', -1, 64)),
	}
}

func (c Config) set(group, name, value string) Setting {
	return Setting{Group: group, Name: name, Value: value, Source: c.source(name)}
}

// secret reports existence and nothing else. The Admin screen is a page an operator
// screenshots when asking for help, and a key on it is a key in a chat thread.
func (c Config) secret(group, name, value string) Setting {
	state := "unset"
	if value != "" {
		state = "set"
	}
	return Setting{Group: group, Name: name, Value: state, Source: c.source(name), Secret: true}
}

// source is the whole reason the screen exists: .env, the shell, or nobody — which
// os.Getenv alone cannot tell apart once loadDotEnv has copied the file in.
func (c Config) source(name string) string {
	switch {
	case c.fromFile[name]:
		return ".env"
	case os.Getenv(name) != "":
		return "env"
	default:
		return "default"
	}
}

// embedURL says "(same as chat)" rather than showing a blank, because empty here is a
// decision — one provider for both endpoints — and a blank cell reads as a missing value.
func embedURL(v string) string {
	if v == "" {
		return "(same as chat)"
	}
	return v
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

// loadDotEnv loads KEY=VALUE lines without overriding existing OS env, and returns the
// keys it actually supplied — the provenance the Admin screen reports, and the one fact
// that is unrecoverable afterwards: these are copied into the environment, so a file value
// and a shell value are indistinguishable to os.Getenv from here on.
func loadDotEnv(path string) map[string]bool {
	set := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return set
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
				continue
			}
			// Recorded *after* the Setenv succeeds and only in this branch: a key the
			// shell already had did not come from the file. Forgetting this line is how
			// the Admin screen reported every .env value as "env" — the one thing it
			// exists to tell apart — while looking entirely healthy.
			set[k] = true
		}
	}
	return set
}

// models resolves what a reader may pick between.
//
// CHAT_MODELS unset is not a missing setting, it is the single-model instance: the four
// knobs that already describe one model *are* the one entry, so nothing needs re-configuring
// to keep working and no fact is written twice. Set it and it supersedes all four — which is
// why the Admin screen lists both rows, with CHAT_MODEL reading as the fallback it now is.
//
// A malformed list is fatal, deliberately. Every alternative is worse: falling back to the
// default model answers questions with something the operator did not choose, and dropping
// the bad entry hides a typo in a spending allowlist.
func models() []Model {
	spec := env("CHAT_MODELS", "")
	if strings.TrimSpace(spec) == "" {
		return []Model{{
			Name:     env("CHAT_MODEL", "gpt-4o-mini"),
			Window:   envInt("CONTEXT_WINDOW", 0),
			PriceIn:  envFloat("PRICE_IN", 0),
			PriceOut: envFloat("PRICE_OUT", 0),
		}}
	}
	list, err := parseModels(spec)
	if err != nil {
		log.Fatalf("CHAT_MODELS is unreadable (%v)\n"+
			"  format: name:window:$in-per-1M:$out-per-1M, comma-separated, first is the default\n"+
			"  example: CHAT_MODELS=gpt-4o-mini:128000:0.15:0.60,gpt-4o:128000:2.50:10.00", err)
	}
	return list
}

// modelList is the CHAT_MODELS row on the Admin screen: the names only. The windows and
// prices are already on the screen as their own rows, and repeating them here would be the
// same number twice in one list.
func modelList(list []Model) string {
	names := make([]string, 0, len(list))
	for _, m := range list {
		names = append(names, m.Name)
	}
	return strings.Join(names, " · ")
}
