/* ══ answer.js — markdown → safe, cited HTML ══════════════════════════════════
   Hides: marked configuration, DOMPurify, and the "[n]" → <a class="cite"> pass.

     turnHtml(turn, diagramsReady, (n) => `s7-${n}`)

   `marked` and `DOMPurify` are npm dependencies bundled by Vite, imported here and
   nowhere else in the app. This is still the only file that knows they exist — that part
   was always the point; they stopped being globals when the bundler arrived.
   ═══════════════════════════════════════════════════════════════════════════ */

import DOMPurify from 'dompurify'
import { marked } from 'marked'

import { isDiagram } from './diagram.js'
import { language } from './highlight.js'

let configured = false

/**
 * Render one conversation turn. This is `answerHtml` with the four mid-stream rules applied,
 * and it lives here rather than in ChatTurn.vue because all of them are decisions about what
 * the HTML may contain — which is this file's job — and a component that decides is a branch
 * rule 11 forbids. What they decide:
 *
 *   · no citation links while streaming — the source list only lands at the end, so a "[1]"
 *     mid-stream has nothing to point at
 *   · no diagrams until the renderer has arrived AND the stream has finished — <nes-mermaid>
 *     must never exist before the thing that draws it, and a half-written fence is not a
 *     diagram yet
 *   · no <nes-code> while streaming, and for a harder reason than the diagram: it reads its
 *     text once at upgrade and then refuses to re-render, so a block created halfway through
 *     a function would show that half for good. A plain fence in the meantime.
 *   · the clarify block stays in the prose while streaming, and only moves out of it once
 *     the reply is whole. Half a checklist is a set of options with one still missing, and
 *     `dressAlerts` renders it as a panel in the meantime — so what the reader sees is the
 *     question forming, then becoming pickable, rather than an empty card
 *
 * @param {{ a: string, streaming?: boolean, citations?: Citation[] }} turn
 * @param {boolean} diagramsReady whether the lazy mermaid chunk has loaded
 * @param {(n: number) => string} srcId maps a document citation number to its row's element id
 * @param {(n: number) => string} webSrcId the same for a [wN] public result, whose numbering
 *   restarts at 1 — so the two ids must not be able to collide
 */
export function turnHtml(turn, diagramsReady, srcId, webSrcId) {
  const done = !turn.streaming
  return answerHtml(done ? stripClarify(turn.a).rest : turn.a, {
    diagrams: done && diagramsReady,
    code: done,
    // The citations themselves, not how many: the engine returns only the sources the answer
    // cited and keeps their original n, so [2] can arrive alone — and `kind` is what says
    // which numbering an entry belongs to.
    cites: done ? turn.citations : [],
    srcId,
    webSrcId,
  })
}

/**
 * Render one answer. Not exported: `turnHtml` above is the only caller, and each option below
 * is a decision it already makes. An export with one in-file caller is a second entry point
 * to keep in agreement with the first — `knip` said so the moment ChatTurn.vue stopped using
 * it.
 *
 * @param {string} markdown raw model output
 * @param {{ cites?: Citation[], srcId?: (n: number) => string,
 *   webSrcId?: (n: number) => string, diagrams?: boolean, code?: boolean }} sources
 *   cites — the citations that exist. NOT a count: the engine returns only the sources the
 *   answer cited, keeping their original numbers, so an answer that used [2] alone arrives
 *   with one entry numbered 2. Comparing against a length would leave that marker unlinked.
 *   srcId / webSrcId — element id for a document source and for a public one. Omit them and
 *   no linking happens, which is the right behaviour mid-stream, when the citation list has
 *   not arrived yet.
 *   diagrams — turn ```mermaid fences into &lt;nes-mermaid&gt;. Off while streaming (half a
 *   graph is a parse error) and off until the renderer is actually loaded, so the fallback is
 *   the code block the model wrote.
 *   code — turn every remaining fence into &lt;nes-code&gt;. Off while streaming for a harder
 *   reason than diagrams: that element reads its text once and then refuses to re-render, so
 *   one created mid-stream would freeze half a function on the page for good.
 * @returns {string} sanitized HTML
 */
