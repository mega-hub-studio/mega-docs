# 2026-07-28 — The two browser checks run on PinchTab, and one had not run at all

`make check-ui` and `make check-wt` are driven by [PinchTab](https://pinchtab.com/) instead of
Playwright. The interesting part is not the swap.

## They were not running

Both wrappers gated on a hardcoded path:

```sh
PW=${PLAYWRIGHT_PATH:-/opt/node22/lib/node_modules/playwright/index.mjs}
if ! command -v node || [ ! -f "$PW" ]; then echo "  skipped ..."; exit 0; fi
```

That path does not exist on this machine, and `PLAYWRIGHT_PATH` was documented nowhere until
today. So the checks printed one grey line and exited 0 — and **a skipped check reads exactly
like a passing one**. Nineteen assertion families about the published guide had been dormant
for as long as nobody looked.

The new gate is `pinchtab doctor`, which is the only thing that can actually answer "is a
browser reachable". Skipping still happens; it now says which of the two reasons applies.

## What it took to make it reliable

The first working version was flaky, and every cause was silent-and-wrong rather than loud:

| symptom | cause | fix |
|---|---|---|
| `npm rebuild pinchtab` | the driver spawned the CLI with `env: {}`, so the npm launcher could not resolve its managed Go binary relative to `HOME` | pass `process.env` |
| every phone assertion passing at 1440px | `set viewport` targets the instance's *current tab*; a fresh instance has none, so it answered `409` on stderr while the run carried on at the default width | `open()` boots a tab first, and `viewport()` reads `innerWidth` back and throws if it did not take |
| `rail: true` on a 390px phone | `<nes-toc>` picks rail-or-bar when it upgrades and does not re-pick on resize | set the viewport **before** navigating, never after — documented at the top of the driver |
| every font reported as a failed request | `drain` matched `\b4\d\d\b` against text, and `nes-mono-400.woff2` contains "400" | parse `pinchtab network --json` and read `entries[].status` |
| `500 resolve current tab url: context canceled`, 2 runs in 3 | commands landed on the shared default instance, which an editor or an MCP integration navigates out from under a measurement (16 `pinchtab mcp` processes were running) | each wrapper starts its own instance on its own port and stops it on exit — `chromium.launch()`/`close()`, spelled differently |
| `404 tab … not found`, then `409 no current tab` | the CLI keeps a *current tab* in its own state directory, so a run starting a fresh instance on the same port inherited the previous run's tab id — and an agent id, tried as a fix, only moved the same staleness behind a per-agent tab | the boot nav takes `--print-tab-id` and every command afterwards carries `--tab <id>`. Ambient state is fine for a human at a prompt and wrong for a check that has to be repeatable |

Measured after: 3 runs of 4 pages × 2 viewports, byte-identical results, then two more
consecutive `DOCS: PASS` plus `WALKTHROUGHS: PASS` once the tab was pinned.

## The one assertion that did not survive

**Touch-target size is no longer checked.** 8bit-nes sizes chrome to 32px and lifts it to 44px
under `@media (pointer: coarse)`; Playwright supplied that pointer through
`devices["iPhone 14"]`, which enables touch emulation. PinchTab cannot:

- `set viewport --mobile` leaves `navigator.maxTouchPoints` at 0
- `set media pointer coarse` answers "applied" while `matchMedia("(pointer: coarse)")` stays
  false — CDP emulates a fixed set of media features and `pointer` is not one of them
- `--touch-events=enabled` via `browser.extraFlags` changed nothing (reverted)
- "touch" appears nowhere in PinchTab's CLI, its config, or its API reference

Raised as a loss and accepted deliberately: nineteen assertion families come back to life, one
goes dark. The rule still holds — AGENTS.md records that 8bit-nes 0.6.1 shipped both touch
fixes this repo reported, which is why the app owns no coarse-pointer CSS of its own — it is
just guarded by upstream's testing and by looking at a phone rather than by a measurement here.

The measurement to restore, if PinchTab ever grows touch emulation, was:

```js
small: [...document.querySelectorAll(".bar a, .bar button, .pages a, nes-toc a, nes-toc button, nes-tabs button, .wt-dot, .wt-nav button")]
  .filter(seen)
  .map((e) => {
    const r = e.getBoundingClientRect();
    const a = getComputedStyle(e, "::after");
    const g = (v) => (v === "auto" ? 0 : Math.max(0, -Number.parseFloat(v) || 0));
    const h = a.content !== "none" && a.position === "absolute"
      ? r.height + g(a.top) + g(a.bottom) : r.height;
    return { label: e.textContent.trim().slice(0, 16), h: Math.round(h), w: Math.round(r.width) };
  })
  .filter(x => x.w > 0 && x.h < 44),
```

asserted as `if (phone) need(o.small.length === 0, ...)`, plus `need(o.find.h >= 44, ...)`.
`o.find.h` is still measured and still printed in the finder diagnostic.

## Host state this needed

`~/.pinchtab/config.json` was changed on this machine, and neither change is in the repo:

- `browser.binary` → `~/.cache/ms-playwright/chromium-1228/chrome-linux64/chrome`. There is no
  system Chrome here; that is Playwright's download, and Chrome for Testing 149 is a Chrome.
  `apt-get install -y chromium` and an empty `binary` is the cleaner long-term answer, and the
  one to switch to if that cache is ever cleaned.
- **`security.allowEvaluate: false → true`**. Non-negotiable, not incidental: almost every
  question these checks ask is one `eval` returning a plain object. Worth knowing it is a
  posture change on the machine, not a repo setting.

`PLAYWRIGHT_PATH` is gone from the Makefile and CLAUDE.md. Playwright itself is no longer
referenced anywhere except the note in `scripts/check-docs-ui.mjs` explaining what its device
preset used to provide.
