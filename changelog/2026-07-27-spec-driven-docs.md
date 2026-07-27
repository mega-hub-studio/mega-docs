# 2026-07-27 — The pages are the spec, and the build enforces it

Asked for the docs to become the source of truth going forward: one artefact that is both
the guide a Vietnamese dev reads and the specification an agent harness implements from.
The half that was missing is not prose — it is the **join**. A feature is described on a
page, reached through a route, configured by a variable and pinned by a test, in four
places with nothing connecting them, so prose drifts from code silently.

## The annotation

A `<section>` that maps to code now declares the mapping in its own markup:

```html
<section id="scope" data-feature="scope"
         data-api="POST /api/chat" data-env="TOP_K"
         data-test="TestScopedSearchRanksWithinTheScope">
```

`web/spec.go` parses those out of the rendered pages into **`spec.json`**, published next
to them (`cmd/rendocs` writes it; `llms.txt` links it; the Pages workflow validates it as
JSON and checks every URL points at this site). 23 features today, each with its section
URL, its summary in the author's own words, both language titles, its routes, its knobs and
its tests.

Nothing about the prose is copied into it. An agent reads the page for that.

## Why this is a spec and not a summary

`web/spec_test.go` checks the join **in both directions**, against the source rather than
against another document:

| direction | check | what it catches |
|---|---|---|
| spec → code | `TestEverySpecNameExistsInTheCode` | a test name that was renamed, a route in prose that no `mux` line serves, a documented variable nothing reads |
| code → spec | `TestEveryRouteAndKnobIsSpecified` | **a new `/api/` route or config variable that no section documents** |
| shape | `TestSpecJSONIsGeneratedFromThePages` | a missing anchor, a duplicate `data-feature`, a missing VI title, a section with no summary, a page contributing nothing |

The second row is the one with teeth: an endpoint cannot go green until it is documented.
That makes the working order **spec first** — write the section, declare the join, watch
`make check` go red, then implement — which is the only version of "the docs are the source
of truth" that survives a deadline. The five steps are on the Dev page under *These pages
are the spec* (`dev.html#spec`), with a diagram, and the same order is stated in
`spec.json` itself for whoever reads that first.

All four checks were **mutation-tested**: renaming `TestScopeTreatsWildcardsAsCharacters`,
adding a `GET /api/undocumented`, adding a `NEW_KNOB`, and pointing two sections at one
`data-feature`. Each fails in the right test, naming the section and the missing name.

The extractors read `internal/server` and `internal/config` **as source**, not as imports:
`web` must not depend on the packages it serves, and "which routes are registered" is a
lexical fact. `TestTheExtractorFindsWhatItClaims` pins them to known routes and knobs so a
silently-empty match set cannot make every other check pass.

## What it turned up immediately

- **Five environment variables were documented nowhere**: `PORT`, `DB_PATH`, `SITE_URL`,
  `CHAT_MODEL`, `AUTH_USER` — plus `AI_BASE_URL`, `AI_API_KEY` and `EMBED_MODEL`, which
  appeared in prose but had no row. The settings table on the Deploy page now has all
  twenty, and the coverage check keeps it that way.
- **Three sections had no opening sentence**, jumping straight into a table
  (`ba.html#stuck`, `deploy.html#access`, `deploy.html#settings`). Each has a lede now — the
  spec wanted one line saying what the section is for, which every other section already
  had.
- **A diagram id collided with a section id.** The new flow diagram was `spec.mmd`, so the
  inlined `<svg id="spec">` landed inside `<section id="spec">` — and mermaid scopes its own
  `<style>` to the svg id, so its font and colours applied to the whole section. Renamed to
  `specflow.mmd`, and `TestNoDiagramIdCollidesWithASectionId` now compares the two id
  spaces (svg roots vs sections only — mermaid reuses ids inside its own output, which is
  its business).

## Two more rules, both with enforcers

`CLAUDE.md` gains rules **15** (the join, both directions) and **16** (`spec.json` and
`llms.txt` are generated, never hand-written). `AGENTS.md` now opens with `spec.json`
rather than `llms.txt`, and says the change flow is spec-first. `README.md` says the same
for a human, and its "where do I add…?" table gains a row for *a feature*.

## Deploying this

Docs and one Go file — no schema change, no re-ingest. `git pull && make build &&
systemctl restart knowledge`. The published `spec.json` appears on the next push to `main`.