function answerHtml(markdown, { cites = [], srcId, webSrcId, diagrams = false, code = false } = {}) {
  if (!configured) {
    marked.setOptions({ breaks: true })
    configured = true
  }
  let html = DOMPurify.sanitize(marked.parse(markdown || ''))
  html = dressTables(html)
  html = dressTaskLists(html)
  html = dressAlerts(html)
  html = dressImages(html)
  if (cites.length && srcId)
    html = linkCites(html, cites, srcId, webSrcId)
  if (diagrams)
    html = asDiagrams(html)
  // Last, and after asDiagrams on purpose — see its own comment.
  return code ? dressCode(html) : html
}

/* ── images: a screenshot is worth the paragraph it replaces ────────────────────
   A BA writes `![what it looks like](https://…)` in a document and the answer that cites it
   shows the picture. Nothing here allows the tag — DOMPurify's defaults already keep <img> with
   src and alt — this only narrows what a src may be and adds what a remote image needs.

   `data:` is refused, and the reason is retrieval rather than security: a document's body is the
   thing that gets chunked and embedded, so a base64 image would be sent to the embedding model
   *as text*. A few hundred KB of it would poison the vector, blow the chunk up and be paid for
   on every re-index. DOMPurify allows data: on <img> by default, so the refusal has to be said
   out loud. The alt text stays behind in its place: dropping the node silently would delete the
   only description of the thing the BA was pointing at.

   `https:` only, and the three attributes are what stop a remote image from costing more than
   it shows: no referrer (the host learns nothing about who is reading), lazy (an image far down
   an answer does not hold up the first paint), async decode (a big screenshot does not stall
   the main thread while the rest of the answer is still arriving). */
function dressImages(html) {
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  for (const img of tpl.content.querySelectorAll('img')) {
    if (!/^https:\/\//i.test(img.getAttribute('src') ?? '')) {
      img.replaceWith(img.getAttribute('alt') ?? '')
      continue
    }
    img.setAttribute('referrerpolicy', 'no-referrer')
    img.setAttribute('loading', 'lazy')
    img.setAttribute('decoding', 'async')
    // The viewer opens on tap, and an <img> is not focusable — so without this the only way in
    // is a pointer. The `.prose` keydown listener that already serves diagrams handles Enter and
    // Space, so one attribute is the whole keyboard path.
    img.setAttribute('tabindex', '0')

    /* A paragraph holding nothing but the image is not a paragraph. Markdown wraps every
       standalone image in one, and that wrapper is what puts the picture inside the *reading*
       measure: the library opts `img` out of `--prose-measure`, but only as a direct child of
       `.prose`, so a `<p>` in between defeats the rule the library states itself — "if the
       content is the width, it opts out".
       Measured on 0.15.0: a 1200px screenshot rendered at 646px, half the card, because its
       parent paragraph was capped at 72ch. Unwrapping hands the picture back to the rule.
       A paragraph with text *and* an image keeps both — there the picture is part of a
       sentence and the measure is the right answer. */
    const p = img.parentElement
    const alone = p?.tagName === 'P'
      && [...p.childNodes].every(n => n === img || (n.nodeType === Node.TEXT_NODE && !n.data.trim()))
    if (alone)
      p.replaceWith(img)
  }
  return tpl.innerHTML
}

/* ── code: the library's block, upgraded later by a real grammar ────────────────
   `<nes-code>` brings the frame, the filename header, a working COPY button and a first pass of
   colour, for nothing — it is already in the bundle. lib/highlight.js replaces the colour with a
   real grammar once one has been fetched; this only has to produce the element and say which
   language it is.

   It runs AFTER asDiagrams, and that order is load-bearing: asDiagrams also matches
   `pre > code`, so a fence turned into <nes-code> first would never become a picture. The
   skip below covers the other half of the same trap — while the mermaid chunk is still
   loading `diagrams` is false, so a graph is still sitting here as a plain fence and must be
   left alone for the render that comes after the renderer lands.

   `file` carries the fence's own word. That slot is a filename header in the library, and using
   it for a language is a judgement call: there is no other slot, and "go" above a block is worth
   more to a reader than an empty header. `data-lang` is the resolved grammar name and is only
   set when this app actually carries one — highlight.js filters on it, so an unknown language
   keeps the vendor's colours instead of asking for a chunk that does not exist. */
