# 2026-07-28 — "Running it on your machine", on the page instead of in a chat reply

The two-terminal dev flow had been worked out and verified, and then existed only as a
message. It is a section now — `dev.html#local`, feature id `local-dev` — with the
commands, the diagram, and the four ways it looks broken when it is not.

## The diagram earns its place

`web/devloop.mmd` → `devloop.svg` (`make diagram`, committed, mermaid never ships). It
draws the thing the prose kept having to repeat: **one edit, two loops**.

```
you edit a .vue ─┬─→ vite :5173 ─┬─→ browser :5173      (hot swap, no reload)
                 │               └─→ make server :8080 ─→ sqlite + provider   (/api proxied)
                 └─→ make ui ─→ web/dist, committed ─→ make build ─→ binary serves :8080
```

Which answers, in one look, the question that generated the whole exchange: why an SFC edit
changes :5173 immediately and :8080 not at all.

Everything in the section was run before it was written: `make ui`, both terminals, and the
HMR claim measured (the DOM swapped with **zero full reloads**, and the conversation already
on screen survived). The "what it is" table is the four failures that actually happened
during that verification, including the honest one — an SFC edit doing nothing on :8080 is
the design, not a bug.

## A screenshot found what the check did not measure

Taking the picture for this entry showed the *previous* section's `<dl class="datalist">`
squeezed to a fourteen-character value column on a phone — the same failure as the
three-column tables, in a recipe last week's check never looked at.

The cause is worth writing down: `.datalist` is a two-column grid, and the design system
*does* stack it — but inside `@container (max-width: 320px)`, which needs a `container-type`
ancestor these pages never establish. So the query never fires and a long key (a Go test
name, a rule id) takes the whole `auto` track.

Fixed the same way the tables were: below 40rem the key becomes a label above its value.
Measured after: every value 49 characters wide, on all four pages, both languages.

And `make check-ui` now measures `.datalist dd` and `.timeline p` alongside table cells and
step bodies — the check was right about the rule and wrong about the surface area, which is
the more common way a check fails.

One more thing the run caught: the `make ui` terminal block needed a sideways scroll on a
390px screen. The parenthetical moved out into a sentence.
