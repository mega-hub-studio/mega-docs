package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Supplementary retrieval: one public search, used only to explain what the documents lean
// on and never to answer in their place.
//
// Two switches, and neither is a heuristic. The instance needs SEARCH_API_KEY, and the reader
// has to ask — a checkbox in the settings drawer, off by default, travelling with the question
// the way the model pick does.
//
// One absolute remains: a question the corpus cannot answer *at all* gets the no-answer
// sentence and the BA route behind it, never a web result, however the toggle is set. That
// route ends in the documents being able to answer the question — external search answers the
// person asking; the QA loop answers everyone who asks next, and a fallback that quietly
// filled the gap would remove the only reason anybody files one.
//
// The provenance rules are the other half, and they are in the prompt: a web claim is [w1],
// [w2]… in its own numbering with its own list under the answer, so a sentence from a search
// API and a sentence from an approved specification cannot render identically.

// searchTimeout bounds the whole call. It is short on purpose: this is a supplement, so a
// slow provider must cost the answer its extra paragraph, never the answer.
const searchTimeout = 20 * time.Second

// webTopK is how many results are read. Three, the same order of magnitude as TOP_K's
// default, and a constant rather than a knob — nobody has asked to turn it, and a knob costs
// a documented section (rule 20).
const webTopK = 3

// webHit is one public result, already extracted. Nothing here scrapes HTML: the provider
// returns the page's text, which is the whole reason this is one POST and not a crawler.
type webHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// searchClient is one search provider over one POST. Tavily's shape, because it returns
// extracted content rather than a snippet — a snippet would need a fetch-and-strip step, and
// that is a crawler living inside a service with a write gate.
//
// A different provider is a SEARCH_BASE_URL away as long as it answers the same shape; when
// one does not, this is the one file that changes.
type searchClient struct {
	base string
	key  string
	http *http.Client
}

// newSearchClient returns nil when no key is configured, and nil is the off switch the whole
// feature reads. Same idiom as BA_PASS: an unset secret removes the surface rather than
// opening it, so forgetting to configure one is never how an instance ends up sending its
// questions to a third party.
func newSearchClient(base, key string) *searchClient {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	if base == "" {
		base = "https://api.tavily.com"
	}
	return &searchClient{
		base: strings.TrimSuffix(base, "/"),
		key:  key,
		http: &http.Client{Timeout: searchTimeout},
	}
}

// search returns what the public web says about a query, or nothing at all.
//
// Every failure is nothing rather than an error, and that is the point: this runs beside a
// grounded answer that is already complete without it. Failing the question because a search
// API was slow would trade the product for the supplement.
func (c *searchClient) search(ctx context.Context, query string) []webHit {
	if c == nil {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"query":       query,
		"max_results": webTopK,
	})
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/search", bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil
	}
	var out struct {
		Results []webHit `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	kept := out.Results[:0]
	for _, h := range out.Results {
		// A result with no text is a link, and a link the model cannot read is a citation it
		// would have to invent a claim for.
		if strings.TrimSpace(h.Content) != "" && strings.TrimSpace(h.URL) != "" {
			kept = append(kept, h)
		}
	}
	return kept
}

// supplement is what the public web adds to this answer, or nothing.
//
// One condition, because the reader already made the decision: they ticked the box for this
// question. `search` is nil unless the instance has a key, so the nil check is the other half
// and both are cheap.
//
// This used to be an automatic judgement — "the corpus said less than the model could read" —
// and it was deleted rather than tuned. Two versions of it were wrong in the same way, both
// caught by measuring the real instance rather than by review: the first compared retrieved
// characters against a fraction of the model's 128k window, which called almost every answer
// thin; the second compared what retrieval returned against the candidate pool, and on a
// 13-document corpus `maxPerDoc` caps that at 39 against a pool of 40, so it also fired on
// everything. Both numbers were an accident of two unrelated constants, and a switch nobody
// can see firing on every question is not a supplement, it is a default. The reader can answer
// "do I want outside help with this one" directly, so nothing here needs to guess it.
func (e *Engine) supplement(ctx context.Context, query string, want bool) (string, []Citation) {
	if !want || e.search == nil {
		return "", nil
	}
	return webContext(e.search.search(ctx, query))
}

// webContext renders the results as their own block with their own numbering, appended after
// the documents. Separate from CONTEXT and separately numbered because the prompt has to be
// able to talk about them as a different kind of thing — [n] is what the organisation
// approved, [wN] is what the internet said.
func webContext(hits []webHit) (string, []Citation) {
	if len(hits) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("\nWEB (public search results — not this organisation's documents):\n")
	cites := make([]Citation, len(hits))
	for i, h := range hits {
		n := i + 1
		fmt.Fprintf(&b, "[w%d] %s — %s\n%s\n\n", n, h.Title, h.URL, h.Content)
		cites[i] = Citation{N: n, Kind: webKind, Title: h.Title, URL: h.URL}
	}
	return b.String(), cites
}