function dressCode(html) {
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  for (const code of tpl.content.querySelectorAll('pre > code')) {
    const cls = [...code.classList].find(c => c.startsWith('language-'))
    const label = cls?.slice(9).toLowerCase() ?? ''
    if (label === 'mermaid' || (!cls && isDiagram(code.textContent)))
      continue
    const el = document.createElement('nes-code')
    if (label) {
      el.setAttribute('file', label)
      const lang = language(label)
      if (lang)
        el.dataset.lang = lang
    }
    el.textContent = code.textContent
    code.parentElement.replaceWith(el)
  }
  return tpl.innerHTML
}

/* Markdown emits a bare <table>, and every one of the design system's table styles hangs
   off a *class* — so an answer that enumerates ("list every convention for X") rendered as
   an unstyled browser table: no mono headers, no row hover, and on a 390px screen a
   three-column table of convention text ran off the side of the page, which is the exact
   failure the docs pages already had to fix.
   `.table-wrap` is the library's own overflow container, so a wide table scrolls inside
   itself and the page never moves sideways. Done here rather than with a marked renderer
   override because it must also run on a table that arrives mid-stream. */
function dressTables(html) {
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  for (const table of tpl.content.querySelectorAll('table')) {
    table.classList.add('table')
    // Already wrapped if the model emitted its own container, or if a re-render runs.
    if (table.parentElement?.classList.contains('table-wrap'))
      continue
    const wrap = document.createElement('div')
    wrap.className = 'table-wrap'
    table.replaceWith(wrap)
    wrap.append(table)
  }
  return tpl.innerHTML
}

/* A checked/unchecked list becomes the design system's `.tasklist`.
   This deliberately teaches the model *no* new syntax: `- [x]` / `- [ ]` is GFM, which every
   model already writes correctly, and marked already parses it into exactly the shape
   `.tasklist` styles — a <ul> of <li>, with `.done` marking the finished ones. A directive
   would have cost prompt tokens on every question to describe a structure that already
   exists, and this repo's own prompt comment is firm that a rule the model never needed
   dilutes the ones that matter.
   The raw <input type=checkbox> is removed because `.tasklist li::before` draws the box
   itself; leaving both renders two. Text nodes are untouched, so `[n]` inside a row is still
   there for linkCites to turn into a citation. */
function dressTaskLists(html) {
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  for (const ul of tpl.content.querySelectorAll('ul')) {
    const items = [...ul.children].filter(li => li.tagName === 'LI')
    const boxes = items.filter(li => li.querySelector('input[type="checkbox"]'))
    // Every item, or it is an ordinary list that happens to mention a checkbox.
    if (!items.length || boxes.length !== items.length)
      continue
    ul.classList.add('tasklist')
    for (const li of items) {
      const box = li.querySelector('input[type="checkbox"]')
      if (box.checked || box.hasAttribute('checked'))
        li.classList.add('done')
      box.remove()
    }
  }
  return tpl.innerHTML
}

/* ── panels: a caveat the reader cannot walk past ───────────────────────────────
   PICK holds the two markers the clarify card owns, ALERTS the five GFM ones plus those
   two, and both regexes are built from the same keys — the kind list is one fact.

   The markers are GitHub's alert syntax because that is the argument dressTaskLists makes
   above, applied to the other half: every model already writes "> [!WARNING]", so the
   prompt spends one line naming which five kinds mean what instead of describing a
   structure. marked 18 has no native support for it — "> [!WARNING]" parses as an ordinary
   <blockquote><p> with the marker as a leading text node — so the marker is stripped here
   rather than by a renderer override, for dressTables' reason: it has to work on a
   blockquote that arrived mid-stream.

   QUESTION and NEXT are in ALERTS as the fallback for a clarify block stripClarify could
   not read. It takes a blockquote and the list under it together, so a marker the model
   wrote without its checklist would otherwise leak "[!QUESTION]" onto the page as text.
   Mid-stream that is every one of them, which is where the fallback actually earns its
   keep: the panel renders as the question is written, and turns into the pickable card
   when the reply is whole. */
