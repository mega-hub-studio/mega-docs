package ai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"knowledge-engine/internal/ai"
)

// The usage frame arrives last, carries no choices, and is the only place a real
// cost can come from. A provider that sends none must leave Usage unreported rather
// than zero-but-believable — the status line prints a blank for one and a number for
// the other.
func TestChatStreamReadsTheUsageFrame(t *testing.T) {
	for _, c := range []struct {
		name     string
		frames   []string
		reported bool
		in, out  int
	}{
		{
			name: "provider reports usage",
			frames: []string{
				`{"choices":[{"delta":{"content":"hi"}}]}`,
				`{"choices":[],"usage":{"prompt_tokens":1234,"completion_tokens":56}}`,
			},
			reported: true, in: 1234, out: 56,
		},
		{
			name:     "provider reports none",
			frames:   []string{`{"choices":[{"delta":{"content":"hi"}}]}`},
			reported: false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, f := range c.frames {
					w.Write([]byte("data: " + f + "\n\n"))
				}
				w.Write([]byte("data: [DONE]\n\n"))
			}))
			defer srv.Close()

			cl := ai.New(ai.Config{ChatBaseURL: srv.URL, APIKey: "k", ChatModel: "m"})
			var got strings.Builder
			u, err := cl.ChatStream(context.Background(), []ai.Msg{{Role: "user", Content: "q"}},
				func(tok string) { got.WriteString(tok) })
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != "hi" {
				t.Fatalf("streamed %q, want %q", got.String(), "hi")
			}
			if u.Reported() != c.reported {
				t.Fatalf("Reported() = %v, want %v (%+v)", u.Reported(), c.reported, u)
			}
			if c.reported && (u.PromptTokens != c.in || u.CompletionTokens != c.out) {
				t.Fatalf("usage = %+v, want in=%d out=%d", u, c.in, c.out)
			}
		})
	}
}

// A usage frame must not be mistaken for content: it has an empty choices array,
// and a client that forwarded it would print JSON into the answer.
func TestTheUsageFrameStreamsNoTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":9}}` + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	cl := ai.New(ai.Config{ChatBaseURL: srv.URL, APIKey: "k", ChatModel: "m"})
	n := 0
	if _, err := cl.ChatStream(context.Background(), []ai.Msg{{Role: "user", Content: "q"}},
		func(string) { n++ }); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("onToken called %d times for a usage-only stream", n)
	}
}
