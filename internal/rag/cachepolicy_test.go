// Package rag's internal tests. In-package on purpose: the cache policy they pin
// (isMiss, and the prompt carrying the sentinel) is unexported, and testing it through
// the public API would test the provider stub instead.
package rag

import "testing"

// What may be cached is a correctness question, not a performance one: caching a
// miss makes a gap permanent, and refusing to cache a partial answer throws away
// the most expensive completions the engine produces. The rule is therefore tested
// against the shapes a model actually returns, not the ones the prompt asks for.
func TestOnlyAWholeMissSkipsTheCache(t *testing.T) {
	for _, c := range []struct {
		name  string
		reply string
		miss  bool
	}{
		{"the sentinel alone", NoAnswer, true},
		{"the sentinel with whitespace a stream leaves behind", "\n" + NoAnswer + "  \n", true},
		{"a partial answer that names what is missing", "Folders are kebab-case [1].\n\nLeave policy: " + NoAnswer, false},
		{"an ordinary grounded answer", "Hybrid search fuses vectors and BM25 [1].", false},
		{"empty", "", false}, // caller checks length; this must not be mistaken for a miss
	} {
		if got := isMiss(c.reply); got != c.miss {
			t.Errorf("%s: isMiss = %v, want %v", c.name, got, c.miss)
		}
	}
}

// The prompt embeds the sentinel, so a future edit that reworded one without the
// other would leave the engine unable to recognise its own no-answer.
func TestThePromptCarriesTheExactSentinel(t *testing.T) {
	if !contains(systemPrompt, NoAnswer) {
		t.Fatal("systemPrompt no longer quotes NoAnswer verbatim — the engine cannot recognise a miss it asked for")
	}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