const PICK = { QUESTION: 'quest', NEXT: 'info' }

/* What the card is called when the model wrote the marker with nothing after it, which
   `make smoke` caught it doing on the first real answer: "> [!NEXT]" alone on its line, so the
   group had no name and the fieldset rendered an empty legend. The prompt asks for the label
   and mostly gets one — this is what happens the rest of the time. */
const LABEL = { QUESTION: 'Which one do you mean?', NEXT: 'Ask next' }
/* GENERAL is this app's own sixth kind, and the only one that is not GitHub's: it holds the
   model's own explanation of a term the documents lean on and never define. It needs a colour
   the library has not already spoken for, because the whole point is that a reader can see at
   a glance which sentences their organisation vouched for — `.explain` in styles.css is four
   lines picking `--teal`, one of the six accent tokens 0.15.0 ships and aliases to nothing. */
const ALERTS = {
  NOTE: 'info',
  TIP: 'tip',
  IMPORTANT: 'memo',
  WARNING: 'warn',
  CAUTION: 'gotcha',
  GENERAL: 'explain',
  ...PICK,
}

/* No glyph is prepended here any more, and the deletion is the point. This file used to put one
   emoji in front of the first word because `.callout` in 0.15.0 was a border and a tint and
   nothing else — so WARNING and CAUTION were two oranges to a reader who had not learned the
   palette, and one colour to a colour-blind one. That went upstream as WCAG 1.4.1 and 0.16.0
   ships the recipe's own: `.callout::before { content: var(--mark) / "" }`, one ASCII character
   per kind, in a reserved gutter, hidden from a screen reader. Keeping both would print two
   marks on every panel, and the library's is the better of the two — a gutter holds for a panel
   that opens with a list, where a character prepended into the first paragraph has no line to
   join. `.explain` is this app's own kind, so styles.css gives it its own `--mark`. */

const marker = kinds => new RegExp(`^\\[!(${Object.keys(kinds).join('|')})\\]\\s*`)
const alertMark = marker(ALERTS)
const clarifyMark = marker(PICK)

/** The text of a blockquote's opening paragraph — where a marker has to be to count. */
const lead = quote => (quote.tokens?.[0]?.type === 'paragraph' ? quote.tokens[0].text : '')

/* The next token that is content. A blank line between the marker and its checklist — which
   is how the block reads, and so how a model writes it — is its own `space` token, and
   treating "followed by" as `tokens[i + 1]` therefore missed every well-formed block and
   matched only the cramped ones. */
function after(tokens, i) {
  let j = i + 1
  while (tokens[j]?.type === 'space') j += 1
  return j
}

