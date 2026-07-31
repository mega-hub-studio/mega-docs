package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"knowledge-engine/internal/db"
)

// Supplementary retrieval: one public search, used only to explain what the documents lean
// on and never to answer in their place.
//
// It is off unless SEARCH_API_KEY is set, and even then it fires under two conditions, both
// in `thin` below. The first is absolute: a question the corpus cannot answer at all gets the
// no-answer sentence and the BA route behind it, never a web result. That route ends in the
// documents being able to answer the question — external search answers the person asking;
// the QA loop answers everyone who asks next, and a fallback that quietly filled the gap
// would remove the only reason anybody files one.
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

// thinShare is the fraction of the retrieval budget below which the corpus counts as having
// said little about the question.
//
// It is not a tuned threshold, it is a statement: the model could have read twice this much
// and the documents did not have it. That statement is only measurable because the budget
// exists — without a window the engine has no idea how much the model could have read, and
// this returns false rather than guessing.
const thinShare = 0.5

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

// thin reports whether the documents said much less about this question than the model could
// have read — the one condition under which a supplement is worth buying.
//
// Two gates, and the first is the one that protects the QA loop: no hits at all is a gap, not
// a thin answer, and a gap belongs to a BA. The second needs a budget to be a fraction of, so
// an instance that never declared a window never searches: it has no way to tell "the corpus
// was quiet" from "the model is small", and guessing between those two would send internal
// questions to a third party on the strength of a number nobody set.
func thin(hits int, chars, budget int) bool {
	if hits == 0 || budget <= 0 {
		return false
	}
	return float64(chars) < float64(budget)*thinShare
}

// supplement is the whole decision, in one call, so Answer reads as intent and keeps its
// branch count: what the public web adds to this answer, or nothing.
//
// Nothing is the common case and every one of its reasons is deliberate — no key configured,
// no window declared, a corpus that answered the question properly, or a search that failed.
func (e *Engine) supplement(ctx context.Context, query, model string, hits []db.Hit) (string, []Citation) {
	chars := 0
	for _, h := range hits {
		chars += len(h.Content)
	}
	if !thin(len(hits), chars, e.contextBudget(model)) {
		return "", nil
	}
	return webContext(e.search.search(ctx, query))
}

// searchSig is what the cache signature carries about this feature.
//
// Whether an instance can reach the web is startup config — the same tier as the prompt and
// the budget — so it belongs in the signature, and turning the key on invalidates every row
// at once, which is right: those answers were produced under a rule the instance no longer
// has. Nothing per-request joins the cache key, because nothing about this is per-request:
// the trigger is derived from the corpus, the question and the budget, all of which the key
// and the signature already carry between them.
//
// The limitation this leaves is worth stating rather than hiding: a cached answer that used
// the web does not re-fetch it. Regenerate is how a reader asks for today's version, exactly
// as it is for a document that changed between re-indexes.
func (e *Engine) searchSig() string {
	if e.search == nil {
		return ""
	}
	return "w"
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
