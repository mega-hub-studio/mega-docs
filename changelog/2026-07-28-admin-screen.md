# 2026-07-28 — The Admin screen, and what it deliberately cannot do

`/#/admin` exists: three tabs, `ADMIN_PASS`, and **not one control that changes anything**.
That last part is the design, not a first increment.

## Why read-only is the whole feature

The brief's Admin does Users, Workspace, Billing, Logs, Analytics and Platform Settings, and
the honest question was which of those an operator actually cannot do today. Answer: none of
them — except *see a setting*. The effective value of a knob lived in three places at once
(`.env`, `internal/config`'s defaults, whatever the shell already had) and **which of the
three won was a guess**. `os.Getenv` cannot tell them apart afterwards, because `loadDotEnv`
copies the file into the environment.

So the screen's centre column is `source`, and everything else follows from it being cheap.

What editing would have cost, and why it is not here: every knob is read once at startup —
`rag.New` bakes `TopK` into the engine — so a write path needs persistence, validation and a
reload for values nobody changes twice a year. "Edit `.env` and restart" stays the answer;
the screen makes it stop being a guess. Trigger to revisit: a knob that must change without
a restart.

Skipped, one line each, and none of them is a backlog item:

- **Users · Workspace** — no accounts, one instance. A password is a permission boundary, not
  an identity; per-user anything is the SaaS phase.
- **Billing** — no payments. `PRICE_IN`/`PRICE_OUT` are display-only, and the Runtime tab
  shows them for what they are.
- **Logs** — `journalctl -u knowledge` is the platform feature. A viewer in the WebUI would
  duplicate it and need a log sink to read from.

## The three tabs, and where their data comes from

| tab | content | source |
|---|---|---|
| Settings | 19 knobs, grouped as `.env.example` groups them, each with value + provenance | **new**: `GET /api/settings` |
| Runtime | corpus counts, chat model, window, prices, reachable, writes | `/api/health` + `/api/corpus`, already fetched by the shell — passed as props |
| Usage | what was asked, hit count, the scope it was answered in | `/api/history`, same |

Only the first needed a backend. The other two are props because the shell already had them
for the status line and the replay list, and a second fetch of the same thing is the second
copy rule 17 is about.

`<nes-tabs>` owns which tab is showing, so neither the component nor the composable keeps tab
state. That is the library doing the job — the reason to check `llms.txt` before writing.

## Decisions worth not re-deriving

**Its own password.** The settings list carries the *provenance of every secret on the box*,
so "may publish an answer" and "may read which passwords exist" are different permissions.
`TestSettingsNeedTheAdminPassword` asserts the BA password in the BA header gets 401 on this
route — the case that would otherwise pass silently for years.

**One gate, two secrets.** `internal/server/gate.go` is new and it *removed* a copy: the
constant-time compare and the 403-versus-401 distinction now exist once, with `BAPass` and
`AdminPass` as two values of it. A second compare is a second place to get subtly wrong.
Header names became constants because `goconst` counted seven copies of `X-BA-Pass` across
the package and was right to.

**An unset `ADMIN_PASS` leaves the route unregistered**, not refusing — so the test asserts
**404**, not 403. Same shape as the QA and import routes: a surface that cannot be unlocked
does not exist. And because a static bundle cannot discover which routes exist,
`/api/health` gained `"admin"`; the nav button renders only when it is true, so nobody taps a
tab that answers 404 and concludes the app is broken.

**Documents are never touched from Admin.** Import, remove, confirm and dismiss stay in
`BaScreen`, where the person doing that work already is. Two screens with the same button is
one button and one lie about which of them matters.

## Verified

Spec-first, and the gate was red for exactly four reasons before any code existed: two
missing tests, one unregistered route, one unread variable. Then:

```
GET /api/health                      → {"ok":true,"writes":true,"admin":true,…}
GET /api/settings  (no header)        → 401
GET /api/settings  (X-BA-Pass)        → 401   ← the separation, live
GET /api/settings  (X-Admin-Pass)     → the inventory, AI_API_KEY as {"value":"set","source":"env"}
```

`make check-full` green. `t.Setenv` on five secrets proves none of their values reach the
response *and* that all five are still present as rows — a response that simply omitted them
would have passed the redaction check for the wrong reason.