function dressAlerts(html) {
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  for (const quote of tpl.content.querySelectorAll('blockquote')) {
    const first = quote.firstElementChild
    const start = first?.tagName === 'P' ? first.firstChild : null
    const kind = start?.nodeType === Node.TEXT_NODE && alertMark.exec(start.data)
    if (!kind)
      continue
    // The marker had the line to itself, so the <br> that `breaks: true` put after it would
    // open the panel with a blank line.
    if (start.data === kind[0] && start.nextSibling?.tagName === 'BR')
      start.nextSibling.remove()
    start.data = start.data.slice(kind[0].length)
    if (!start.data) {
      start.remove()
      // The marker had the whole paragraph — "> [!NOTE]" with its prose in the next quoted
      // block, which is how a model writes one about half the time. What is left is an empty
      // <p>, and it used to render as a blank first line inside the panel; now it would take
      // the glyph with it and put that on a line of its own.
      if (!first.hasChildNodes())
        first.remove()
    }
    // Nothing left, so there is nothing to panel. Two ways a blockquote arrives holding only
    // its marker, and both shipped an empty bordered box with the real content stranded under
    // it: the model wrote `> [!NOTE]` and put the prose in a *sibling* block instead of the
    // quote, and — every alert, every time — the moment mid-stream when the marker has arrived
    // and its first word has not. Removing the quote rather than keeping an empty one is what
    // makes the streaming case read right: the panel appears with its first character instead
    // of flashing as an empty frame first. An alert with no content is not an alert.
    if (!quote.textContent.trim()) {
      quote.remove()
      continue
    }
    /* A <div>, not the blockquote with a class added: the library styles blockquote as a
       pull-quote — italic, and capped at a reading measure — and .callout does not undo
       either, so a panel left as one rendered at 646px inside a 1207px card, in italics.
       This is the markup the guide pages already use for the same recipe, which is the other
       reason to swap: one appearance for one thing. It also makes the pass idempotent for
       free — a second run finds a div and no blockquote to match. */
    const panel = document.createElement('div')
    panel.className = `callout ${ALERTS[kind[1]]}`
    panel.append(...quote.childNodes)
    dropRepeats(panel)
    quote.replaceWith(panel)
  }
  return tpl.innerHTML
}

/* The marker above is read off the *first* text node of the first paragraph, which is the only
   place a well-formed alert puts one. A model that writes two caveats writes them as two lines
   of one quote:

     > [!WARNING] Cú pháp cho cấu trúc dữ liệu không có trong tài liệu.
     > [!WARNING] Cú pháp cơ bản không có trong tài liệu.

   marked folds consecutive quoted lines into one blockquote and one paragraph, and `breaks:
   true` joins them with a <br> — so the second marker is a later text node, the loop above has
   already moved past it, and "[!WARNING]" rendered as literal text inside the panel it was
   asking for. The invariant is the point: a marker is syntax, so it never reaches the reader
   as characters. The panel keeps the kind the first one named — a second panel for a second
   line would split one caveat in two. */
function dropRepeats(panel) {
  const walk = document.createTreeWalker(panel, NodeFilter.SHOW_TEXT)
  while (walk.nextNode()) {
    const n = walk.currentNode
    // Line-leading only: a sentence *about* "[!WARNING]" is prose, and stripping that would
    // edit what the documents say.
    if (n.previousSibling?.tagName !== 'BR')
      continue
    n.data = n.data.replace(alertMark, '')
  }
}

/**
 * The pickable block a turn is currently showing, or null.
 *
 * `turnHtml`'s companion, and here for its reason: whether the block may be interactive yet is
 * a rendering rule, so a component asking for it holds no branch of its own. Nothing stores
 * the result — the block is still in `turn.a`, which is the field that already persists, so a
 * card the reader has not answered survives a reload without session.js knowing it exists.
 *
 * @param {{ a: string, streaming?: boolean }} turn
 * @returns {Clarify | null}
 */
export function turnClarify(turn) {
  return turn.streaming ? null : stripClarify(turn.a).clarify
}

/**
 * @typedef {object} Clarify
 * @property {string} kind QUESTION when the reply is a question back, NEXT when it is an offer
 * @property {string} prompt the sentence sharing the marker's line
 * @property {{ text: string, cites: string[], recommended: boolean }[]} options one per item
 */

/* An option's [n] is a citation, not part of its wording, and it used to be treated as both:
   rendered verbatim in the card — "Các biến và cách sử dụng [2]", bare brackets on a row whose
   own prose above renders the same marker as a chip — and then stripped a second time inside
   composeClarify before the pick went back as a question. Two copies of one rule, and the
   reader got the raw one.

   Split once, here. The label is what a reader reads, `cites` is what the card renders with the
   library's `.cite` recipe, and composeClarify has nothing left to strip. */
/** @typedef {import("./chat.js").Citation} Citation */

