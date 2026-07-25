// Package ai is a minimal OpenAI-compatible client (embeddings + streaming chat).
// Works with OpenAI, Azure, Groq, Together, OpenRouter, local Ollama/LM Studio, etc.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Client struct {
	BaseURL    string // e.g. https://api.openai.com/v1
	APIKey     string
	EmbedModel string
	ChatModel  string
	http       *http.Client
}

func New(baseURL, apiKey, embedModel, chatModel string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		EmbedModel: embedModel,
		ChatModel:  chatModel,
		http:       &http.Client{},
	}
}

func (c *Client) post(ctx context.Context, path string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	return c.http.Do(req)
}

// Embed returns one vector per input string.
func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	resp, err := c.post(ctx, "/embeddings", map[string]any{
		"model": c.EmbedModel,
		"input": inputs,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, apiErr("embeddings", resp)
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

type Msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatStream streams the assistant reply token-by-token to onToken.
func (c *Client) ChatStream(ctx context.Context, msgs []Msg, onToken func(string)) error {
	resp, err := c.post(ctx, "/chat/completions", map[string]any{
		"model":       c.ChatModel,
		"messages":    msgs,
		"stream":      true,
		"temperature": 0.1,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return apiErr("chat", resp)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				onToken(ch.Delta.Content)
			}
		}
	}
	return sc.Err()
}

func apiErr(where string, resp *http.Response) error {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return fmt.Errorf("%s API %d: %s", where, resp.StatusCode, strings.TrimSpace(buf.String()))
}
