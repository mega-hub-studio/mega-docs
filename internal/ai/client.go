// Package ai is a minimal OpenAI-compatible client (embeddings + streaming chat).
// One OpenAI-compatible client. The MVP targets OpenAI only — a single vendor is the
// whole of the AI stack, and provider choice is a SaaS-phase plugin, not an MVP knob.
// The wire format is still the OpenAI one, so Azure/Groq/OpenRouter work by base URL alone;
// that is a property of the protocol, not a feature this repo maintains.
// and any gateway that speaks the same two endpoints.
//
// Embeddings and chat carry separate base URLs on purpose: plenty of gateways
// serve /chat/completions but not /embeddings, and a RAG index needs both. Point
// EmbedBaseURL somewhere that has them and keep chat
// wherever you want it.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Timeouts: embeddings are a normal request/response, so a total deadline is
// right. A chat stream is long-lived by design — bounding the *whole* call would
// cut a long answer off mid-sentence, so only the wait for response headers is
// capped there.
const (
	embedTimeout       = 60 * time.Second
	chatHeaderTimeout  = 45 * time.Second
	maxErrBody         = 2 << 10 // enough to read a provider's message, not its HTML
	maxSSELineBytes    = 1 << 20
	initialSSELineSize = 64 << 10
)

// Client is one OpenAI-compatible provider: embeddings, streaming chat, and what the
// completion cost. Chat and embeddings may live at different base URLs with different
// keys, because a coding-agent gateway often serves chat only.
type Client struct {
	ChatBaseURL  string // e.g. https://api.openai.com/v1
	EmbedBaseURL string // usually the same; split when a gateway lacks /embeddings
	APIKey       string
	EmbedAPIKey  string // set only when embeddings live on a different provider
	EmbedModel   string
	ChatModel    string

	embedHTTP *http.Client
	chatHTTP  *http.Client
}

// Config names what a client needs. A struct rather than positional arguments
// because the fields are all strings and two pairs of them are interchangeable at
// the type level — swapping EmbedModel and ChatModel, or the two keys, would compile
// and then quietly use the wrong one.
type Config struct {
	// ChatBaseURL is required. EmbedBaseURL is optional and defaults to it, which is
	// the common case: one provider serving both endpoints.
	ChatBaseURL, EmbedBaseURL string
	// APIKey authenticates both endpoints unless EmbedAPIKey is set. That matters when
	// the two base URLs are different providers: a gateway's key is not valid at
	// OpenAI, so splitting the URL without splitting the key just fails auth.
	APIKey, EmbedAPIKey   string
	EmbedModel, ChatModel string
}

// New builds a client from cfg, filling the embedding side from the chat side wherever
// cfg leaves it empty — one provider serving both endpoints is the common case, and two
// places to configure it is two places to get it wrong.
func New(cfg Config) *Client {
	chat := strings.TrimRight(cfg.ChatBaseURL, "/")
	embed := strings.TrimRight(cfg.EmbedBaseURL, "/")
	if embed == "" {
		embed = chat
	}
	key := cfg.EmbedAPIKey
	if key == "" {
		key = cfg.APIKey
	}
	return &Client{
		ChatBaseURL:  chat,
		EmbedBaseURL: embed,
		APIKey:       cfg.APIKey,
		EmbedAPIKey:  key,
		EmbedModel:   cfg.EmbedModel,
		ChatModel:    cfg.ChatModel,
		embedHTTP:    &http.Client{Timeout: embedTimeout},
		chatHTTP: &http.Client{
			Transport: &http.Transport{ResponseHeaderTimeout: chatHeaderTimeout},
		},
	}
}

// post sends to one endpoint with that endpoint's key — embeddings and chat can be
// different providers, so the key travels with the base URL, not with the client.
func (c *Client) post(ctx context.Context, hc *http.Client, base, key, path string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return hc.Do(req)
}

// Embed returns one vector per input, in the same order as the inputs.
func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	resp, err := c.post(ctx, c.embedHTTP, c.EmbedBaseURL, c.EmbedAPIKey, "/embeddings", map[string]any{
		"model": c.EmbedModel,
		"input": inputs,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, apiErr("embeddings", c.EmbedBaseURL, resp)
	}

	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("embeddings: decode: %w", err)
	}

	// The API contract orders by `index`, not by position in the array. Trusting
	// array order would silently attach embeddings to the wrong chunks — an index
	// that looks fine and retrieves nonsense.
	sort.SliceStable(out.Data, func(i, j int) bool { return out.Data[i].Index < out.Data[j].Index })

	if len(out.Data) != len(inputs) {
		return nil, fmt.Errorf("embeddings: asked for %d vectors, got %d", len(inputs), len(out.Data))
	}
	vecs := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		if len(d.Embedding) == 0 {
			return nil, fmt.Errorf("embeddings: vector %d is empty", i)
		}
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

// Msg is one chat message in the provider's wire format.
type Msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage is what the provider said one completion cost. Zero fields mean it
// reported nothing — never that nothing was spent, so a caller showing a number
// must check before believing it.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Reported is false when the provider sent no usage block, which is the case for
// several OpenAI-compatible gateways.
func (u Usage) Reported() bool { return u.PromptTokens > 0 || u.CompletionTokens > 0 }

// ChatStream streams the assistant reply token-by-token to onToken and returns what
// the provider said it cost.
//
// stream_options.include_usage asks OpenAI to append a final frame with the token
// counts; providers that don't know the field ignore it, and providers that don't
// answer it leave Usage zero. That is the whole reason Usage has Reported(): a UI
// must be able to tell "free" from "unmeasured".
func (c *Client) ChatStream(ctx context.Context, msgs []Msg, onToken func(string)) (Usage, error) {
	var usage Usage
	resp, err := c.post(ctx, c.chatHTTP, c.ChatBaseURL, c.APIKey, "/chat/completions", map[string]any{
		"model":          c.ChatModel,
		"messages":       msgs,
		"stream":         true,
		"temperature":    0.1,
		"stream_options": map[string]any{"include_usage": true},
	})
	if err != nil {
		return usage, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return usage, apiErr("chat", c.ChatBaseURL, resp)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, initialSSELineSize), maxSSELineBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue // comments, keep-alives and blank separators
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return usage, nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			// The usage frame arrives last and carries no choices, so it must be
			// read from the same loop rather than after it.
			Usage *Usage `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue // a partial frame isn't worth failing a whole answer over
		}
		// Some providers report a mid-stream failure as a data frame rather than
		// an HTTP status; swallowing it would look like a short answer.
		if chunk.Error != nil && chunk.Error.Message != "" {
			return usage, fmt.Errorf("chat: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				onToken(ch.Delta.Content)
			}
		}
	}
	return usage, sc.Err()
}

// apiErr names the base URL, because "404 on /embeddings" is nearly always a
// provider that doesn't have that endpoint rather than a broken request.
func apiErr(where, base string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	if resp.StatusCode == http.StatusNotFound && where == "embeddings" {
		return fmt.Errorf("embeddings: %s%s returned 404 — this provider has no embeddings endpoint. "+
			"Point EMBED_BASE_URL at one that does and keep chat where it is. Body: %s",
			base, "/embeddings", msg)
	}
	return fmt.Errorf("%s API %d at %s: %s", where, resp.StatusCode, base, msg)
}
