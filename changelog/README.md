# changelog/

Session handoffs, newest last. One file per working session, named
`YYYY-MM-DD-<what-it-was-about>.md`.

These are **not** release notes — the git log already is one. A file here exists so
the next session (a different harness, a fresh context, a cloud agent) can pick the
work up without re-deriving what a previous one already learned: what is deployed
where, which decisions are settled, which are still open, and which host quirks cost
an hour to find.

Write one when a session ends with state outside the repo (a running instance, a
half-finished decision, a pending credential) — not for an ordinary commit.

What earns a place in one:

- **State that lives outside git**: hosts, paths, unit names, what is running.
- **Decisions and their reason**, so the next session does not relitigate them.
- **Pending work with an acceptance test**, not a wish list.
- **Landmines**: the thing that looked obvious and was wrong.

What does not: a diff summary (`git log` has it), anything already in
[`README.md`](../README.md) or [`AGENTS.md`](../AGENTS.md), and **never a secret** —
name the file that holds it instead.
