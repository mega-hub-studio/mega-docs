# 2026-07-29 — A citation marker was rendering as an ordinary link, and two upstream requests

The `[n]` markers in an answer are the library's `.cite` recipe: a superscript digit in a slot
background with a cyan edge, `text-decoration: none`. On screen they were green with a 2px
green underline struck through the digit.

## The cause is a collision inside 8bit-nes 0.8.0, not this app

Both rules are in the library's own `components` layer, so the layer cannot separate them and
specificity decides:

| rule | specificity | says |
|---|---|---|
| `.prose a` | (0,1,1) | `color: var(--accent)`, `underline`, thickness `--bw-2` |
| `.cite` | (0,1,0) | `color: var(--cyan)`, `text-decoration: none` |

A marker lives inside `.prose`, so `.prose a` won every time and `.cite`'s two most visible
declarations never applied. Measured on the built bundle rather than reasoned about:
`getComputedStyle` on an `a.cite` inside `.prose` returned `rgb(86, 211, 100)` — that is
`--good`, which `.prose` sets `--accent` to — with `underline` at `2px`. The same for
`:hover`: `.prose a:hover` (0,2,1) recolours to `--ink` and outscores a bare `.cite:hover`,
whose only job is the background lift.

Why it mattered beyond neatness: the chip **is** the affordance — superscript, own edge, own
background — so the underline is a second affordance drawn through the digit, in the colour
the same paragraph uses for ordinary links. The one thing on the line that is not a link to
the web was the thing most decorated like one.

## What landed

Three rules in `web/ui/src/styles.css`, all scoped, no component reimplemented:

- `.prose a.cite` (and its `:hover`) back to cyan with no decoration. This is
  **local override #2** in the sense `AGENTS.md` counts them, and its delete condition is a
  release that scopes the prose-link rule away from `.cite`.
- `.source .source-title:hover` un-underlined. The recipe expects an `<a>`; this app renders
  the source row as three `<span>`s, because nothing in the ASK screen can open a document —
  so the underline promised a link that does not exist, on every row of every answer. Not an
  upstream bug, so it has no delete condition beyond the row becoming clickable.
- `.source:target` gets a cyan outline. A marker click used to scroll and say nothing at all,
  and on a short answer the sources are already on screen — so the interaction was a scroll of
  zero pixels with no reply. An outline rather than a border or a background, so the row does
  not move as it lands.

Real markdown links inside an answer keep their underline. That is deliberate: colour alone
is not a link cue (WCAG 1.4.1), and a body link is exactly the case the underline is for.

## Two requests for upstream, both still open

Neither blocks anything here; both would delete a local rule:

1. Scope the prose-link rule — `.prose a:not(.cite)`, or move `.cite` to a later layer — so a
   component recipe is not defeated by a container recipe in the same layer. This is the
   pattern to look for on the next bump: `.cite`, `.chip`, `.badge` and `.kbd` are all
   single-class recipes that can appear inside `.prose`, and each loses the same argument.
2. `.source-title`'s hover underline belongs on `a.source-title`, not on the class, so a
   read-only source list does not advertise a link.

## Verifying this again after a version bump

`make check-ui` does not cover the app — it measures the published guide — so the check is a
measurement, not a look:

```js
getComputedStyle(document.querySelector('.prose a.cite')).textDecorationLine  // 'none'
getComputedStyle(document.querySelector('.prose a.cite')).color               // rgb(51, 224, 224)
```

Run it against `make ui-dev` with any answered question on screen. If the library has fixed
the collision, both hold with the local override deleted — which is when to delete it.