/* Both markers, one pattern, because neither caller cares which kind it found: `[1]` is one of
   this organisation's documents, `[w1]` is a public search result. They cannot collide — the
   character after `[` is a digit in one and `w` in the other — so one regex reads both and the
   capture says which.

   Declared above both readers rather than beside either. `split` renders a marker inside the
   prose and `option` below takes one out of a checklist item, and the day those were two
   patterns the clarify card printed "[w2]" as characters while the paragraph above it drew the
   same marker as a chip — the exact defect this file already fixed once for `[2]`. One fact,
   one place, or the second copy is the one that goes stale. */
const CITE = /\[(w?)(\d+)\]/g

/**
 * One checklist item → a pickable option, with its citations taken out of the wording.
 *
 * `cites` holds the marker's own text — "2" for a document, "w1" for a public result — which is
 * exactly what `cite()` prints for the same marker in the prose, so the card and the paragraph
 * label a citation identically.
 *
 * One pass with `replaceAll` and a function, not a `matchAll` beside a `replaceAll`: `CITE` is
 * global and shared now, and two calls would be two chances to inherit a `lastIndex` — the trap
 * `linkCites` below documents having hit.
 */
function option(item) {
  const cites = []
  const text = item.text
    .replaceAll(CITE, (m) => {
      cites.push(m.slice(1, -1))
      return ''
    })
    // A marker between two words leaves two spaces behind; trim only catches the ends.
    .replace(/\s{2,}/g, ' ')
    .trim()
  return { text, cites, recommended: item.checked === true }
}

/**
 * Split a reply into the prose that renders and the pickable block inside it.
 *
 * Read with marked's own lexer rather than a scan over the text: the block is GFM this file
 * already parses, so the parser is what should say what is in it — a regex over "> "-prefixed
 * lines reimplements blockquote and list parsing, badly, which is the mistake dressTaskLists
 * argues against one layer up.
 *
 * Only a blockquote *immediately followed by* a list counts. That is what lets the two
 * positions the prompt asks for — [!QUESTION] opening a reply, [!NEXT] closing one — share
 * one code path, and what stops a [!WARNING] panel that happens to have a list under it from
 * being mistaken for one.
 *
 * Every one of them comes out of the prose, and only one comes back. The prompt already says a
 * [!QUESTION] ends the reply ("write nothing after that checklist"), so a reply holding both is
 * a model ignoring it — and taking the first match alone rendered that mistake at its worst:
 * the card is drawn *under* the prose, so lifting the [!QUESTION] out and leaving the [!NEXT]
 * behind put the offer at the top of the answer, as an inert `.callout` with a `.tasklist`
 * nobody can tick, and the real card below it. Two boxes that look alike, in the wrong order,
 * one of them clickable. A reply that asks has nothing to offer next, so the question wins and
 * the offer goes; either way nothing is left in the prose pretending to be pickable.
 *
 * @param {string} markdown raw model output
 * @returns {{ rest: string, clarify: Clarify | null }} rest is the markdown without any block
 */
function stripClarify(markdown) {
  const tokens = marked.lexer(markdown || '')
  const found = []
  for (const [i, t] of tokens.entries()) {
    if (t.type !== 'blockquote' || !clarifyMark.test(lead(t)))
      continue
    const end = after(tokens, i)
    if (tokens[end]?.type === 'list')
      found.push({ at: i, end })
  }
  if (found.length === 0)
    return { rest: markdown, clarify: null }
  const read = ({ at, end }) => {
    const opening = lead(tokens[at])
    const kind = clarifyMark.exec(opening)[1]
    return {
      kind,
      prompt: opening.replace(clarifyMark, '').trim() || LABEL[kind],
      options: tokens[end].items.map(option),
    }
  }
  const cards = found.map(read)
  return {
    // A top-level token's `raw` is its own slice of the source, so dropping every block's own
    // tokens and joining the rest is how the answer around them survives being read. The range
    // rather than the two ends: the blank line between them is a token too, and leaving it
    // behind puts a gap where the block used to be.
    rest: tokens
      .filter((_, i) => !found.some(({ at, end }) => i >= at && i <= end))
      .map(t => t.raw)
      .join(''),
    clarify: cards.find(c => c.kind === 'QUESTION') ?? cards[0],
  }
}

