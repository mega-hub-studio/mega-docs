# Mega Knowledge Engine Platform vNext

> **This is the brief, not the spec.** Every line below describes the product this is
> becoming; none of it describes the tree you are reading. Before implementing anything
> from here, read **Now vs vNext** in [`README.md`](README.md) — it marks each line
> shipped, next, or blocked-and-on-what. Three do not simply await work: hybrid retrieval
> **stays** (a settled decision against this brief), the source-of-truth inversion is
> blocked on an **off-box DB backup** (the migration runner it also needed has shipped),
> and PDF/DOCX is an **open parser choice**. The reasoning is in
> [`changelog/`](changelog/); re-deriving it costs a day and lands on the wrong answer.
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
- CLI import
- Folder watch
- Git sync
- Background sync
- Hybrid pipelines

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
- PDF
- DOCX
- Markdown
- TXT

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

## Success Metrics
- One knowledge source
- One RAG pipeline
- One documentation standard
- Three roles
- WebUI only
- OpenAI only (MVP)
- Markdown Components → NES Renderer
