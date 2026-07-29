# 2026-07-29 — The settings panel lost its sentences, and the pin moved to 0.14.0

## The panel

Reported from a phone, in Vietnamese: six facts rendered as three paragraphs. "Rides on every
question. This instance refuses any other." · "Chưa cấu hình giá" · "Bắt đầu từ câu hỏi đầu
tiên của bạn" — each wrapping to three lines beside a mono uppercase label as wide as the value
column, so finding the one control anybody opens this panel for meant reading all of it.

Every row is now `[icon] [value]` on one grid: `cpu` · `layers` · `bolt` · `chat` · `globe` ·
`volume`/`mute` · `sliders`, each with its sentence moved into `title`. Measured at 390×844: one
x for every icon (417), one x for every value (449), and the body went from 1200px of prose to
752px that fits without scrolling.

Three details that are the difference between "fewer words" and "less to read":

- **The unit is a glyph.** `128k`, not `128,000` and not a label saying tokens; `$2.5 / $10`,
  where the `$` is what tells the price row from the window row at a glance. Neither can wrap.
- **Unknown is `—`.** Not a sentence about being unknown. The old panel spent two of its six
  rows explaining that nothing was configured.
- **The icon carries state where it can.** The sound row shows `mute` or `volume`, so it says
  which way the switch is without a word beside it.

`.meter` is new in 0.14.0 — a cell bar — and it is what turned "3 of 8 turns survived the
window" into three lit boxes of eight. The figure stays beside it for anyone who wants it.

The one 4px thing, because it is the kind that reads as *slightly wrong* without being
nameable: a checkbox keeps a UA margin under `appearance: none`, so the switch was the one value
starting 4px right of the other five. `.set-row > .switch { margin: 0 }`.

## The bump: 0.13.0 → 0.14.0

AGENTS.md requires re-measuring every local override on a bump rather than trusting a
changelog, and this is that measurement: **all eight are still needed.** `.palette-list` is
still capped at `min(50vh, 340px)`; `.prose a` still outscores `.cite`; `.drawer` still leaves
`inset-inline-start` alone, so it still anchors left on a `<dialog>`; `.source-title` still
underlines on hover; `.result` is still a control; `.statusline` is still `inline-size: 100%`
with `.sl-end` pushed; `.palette-empty` is still centred; `::selection` is still a solid fill.

Two facts worth keeping from the fetch: only **`all.min.css` changed digest** — `elements.min.js`
and all three fonts came back byte-identical to 0.13.0 — and the icon set is unchanged, so
`history` is still absent and the memory row still uses `chat`.

Both manifests moved together (rule 7 keeps them independent, not out of step):
`web/ui/package.json` for the bundled app, `web/vendor.sha384` for the guide's CDN pins, with
`make vendor` verifying the five files it fetches. `TestAgentNotesPinMatchesTheManifest` is what
made AGENTS.md's quoted version part of the same change rather than a follow-up.

## Still open, for upstream, now confirmed against 0.14.0

1. `.prose a` should be scoped away from `.cite` (or `.cite` moved to a later layer) — the same
   collision reaches `.chip`, `.badge` and `.kbd`.
2. `.source-title`'s hover underline belongs on `a.source-title`.
3. `.drawer` should qualify its own anchoring: on a `<dialog>` the UA has already set
   `inset-inline: 0`, so `inset-inline-end: 0` plus the recipe's `margin: 0` is over-constrained
   and the *start* wins. `.drawer.start` renders identically to `.drawer`, which is the tell.

## The constants under the engine became knobs — the two that earned it

"Put the system's constants into the settings panel" is a good instinct with a hard boundary in
the middle, and the boundary is **whether changing the number makes the stored index disagree
with the code**:

| constant | verdict |
|---|---|
| `TOP_K` (already a knob) | safe at any time — but it decides how many sections an answer was built from, so it belongs in the **cache signature**, and it was not in it |
| `threadShare` → `THREAD_SHARE` | safe: it only shapes follow-ups, and a follow-up is never cached in either direction |
| `keep` → `CACHE_KEEP` | safe: a row count, not answer content |
| `maxChars` · `minChars` · `overlap` (`chunk.go`) | **not settings.** Changing them changes chunk boundaries, so every stored chunk and vector came from a different chunker — that is a full re-ingest, an *operation* with a bill |
| `EMBED_MODEL` · `EMBED_DIM` | same, harder: `db.Open(path, dim)` creates the sqlite-vec table at that width, and vectors from two models are not comparable |
| `perToken` · `maxTurnChars` · `MaxDepth` · `crumbSep` · the prompts | nobody turns a heuristic denominator or a separator, and the prompt is already in the signature by hash. Rule 20: a knob nobody turns is overhead with a documented section attached |

So: two new knobs, and one latent cache bug closed. `TestChangingTopKInvalidatesTheCache` is the
new assertion, and it is worth reading as a warning rather than a feature — the cache was filled
at six sections and read at twelve, serving the narrower answer under the wider setting with
nothing on screen saying which one arrived. `TOP_K` is in the signature and not the key, because
unlike a scope or a model there is no *other* `TOP_K` whose rows are still worth keeping.

All three numbers are on `/api/health` — none of them is a secret — so the panel shows what this
instance is actually tuned like without a password: `grid 6` · `chat 35%` · `database 200`. Nine
rows now, still one x for every icon and one for every value, still no sentences.

## Deliberately not built: writing settings from the browser

The next step is a `settings` table, a `PUT /api/settings` behind `ADMIN_PASS`, and turning
those rows into inputs. It is *unblocked* — none of these three is a secret, so nothing sensitive
would ride along to wherever `scripts/backup.sh` publishes the database nightly — and it is not
built yet because the honest cost/benefit for **two numbers** is a whole write surface,
validation at the boundary, a migration, precedence logic (`db > env > default`) and a fourth
provenance value on the Admin screen. It earns its keep when the third or fourth runtime knob
arrives, and the knobs have to exist before they can be edited, which is what landed here.

An API key must not go in that table whatever else does: the database is copied off this machine
every night by design, so a key stored in it is a key replicated to every backup, forever. If
provider switching from the UI is wanted, the shape that keeps the secret at home is a providers
table holding the *name of the environment variable* that carries each key, never the key.

## Recorded, not built: `AUTH_PASS` is a placeholder for accounts

The next direction for it is login, per-account quota and a role (`admin`), and it is written
here so the shape of what just landed does not have to be re-derived: these three knobs are
**instance-wide** and stay that way. A per-account quota is a table keyed by account with its own
counters, not a rework of the engine's tuning — the engine asks "how many sections, how much
window, how many cached rows", and none of those questions has an account in it. What *will* need
the account is the write gate (`BA_PASS`/`ADMIN_PASS` become roles) and the cost figures the
status line already computes per answer.
