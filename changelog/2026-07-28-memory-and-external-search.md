# 2026-07-28 — Design: conversation memory, and external search

Three asks, in increasing order of how much they can break: talk like a real assistant,
remember the conversation, and search outside the documents when they fall short. The first
is largely done. The second is a contained change with two traps. The third contradicts the
product's central promise unless it is built with visible provenance.

## 1. "Like a real assistant" — mostly shipped

A greeting is answered, an unclear question is asked back, the assistant's own model is
named rather than refused, and an ambiguous question is met with "which of these two did you
mean?" instead of a refusal. All of it before retrieval, so none of it costs a completion.

What is still missing from that impression is exactly item 2: an assistant that forgets the
previous sentence does not read as a person.

## 2. Conversation memory — contained, but two things must move with it

The turns already exist client-side: `useConversation` persists them through `session.js`.
What is missing is that `POST /api/chat` sends only `{question, scope}`, and `rag.Ask` has no
field for history. So this is a change to the seam, not a new subsystem.

**Trap A — the cache will serve a wrong answer.** The key is `q_norm`: the normalised
question plus the scope. "còn bước 2 thì sao?" is not self-contained, so cached under its own
text it would be returned to a *different* conversation asking the same four words about
something else. Two ways out, and the cheap one is right for now:

  - Do not cache a turn that used history. Costs a completion per follow-up; correct by
    construction, and one line.
  - Or fold a hash of the history into the key. Cheaper per answer, and it makes the
    cache-hit rate on follow-ups approximately zero anyway, because two conversations rarely
    share a prefix. Not worth the complexity until measured.

**Trap B — retrieval cannot embed a pronoun.** "còn bước 2?" embeds to nothing useful and
BM25 finds no keyword, so the *answer* would have context while the *retrieval* had none —
which reads as the assistant forgetting mid-sentence. The standard fix is a rewrite step:
turn the follow-up into a standalone question using the history, then retrieve on that. It
costs one cheap completion per follow-up, and it is what makes memory actually work rather
than merely appear to.

Order: rewrite for retrieval, history for the answer, no caching of either. Acceptance: ask
"how do I cancel a paid booking", then "and if it was refunded already?" — the second answer
cites a section the second question's own words could never have retrieved.

## 3. External search — the one that needs a decision, not a sprint

The system prompt's first rule is "Answer ONLY from the CONTEXT. Never use outside
knowledge", and every claim carries a citation to a file a person approved. Web results
break that in a way that is invisible on screen: a sentence from a search API and a sentence
from a booking specification render identically. Three things have to be true before it ships,
and none of them is the search call itself.

**Provenance must be visible, not implied.** An external claim needs its own marker and its
own list — not `[n]` pointing at a document. The library already has what this needs: a
`.callout` for the block and a `.badge` for the row, so a reader can see at a glance which
half of an answer their organisation actually vouched for.

**The cache signature must carry it.** Invariant 3 covers corpus, chat model and prompt hash.
It does not cover "this answer used the web". Without that, one question answered with search
on is served later to someone who has it off — a document-sourced answer that never was.

**It sends internal questions to a third party.** "quy trình chốt giá cho khách VIP" is
business intelligence, and a search API is an external party receiving it. That is a
workspace-level decision (and, in the SaaS phase, a per-tenant one with a contract behind
it), not a default. Off unless switched on, and visible when it is on.

My recommendation is to build 2 first and completely. Memory makes the assistant feel human,
costs one rewrite per follow-up, and cannot invent a fact about the organisation. External
search buys reach at the price of the one thing this product sells — that every claim traces
to a document somebody approved — so it should be a deliberate, labelled, off-by-default
feature rather than a fallback that quietly fills gaps.

The BA loop is also the honest alternative to it: a question the documents cannot answer
already has a route that *ends in the documents being able to answer it*. External search
answers the person; the BA loop answers everyone who asks next.
