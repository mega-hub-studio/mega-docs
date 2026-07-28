# 2026-07-28 — Vite + SFCs for the app, and the docs pages stay exactly as they were

Asked for, after the trade-off was stated and reaffirmed: Vue with Vite, JavaScript only
(one maintainer, so no TypeScript). This is the migration, and the parts that were *not*
migrated are as important as the parts that were.

## Two front ends, on purpose

| | the app | the guide |
|---|---|---|
| source | `web/ui` — Vue 3.5 SFCs, JS | `web/*.html` — Go templates, both languages inline |
| build | Vite → `web/dist` (committed, embedded) | none, ever |
| deps | npm, bundled and content-hashed | jsDelivr, pinned by digest in `web/vendor.sha384` |
| why | it has state: streaming, tickets, imports, scope | it must be readable **before** the app exists and **while** it is down, and it lives on GitHub Pages |

The app was the only thing that ever wanted a framework. The guide keeps its zero-build
delivery, so `web/vendor.sha384` is now only its CDN pins — Vue, marked, DOMPurify and
mermaid are gone from it, and a test fails if anyone pins them there again. Two manifests,
neither knowing the other's versions.

## What moved

- `web/app/*.js` → `web/ui/src/lib/` (plumbing, unchanged except real imports) and
  `web/ui/src/composables/` (`const { ref } = Vue` → `import { ref } from "vue"`).
- The two in-DOM templates in `web/index.html` (570 lines, two `<template>` blocks) →
  eight SFCs: `App.vue` (wiring), `ChatTurn`, `EmptyScreen`, `ScopePicker`, `StatusLine`,
  `BaScreen`, `ImportPanel`, `TicketCard`, `CorpusTree`. The BA screen's importer moved
  into `ImportPanel.vue`, because a composable belongs to whoever renders its state.
- `use/toast.js` is gone: it existed only because the design system's CDN URL had to stay
  in one file. `toast` and `setMute` are imports now.
- mermaid: `<script src=…>` with an SRI attribute → `await import("mermaid")`, which Rollup
  gives its own chunk. Still lazy — the entry bundle is 190 kB (64 kB gzipped) and mermaid's
  chunks are only fetched when an answer actually contains a diagram. `globalThis.mermaid`
  is set for `<nes-mermaid>`, which prefers an instance it did not have to fetch.
- `web/index.html` (a Go template) → `web/ui/index.html` (a static Vite entry). Nothing is
  templated into the page any more: `SITE_URL` reaches the header through `/api/health`,
  with everything else the instance knows about itself.
- `ASSET_BASE` is **deleted**. There is no CDN to switch away from and nothing to vendor for
  the app. `internal/config` no longer reads it — and the spec check immediately failed
  until its row came out of the Deploy page's settings table, which is the system working.
- Routes: `GET /app/…` and `GET /vendor/…` are gone; `GET /assets/…` is served immutable,
  which is only safe because every filename carries a content hash. `etagFS` had no caller
  left and went with them.

## Why the build output is committed

Because `go build`, `go install` and `git pull && make build` on the deploy host must keep
working with no Node — that is CLAUDE.md rule 14, and it survived the migration by changing
shape rather than being dropped. Node is a contributor's tool now: `make ui`, `make ui-dev`,
`make lint-js`, each of which says so when it is absent.

A committed build artefact has exactly one failure mode: going stale. So it is checked
twice.

- The build stamps `web/dist/build.json` with a sha256 of everything it was built from
  (`src/**`, `index.html`, `vite.config.js`, `package-lock.json`).
  `TestBuiltUIMatchesItsSources` recomputes that hash in Go and fails with "run `make ui`".
  Same trick as the committed diagrams, whose SVG carries its `.mmd` hash.
- CI goes further: `npm ci && make ui`, then `git diff --exit-code web/dist`. A bundle
  cannot carry anything its sources do not produce.

Source maps are off. With them the tree was 16 MB, 12 of it generated files rewritten on
every build; debugging happens in `make ui-dev`, which has the real sources.

## The layer rules survived, and one improved

`web/frontend_test.go` still enforces four, retargeted at the new tree: plumbing imports no
Vue, a composable imports no composable, a component's `<script setup>` holds no branch,
and the shell does not reach into `lib/` (with `viewport.js` named as the one exception,
since it binds the dock element the shell owns).

The fifth — "everything a template binds exists behind it" — is **ESLint now**, and
`make lint-js` is inside `make check`. The old Go test approximated it with a regex over
module text; `vue/no-undef-properties` reads a real parse of a real component. The trigger
for promoting @antfu/eslint-config was written down when it was measured and rejected: *the
day TypeScript or `.vue` files land here*. They landed.

It earned the place immediately. Its first run found a **real** break: `answer.js` still
used `marked` and `DOMPurify` as globals, which no longer exist — every answer would have
rendered as an error the moment it was deployed. Also a duplicate `8bit-nes` import and two
imports left unused by the BaScreen split. Everything else it reported was formatting; those
rules are off, each with the reason next to it, exactly like `.golangci.yml`.

## Verified, not assumed

The built binary was run against a fake OpenAI-compatible provider with a two-document
corpus and driven in Chromium at 390px: mount, health dot, corpus counts, starters, ask,
streamed answer, `[1]` citation linked to its source row, **the mermaid diagram rendered
from the bundled chunk**, "TAP TO ZOOM", the same question asked twice → `CACHED` and
`cached · free` in the status line, the scope tree → `BOOKING` badge and the picker
closing itself, BA mode → stats, the prompt disappearing, unlock with `BA_PASS`, the import
panel with its folder datalist. No console errors, no failed requests, no 4xx.

`make check` is green with golangci-lint 2.12.2 (0 findings) and ESLint (0 findings).

One trap found on the way, now in CLAUDE.md: `go ./...` walks into `web/ui/node_modules`,
where one npm dependency ships a Go package — the linter reported seven findings from
somebody else's code. The Go tool has no directory ignore, so the Makefile spells the
packages out (`PKGS := ./cmd/... ./internal/... ./web`).

## Deploying this

`git pull && make build && sudo systemctl restart knowledge`. **No Node on the host**, no
schema change, no re-ingest. The startup line now names the UI build:

```
mega-docs on http://127.0.0.1:8080 (ui: vue 3.5.40 · 8bit-nes 0.7.2 · build 4efbc3b0, auth: off, writes: BA into corpus)
```

If a browser was left open on the old app, one reload is enough: `index.html` is
revalidated on every request and the asset names it points at all changed.
