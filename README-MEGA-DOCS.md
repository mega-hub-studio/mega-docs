# Mega Knowledge Engine Platform vNext

> **This is the brief, not the spec.** Every line below describes the product this is
> becoming; none of it describes the tree you are reading. Before implementing anything
> from here, read **Now vs vNext** in [`README.md`](README.md) — it marks each line
> shipped, next, or blocked-and-on-what. Two do not simply await work: hybrid retrieval
> **stays** and PDF/DOCX is **out of scope** — both settled decisions against this brief. The
> source-of-truth inversion is no longer blocked: the migration runner shipped, and the backup
> precondition was dropped and then met anyway. The reasoning is in [`changelog/`](changelog/); re-deriving it
> costs a day and lands on the wrong answer.
>
> Keep this file a statement of intent. A claim about what the code does today goes in
> `README.md` — that is critical rule 19, and `TestRootDocsAreTheFourWeKnowAbout` fails
> when the join disappears.

## Vision
Build a **Knowledge Engine Platform** for enterprises with:
- ChatGPT-like AI chat
- RAG-powered answers
- NES 8-bit visual components
- BA-managed knowledge through WebUI
- Dev consumes knowledge
- Admin manages platform

## Principles
1. KISS
2. DX
3. ROI
4. Maintainability
5. SaaS Ready
6. Clean Architecture
7. Web First
8. Component Driven

## Removed
- Ollama
- LM Studio
- Local LLMs
- Local Embeddings
- CLI import *(as a way **in**. `ingest` stays as an operator recovery tool)*
- Folder watch *(done — the `corpus-sync.path` unit is gone)*
- Git sync *(done 2026-07-28 — `scripts/corpus-sync.sh` deleted. Nothing backs the corpus
  up automatically now; backing up is an operator action)*
- Background sync
- Hybrid **provider and import** pipelines — *not* hybrid retrieval: vector + BM25 fused
  with RRF **stays**, because BM25 is the half that matches an error code or a config key
  verbatim over a Vietnamese corpus. "One pipeline" means one path a question travels, and
  there is already exactly one `Answer`. Decided 2026-07-28.

## Single Knowledge Pipeline
```text
BA
 ↓
WebUI Upload
 ↓
Knowledge DB
 ↓
OpenAI text-embedding-3-small
 ↓
Vector Index
 ↓
Chat
 ↓
NES Components Preview
```

## Single Source of Truth
Only BA uploads via WebUI.

## Supported Files
- Markdown
- TXT

**PDF and DOCX are out of scope.** Decided 2026-07-28, for KISS: a binary-format parser
inside the binary is a CVE surface in a service with a write gate, and an external converter
invoked at upload is a per-file failure a BA cannot fix. Converting stays a one-time step
outside the product — `markitdown spec.pdf > spec.md` — and the upload refusal names that
command rather than reporting an unsupported type.

Automatic:
Chunk → Embed → Save → Index

## Roles

### Admin
- Users
- Workspace
- Billing
- Logs
- Analytics
- Platform Settings

### BA
- Upload
- CRUD
- Preview
- Version
- Publish
- Archive
- Reindex

### DEV
- Search
- Chat
- Export
- Feedback
- Read Only

## AI Stack (MVP)

### LLM
- OpenAI

### Embeddings
- text-embedding-3-small

No vendor switching during MVP.

Future SaaS:
User Settings → Provider Plugin
- OpenAI
- Claude
- Gemini
- Azure OpenAI
- OpenRouter

## Rendering

LLM
↓
Markdown Components
↓
Renderer
↓
NES Components

Never render raw HTML.

## Response Format
- Answer
- Visual Components
- References
- Related Documents
- Suggested Actions

## RAG Flow
```text
Question
 ↓
Embedding
 ↓
Vector Search
 ↓
Context
 ↓
LLM
 ↓
Markdown Components
 ↓
Renderer
 ↓
NES UI
```

## Knowledge Model
- Document
- Sections
- Chunks
- Embeddings
- References
- Tags
- Categories
- Relations
- Version

## UI
Everything through WebUI.
No CLI.
No YAML.
No manual config.

Reuse NES component library before creating new components.

## Self-host: cross-OS, fastest path
One binary, no runtime, nothing fetched at start — so the same artifact self-hosts on
**WSL2** and **macOS** as well as a Linux VM. Only the process supervisor differs, and that
difference is documented rather than abstracted away: systemd on Linux/WSL2, `launchd` on
macOS. No Docker, no orchestration, no per-OS build.

Two platform facts that cost an hour each if undocumented, and are therefore on the Deploy
page: WSL2 ships with **systemd off** (`/etc/wsl.conf` → `[boot] systemd=true`), and
`launchd` has **no `EnvironmentFile=`**, so the process must start with the right working
directory for `.env` to be found.

## Success Metrics
- One knowledge source
- One RAG pipeline
- One documentation standard
- Three roles
- WebUI only
- OpenAI only (MVP)
- Markdown Components → NES Renderer
