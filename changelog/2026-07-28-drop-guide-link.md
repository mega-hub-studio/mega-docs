# The app stopped linking out to the guide

Settled. `SITE_URL` is gone from the code, `.env.example`, the Deploy page's `data-env`,
and `/api/health`. Do not put it back — this file is the reason, so nothing else has to
carry it.

## Why

The guide is published from this repository to its own Pages domain on its own cadence.
An address for it held by a *running server* is a second home for a fact that already has
one — `cmd/rendocs -site`, where it belongs, because it is a property of a render and not
of a process. The app is the chat app; one product, one surface.

`DefaultSiteURL`'s doc comment used to claim it was also "the base of the absolute URLs in
`/llms.txt`". It was not, for the binary: `internal/server` serves no guide route at all
(`TestGuideRoutesAreNotServed` asserts `/docs`, `/dev`, `/deploy`, `/llms.txt` are 404), and
`cmd/rendocs` passes its own `-site` flag. That sentence was the only thing making the knob
look load-bearing.

## What went, in one pass

Removing the button alone would have left a knob nobody turns, so the whole chain went
together:

`App.vue`'s header `<a>` · `i18n.js`'s `guide` key (EN + VI) · `site` in `runtime.js` and in
all three shapes `chat.js#health()` returns · `server.Runtime.Site` and `"site"` in the
`/api/health` body · `cfg.SiteURL` in `cmd/server` · `Config.SiteURL`, `DefaultSiteURL` and
`env("SITE_URL", …)` · the `.env.example` key · `SITE_URL` in `deploy.html`'s `data-env` and
its two `<tr>` rows.

Rule 15 is what forced the order: delete the knob in `internal/config` without deleting the
Deploy page's `data-env` entry and `make check` goes red on
`TestEverySpecNameExistsInTheCode`. There is no way to do half of this.

## Breaking

`/api/health` no longer carries `site`. The body is now
`{"ok","writes","model","window","price_in","price_out"}`. Nothing in the repo read it;
an external probe that did will get `undefined`.

## Where the fact lives now

One line each, no prose duplicated:

- `CLAUDE.md` — the rule ("do not reintroduce a `SITE_URL`"), with the one-sentence reason.
- `README.md` — the reference: the binary does not serve the guide and does not link to it.
- `AGENTS.md` — the same clause, aimed at an agent.
- here — the decision and its history.

`internal/config/config.go` carries a pointer to this file and nothing else. A paragraph
there explaining an absent knob was a fifth copy, and the one with no symbol to attach to.
