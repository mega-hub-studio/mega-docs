package rag

import (
	"context"
	"slices"
	"strings"

	"knowledge-engine/internal/ai"
)

// Turn is one exchange already on screen. The client sends the tail of the thread with
// each question, because the thread lives there — `useConversation` persists it and the
// server keeps no session.
//
// The JSON tags are here rather than on a copy in internal/server: this is the wire shape
// of a turn, and two spellings of it would be two things to keep in step.
type Turn struct {
	Q string `json:"q"`
	A string `json:"a"`
}

// threadShare is how much of a model's context window the conversation may take.
//
// The other two thirds are not spare: the retrieved sections are the reason an answer is
// grounded, and the completion has to fit after them. A thread that crowds either one out
// buys memory by making the answer worse, which is the trade nobody asks for — an assistant
// that recalls message four and cites nothing is not remembering, it is guessing with
// context.
const threadShare = 0.35

// perToken is the crudest useful estimate of a token in characters, and it is deliberate.
// A real tokenizer is a dependency and a per-model table for a number that only decides
// *where to cut a list*; the provider reports the true count in its usage frame moments
// later, and that is what the status line prints. Four is low for prose and high for dense
// paths, so the budget errs toward keeping fewer turns — the failure that costs nothing.
const perToken = 4

// replay turns a thread into the messages a model can read, newest first and only as much
// of it as the model can hold.
//
// A turn with no answer is dropped rather than half-sent: a question without its answer
// tells the model that question went unanswered, which is not what happened — it is a
// turn that errored, was stopped, or is the one being asked right now.
//
// `window` is the picked model's context window, and 0 means the operator never said. Then
// nothing is trimmed: refusing to remember because a number is missing would make a working
// thread depend on an optional display knob, and the client already caps what it sends.
//
// Returns the messages and how many turns arrived, so the caller can report "3 of 8" — memory
// you cannot see is memory you cannot trust, and a silent drop is exactly how an assistant
// appears to forget for no reason.
func replay(history []Turn, window int) (msgs []ai.Msg, kept, offered int) {
	whole := make([]Turn, 0, len(history))
	for _, t := range history {
		if strings.TrimSpace(t.Q) == "" || strings.TrimSpace(t.A) == "" {
			continue
		}
		whole = append(whole, t)
	}
	offered = len(whole)

	budget := len(whole) * maxTurnChars // no window given: keep everything the client sent
	if window > 0 {
		budget = int(float64(window)*threadShare) * perToken
	}
	// Newest first, because the turn a follow-up points at is the one just above it. The
	// slice is then flipped back, since a model reads a conversation forwards.
	first := len(whole)
	for i, t := range slices.Backward(whole) {
		cost := len(t.Q) + len(t.A)
		if budget-cost < 0 {
			break
		}
		budget -= cost
		first = i
	}
	kept = len(whole) - first

	msgs = make([]ai.Msg, 0, kept*2)
	for _, t := range whole[first:] {
		msgs = append(msgs,
			ai.Msg{Role: "user", Content: strings.TrimSpace(t.Q)},
			ai.Msg{Role: "assistant", Content: strings.TrimSpace(t.A)})
	}
	return msgs, kept, offered
}

// maxTurnChars is the per-turn allowance used only when no window is configured: it makes the
// no-window path "keep what arrived" without a second branch, since the client caps the tail
// it sends and the request body is capped again by the handler.
const maxTurnChars = 1 << 20

// rewritePrompt asks for one thing and forbids the ways a model gets it wrong: a
// preamble, a translation, and answering the question instead of restating it.
const rewritePrompt = `Rewrite the user's latest message as a question that stands on its own, using the conversation above only to resolve what it refers to.

RULES:
- Output the question and nothing else. No preamble, no quotes, no explanation, and never an answer.
- Resolve what the message points at: "it", "that step", "còn bước 2 thì sao?" become the thing they name.
- Keep the language the latest message was written in.
- Keep every identifier exactly as written: file paths, commands, config keys, error codes, field names.
- If the message already stands on its own, output it unchanged.`

// standalone is the query retrieval runs on, which is not always the question that was
// typed.
//
// "còn bước 2 thì sao?" embeds to nothing useful and gives BM25 no keyword to match, so
// without this step the *answer* would have the conversation while the *retrieval* had
// none of it — an assistant that forgets mid-sentence, which is worse than one that never
// remembered. One cheap completion is what makes memory work rather than merely appear to.
//
// A failure here is not a failed question. The original wording is a worse query, not an
// invalid one, so the error is dropped and retrieval proceeds on what the user typed —
// the same reason an unreadable cache signature means "do not cache", never "fail".
func (e *Engine) standalone(ctx context.Context, chat *ai.Client, question string, turns []ai.Msg) (string, ai.Usage) {
	if len(turns) == 0 {
		return question, ai.Usage{}
	}

	msgs := make([]ai.Msg, 0, len(turns)+2)
	msgs = append(msgs, ai.Msg{Role: "system", Content: rewritePrompt})
	msgs = append(msgs, turns...)
	msgs = append(msgs, ai.Msg{Role: "user", Content: question})

	var b strings.Builder
	usage, err := chat.ChatStream(ctx, msgs, func(tok string) { b.WriteString(tok) })
	rewritten := strings.TrimSpace(b.String())
	if err != nil || rewritten == "" {
		return question, usage
	}
	return rewritten, usage
}

// window is the context window of the model about to answer, or 0 when the operator never
// said. The engine holds the list because it is the layer that decides what fits: the HTTP
// surface publishes the same numbers for display, but a trimming rule that lived up there
// would be a second opinion about the prompt.
func (e *Engine) window(model string) int {
	for _, m := range e.models {
		if m.Name == model {
			return m.Window
		}
	}
	return 0
}