/**
 * The question a submitted pick is worth asking again.
 *
 * Empty when nothing was ticked, which needs no handling of its own: `ask()` already returns
 * on a blank question, so an empty submit is a silent no-op rather than a turn with no
 * question in it.
 *
 * The picks and nothing else, for both kinds. A [!QUESTION] pick used to be sent with the
 * card's own legend in front of it, on the reasoning that a reading means nothing without the
 * question it is a reading *of* — and what that produced was a new turn whose heading repeated,
 * word for word, the sentence still on screen two blocks above it: "Bạn muốn biết về phần nào
 * của cú pháp Go? Cú pháp cơ bản". It read as a bug, and it retrieved like one too — the legend
 * is the model asking something, so half the query was wording that appears in no document.
 * The option is the wording a reader would recognise, which is what the prompt asks the model
 * to put there and what retrieval wants.
 *
 * @param {FormData} form the card's own form — one "reading" entry per ticked option
 * @returns {string}
 */
export function composeClarify(form) {
  // No [n] to strip: `option` above took the citation out of the wording before the card ever
  // rendered it, so what a checkbox carries is already the question's words.
  return form.getAll('reading').map(text => text.trim()).filter(Boolean).join(' ; ')
}

/* Runs on the sanitized DOM, never on the markdown: injecting anchors before
   marked would make a "[1]" inside a code fence render as literal HTML. Text
   inside code/pre — or an existing link — is left exactly as written. */
function linkCites(html, cites, srcId, webSrcId) {
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  // Two sets rather than one, because the numbering restarts: [1] and [w1] are different
  // sources and a single set would link one of them to the other's row.
  const docs = new Set(cites.filter(c => c.kind !== 'web').map(c => c.n))
  const web = new Set(cites.filter(c => c.kind === 'web').map(c => c.n))

  const walk = document.createTreeWalker(tpl.content, NodeFilter.SHOW_TEXT)
  const hits = []
  while (walk.nextNode()) {
    const n = walk.currentNode
    if (n.parentElement?.closest('code, pre, a'))
      continue
    // Its own pattern, without CITE's captures and without its /g: a shared /g regex carries
    // lastIndex between calls, so every other text node would report no match.
    if (/\[w?\d+\]/.test(n.nodeValue))
      hits.push(n)
  }

  for (const node of hits)
    node.replaceWith(split(node.nodeValue, { docs, web, srcId, webSrcId }))
  return tpl.innerHTML
}

function split(text, { docs, web, srcId, webSrcId }) {
  const frag = document.createDocumentFragment()
  let last = 0
  for (const m of text.matchAll(CITE)) {
    const isWeb = m[1] === 'w'
    const n = Number(m[2])
    const valid = isWeb ? web : docs
    if (!valid.has(n))
      continue // a marker with no source points at nothing
    frag.append(text.slice(last, m.index), cite(m[0].slice(1, -1), (isWeb ? webSrcId : srcId)(n)))
    last = m.index + m[0].length
  }
  frag.append(text.slice(last))
  return frag
}

/* Runs after DOMPurify, never before: <nes-mermaid> is a custom element and the
   sanitizer would strip it. The diagram source is the code block's own text, so it
   was already sanitized as text — and the element reads its textContent, which is
   why this survives being serialized back to a string. */
function asDiagrams(html) {
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  for (const code of tpl.content.querySelectorAll('pre > code')) {
    const lang = [...code.classList].find(c => c.startsWith('language-'))
    // Labelled `mermaid`, or unlabelled and starting like a diagram. The second case
    // is the common one: models fence a graph without naming the language, and a
    // reader then gets source code where a picture was meant.
    const labelled = lang?.slice(9).toLowerCase() === 'mermaid'
    if (!labelled && !(lang === undefined && isDiagram(code.textContent)))
      continue
    const el = document.createElement('nes-mermaid')
    el.textContent = code.textContent
    const pre = code.parentElement
    const walk = walkAfter(pre)
    pre.replaceWith(el)
    if (walk)
      walk.attach(el)
  }
  return tpl.innerHTML
}

