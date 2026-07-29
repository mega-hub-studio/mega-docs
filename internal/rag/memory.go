package rag

import (
	"context"
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

// replay turns a thread into the messages a model can read.
//
// A turn with no answer is dropped rather than half-sent: a question without its answer
// tells the model that question went unanswered, which is not what happened — it is a
// turn that errored, was stopped, or is the one being asked right now.
func replay(history []Turn) []ai.Msg {
	msgs := make([]ai.Msg, 0, len(history)*2)
	for _, t := range history {
		q, a := strings.TrimSpace(t.Q), strings.TrimSpace(t.A)
		if q == "" || a == "" {
			continue
		}
		msgs = append(msgs, ai.Msg{Role: "user", Content: q}, ai.Msg{Role: "assistant", Content: a})
	}
	return msgs
}

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
func (e *Engine) standalone(ctx context.Context, question string, turns []ai.Msg) (string, ai.Usage) {
	if len(turns) == 0 {
		return question, ai.Usage{}
	}

	msgs := make([]ai.Msg, 0, len(turns)+2)
	msgs = append(msgs, ai.Msg{Role: "system", Content: rewritePrompt})
	msgs = append(msgs, turns...)
	msgs = append(msgs, ai.Msg{Role: "user", Content: question})

	var b strings.Builder
	usage, err := e.ai.ChatStream(ctx, msgs, func(tok string) { b.WriteString(tok) })
	rewritten := strings.TrimSpace(b.String())
	if err != nil || rewritten == "" {
		return question, usage
	}
	return rewritten, usage
}
