# 2026-07-29 — Four claims that were not true, and 189 KB nobody reads

A ponytail-lens audit of the whole tree. The mechanical half found nothing: `make dead` 0,
`make lint` 0 issues, `make lint-js` 0, `npx knip` 0, `make check` green, no dead exports in
`web/ui/src/lib`, no dead classes in `styles.css`, no unused i18n key. Everything below is in
the half no tool checks — and every one of them was a *claim* rather than code.

## `npx knip` was cited as an enforcer and nothing ran it

Rules 17 and 20 named it in the enforcer column. No Make target runs it, it is not a
devDependency, and `make check` runs vet · lint · lint-js · dead and no knip. By this file's
own standard — "a rule with no enforcer is a hope" — the citation was decoration.

**Deleted from both columns rather than wired in**, and the reason is the number: run by hand
over a 20-file front end it reports **zero**. Wiring it would add a `npx` network fetch to
every CI run, or a devDependency, to keep finding zero. If the front end ever grows past one
person's head, add it to `lint-js` in the same commit as the row that cites it.

## `skills-lock.json` does not pin anything

Rule 23 said the skills were "hash-pinned". They are not: the `computedHash` of all 16 fails
to reproduce with sha256/sha1/sha512 over the raw file, the stripped file, CRLF-normalised,
frontmatter-stripped, or the whole directory concatenated — and nothing in this repo computes
or even reads the file. It is written by whatever fetched the skills, so it is **provenance,
not a seal**, and the wording now says that.

What replaced the hope is the join CLAUDE.md had already specified and nobody had written:
`TestVendoredSkillsMatchTheirRouting` in `web/embed_test.go` (rule 21 — the file that owns
root-doc invariants, not a new one). It asserts `.agents/skills/*` ⇔ `.claude/skills/*` ⇔
`skills-lock.json` ⇔ named in CLAUDE.md's *Skills* section, plus the reverse for the refused
list. Verified red both ways before being left green: removing one symlink fails on the set
mismatch, re-adding `pnpm` fails on "listed as deliberately not vendored and is vendored
anyway". The hash is deliberately **not** asserted — a check that can only ever fail is worse
than no check.

## Three skills were vendored for surfaces this repo does not have

`pnpm`, `antfu-design` and `code-documenter` — **189 KB in 37 reference files**, all three
already listed in CLAUDE.md as not applicable. The old reasoning was that a deleted skill is a
re-added skill. It is not: the name with its reason does that job at zero bytes, and it is now
enforced by the test above. Deleted: directories, symlinks, lock entries. The rows stay, under
a heading that no longer claims they are on disk — each now naming its upstream (`antfu/skills`,
`jeffallan/claude-skills`), because the lock entry was the only in-tree record of where a skill
came from and deleting it without that would have made re-vendoring an archaeology dig.

Sixteen remain — 102 files, 674 KB of content (`du` says 1.1M; that is 4K blocks, not bytes) —
and the routing table in CLAUDE.md is unchanged: every one of them was already routed.

## `.env.example` was a second copy of every default

Nineteen knobs, fourteen of them printed at the value `internal/config` already defines. The
file's own header admitted it ("Every other line here is already its default"), which is the
exact shape rule 17 cites as its worked example — nine keys with two homes, deleted once
before.

What made the duplication unnecessary is new: `/#/admin` lists every setting with the value it
resolved to **and where that came from**. So the file now carries the five keys you have a
decision about (`AI_API_KEY`, `AUTH_USER`, `AUTH_PASS`, `ADMIN_PASS`, `BA_PASS`), the traps
that cost an hour each (EMBED_DIM must match the model · loopback is deliberate · zero hides a
figure), and one line naming the other fourteen. Checked by hand that all 19 in
`Config.Inventory()` are still named somewhere in it — a knob nobody names is a knob nobody
sets.

`config.go`'s comment said Inventory follows "the order and grouping .env.example uses". The
direction is now stated the other way round, because the code is the source and the file is
what points at it.

## Also stale, also fixed

- `README.md` claimed 8bit-nes **0.8.0** while the pin had reached 0.13.0. The number is gone
  rather than corrected: `web/vendor.sha384` is its one home, and the test that guards a quoted
  version only reads `AGENTS.md` — which is why this drifted with `make check` green.
- Five comments in `styles.css` claimed the present tense about 0.8.0; each fact was re-read
  from the installed 0.13.0 CSS and the numbers moved. One also said "THE ONE LOCAL OVERRIDE"
  while AGENTS.md lists seven.
- Three hardcoded `2px` where the library now ships tokens (`--sp-hair` for a list gap,
  `--bw-2` for two hairlines). Identical values, so nothing moved on screen; the point is that
  the app stops restating a library number.

## What was looked at and left alone

- **Single-caller exports in `web/ui/src/lib`** (13 of them). Not rule-20 violations: each is a
  layer boundary, which is what rule 22 buys with them.
- **The long comments.** Ponytail says delete an explanation longer than its code; *Conventions*
  says this repo's comments are the style guide and name the failure that motivated the code.
  Rules outrank a skill (precedence tier 1 over tier 3), so the density stays — but a comment
  whose *fact* has expired is rule-24 debt, which is what the fixes above were.
- **`outline-offset: 2px`** — no token means that, and inventing a mapping is worse than a
  literal.
