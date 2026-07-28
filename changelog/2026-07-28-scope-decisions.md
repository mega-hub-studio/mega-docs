# 2026-07-28 — Two of the three vNext collisions are closed, by decision

`2026-07-28-vnext-collisions.md` left three items needing a decision rather than a commit.
Two are now decided, both toward *less*, and both are recorded here so the brief's own lines
stop reading as open work:

| collision | decision | what it costs |
|---|---|---|
| "Removed: Hybrid pipelines" | **BM25 stays.** Retrieval is vector KNN + BM25 fused with RRF, unchanged | nothing — the simplification the brief wanted was provider and import sprawl, and that is already gone |
| "Supported Files: PDF, DOCX" | **Out of scope. `.md` · `.markdown` · `.txt` only** | a BA with a PDF converts it first; the product never learns to parse one |

The third — inverting the source of truth — is unchanged and still blocked, on one thing
now rather than two: `internal/db/migrate.go` shipped, an off-box DB backup has not.

## Why BM25 stays

The FTS5 tokenizer is `unicode61 remove_diacritics 2`, chosen for Vietnamese, and BM25 is
the half that matches an error code, a config key or a rule id **verbatim**. The BA guide's
own advice is that an error code beats a paraphrase, so removing the retriever that makes
that true would contradict the page while it is still published.

Worth keeping the two meanings of "one pipeline" apart, because the brief's phrasing invites
the wrong one:

- *one path a question travels* — already true, there is exactly one `Answer`
- *one candidate generator* — what dropping BM25 would mean, and it is a measurable accuracy
  loss dressed as a cleanup

Invariant 4 (a scope filters **both** retrievers before they rank) therefore stays, along
with `TestScopedSearchRanksWithinTheScope`.

## Why PDF/DOCX is out of scope rather than pending

The three ways to accept a PDF were: a Go parser in the binary, an external tool invoked at
upload, or neither. Neither wins, and the reason is the same reason `TextExts` was narrow in
the first place: it keeps "the documents are messy" a **one-time cleaning step outside the
product** instead of a runtime failure mode inside it.

What the other two would have cost:

- **A Go parser** puts a binary-format parser's CVE surface inside a service that has a
  write gate and no accounts. One binary is a deployment property worth protecting, not a
  reason to swallow a dependency that parses hostile input.
- **An external tool at upload** adds a process this binary has to find, sandbox and
  version, and turns "convert your file" into "the server converts your file, sometimes" —
  a failure that appears at upload time, per file, to a BA who cannot fix it.

So the answer stays `markitdown spec.pdf > spec.md`, and the thing that makes it good DX is
that the refusal already says so: `internal/rag/upload.go` names the command in the error
rather than reporting an unsupported type. **A rejection that tells you the fix is not a
missing feature.**

This is not "PDF later". If it is revisited, it is revisited as a decision with a new reason,
not as a backlog item somebody finds.