/* ── the walk: a diagram that explains itself, one node at a time ───────────────
   A picture shows the shape and says nothing about what any box means, so the prompt
   asks for the explanation as a numbered list under the diagram — one item per node,
   opening with that node's label in bold. This turns that list into
   <nes-walkthrough>, which is prev/next + arrow keys + progress dots, and which calls
   `highlight(focus)` on whatever `for` names.

   Nothing here highlights anything: <nes-mermaid> ships `highlight()` and re-applies
   the focus after every draw, so the only work is naming the node. That is the whole
   reason this is short — the same reason the guide's five diagrams are twenty lines in
   docsbase.html rather than a component each.

   It is a *fold*, not an addition: the list becomes the stepper and the original is
   removed, or the same prose would sit on the page twice. So it happens only when the
   diagram really became a drawing — while the renderer is still loading the fence stays
   a fence, and its list has to stay with it or the answer loses the explanation and
   keeps nothing that shows it. */
let nWalks = 0

/**
 * Read the walk that belongs to a diagram, without touching the document yet.
 *
 * Two passes, because the decision comes before the mutation: a list where one item has
 * no bold lead is not a node walk — a model listing something else entirely — and half a
 * fold would leave the answer holding a stepper with a hole in it.
 *
 * @param {Element} pre the diagram's own <pre>
 * @returns {{ attach: (host: Element) => void } | null} null when there is no walk to make
 */
function walkAfter(pre) {
  const ol = pre.nextElementSibling
  if (ol?.tagName !== 'OL')
    return null
  const steps = []
  for (const li of ol.children) {
    if (li.tagName !== 'LI')
      continue
    const copy = li.cloneNode(true)
    const lead = copy.querySelector(':scope > strong')
    const label = lead?.textContent?.trim()
    if (!label)
      return null
    lead.remove()
    steps.push({
      title: label,
      // What the library matches against the text of every .node / .cluster it drew.
      focus: [label],
      // Already sanitized — this is a slice of the DOMPurify output above, and the
      // component sets it with innerHTML.
      body: copy.innerHTML.replace(/^\s*(?:[—–:-]\s*)?/, ''),
    })
  }
  // One step is not a walk: both buttons would be disabled and the dots would be a dot.
  if (steps.length < 2)
    return null

  return { attach: host => fold(host, ol, steps) }
}

function fold(host, ol, steps) {
  host.id = `nes-walk-${++nWalks}`
  // Only a diagram with a walk under it gets the height cap — see styles.css.
  host.dataset.walk = '1'
  const wt = document.createElement('nes-walkthrough')
  wt.setAttribute('for', host.id)
  const json = document.createElement('script')
  json.type = 'application/json'
  // `<script>` is a raw-text element, so the serializer does not escape what is inside
  // it: a "</script" anywhere in this JSON would end the block early and spill the rest
  // as markup. DOMPurify cannot leave one behind (it escapes text and drops scripts),
  // and this closes the class rather than relying on that.
  json.textContent = JSON.stringify(steps).replaceAll('<', '\\u003c')
  wt.append(json)
  ol.replaceWith(wt)
}

function cite(n, href) {
  const a = document.createElement('a')
  a.className = 'cite' // 8bit-nes recipe: superscript marker, jumps to .source
  a.href = `#${href}`
  a.setAttribute('aria-label', `Source ${n}`)
  a.textContent = String(n)
  return a
}

/** "docs/architecture/retrieval.md" → "retrieval.md" */
export function fileName(path) {
  return (path || '').split('/').pop()
}

/**
 * "Booking List Module > 12. Flows > 12.14 Print Preview" → "12.14 Print Preview"
 *
 * The same trade as `fileName` above, for the other half of a citation: show the leaf,
 * keep the whole path in the `title`. The head of a breadcrumb is the document, which the
 * filename beside it already names — so it is the part that repeats on every row, and the
 * tail is the part that says *where in the file*. Six citations from two documents spent
 * their width on "Developer Handoff — …" six times and pushed the section that
 * distinguishes them onto a second line.
 */
export function section(heading) {
  return (heading || '').split('>').pop().trim()
}
