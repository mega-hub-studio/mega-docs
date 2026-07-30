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

let configured = false

/**
 * Render one conversation turn. This is `answerHtml` with the three mid-stream rules applied,
 * and it lives here rather than in ChatTurn.vue because all of them are decisions about what
 * the HTML may contain — which is this file's job — and a component that decides is a branch
 * rule 11 forbids. What they decide:
 *
 *   · no citation links while streaming — the source list only lands at the end, so a "[1]"
 *     mid-stream has nothing to point at
 *   · no diagrams until the renderer has arrived AND the stream has finished — <nes-mermaid>
 *     must never exist before the thing that draws it, and a half-written fence is not a
 *     diagram yet
 *   · the clarify block stays in the prose while streaming, and only moves out of it once
 *     the reply is whole. Half a checklist is a set of options with one still missing, and
 *     `dressAlerts` renders it as a panel in the meantime — so what the reader sees is the
 *     question forming, then becoming pickable, rather than an empty card
 *
 * @param {{ a: string, streaming?: boolean, citations?: {n: number}[] }} turn
 * @param {boolean} diagramsReady whether the lazy mermaid chunk has loaded
 * @param {(n: number) => string} srcId maps a citation number to its source element id
 */
export function turnHtml(turn, diagramsReady, srcId) {
  const done = !turn.streaming
  return answerHtml(done ? stripClarify(turn.a).rest : turn.a, {
    diagrams: done && diagramsReady,
    // The numbers, not how many: the engine returns only the sources the answer cited and
    // keeps their original n, so [2] can arrive alone.
    nums: done ? turn.citations.map(c => c.n) : [],
    srcId,
  })
}

/**
 * Render one answer. Not exported: `turnHtml` above is the only caller, and each option below
 * is a decision it already makes. An export with one in-file caller is a second entry point
 * to keep in agreement with the first — `knip` said so the moment ChatTurn.vue stopped using
 * it.
 *
 * @param {string} markdown raw model output
 * @param {{ nums?: number[], srcId?: (n: number) => string, diagrams?: boolean }} sources
 *   nums — the citation numbers that exist. NOT a count: the engine returns only the sources
 *   the answer cited, keeping their original numbers, so an answer that used [2] alone
 *   arrives with nums [2] and one source. Comparing against a length would leave that marker
 *   unlinked.
 *   srcId — element id for source n. Omit either and no linking happens, which is the right
 *   behaviour mid-stream, when the citation list has not arrived yet.
 *   diagrams — turn ```mermaid fences into &lt;nes-mermaid&gt;. Off while streaming (half a
 *   graph is a parse error) and off until the renderer is actually loaded, so the fallback is
 *   the code block the model wrote.
 * @returns {string} sanitized HTML
 */
function answerHtml(markdown, { nums = [], srcId, diagrams = false } = {}) {
  if (!configured) {
    marked.setOptions({ breaks: true })
    configured = true
  }
  let html = DOMPurify.sanitize(marked.parse(markdown || ''))
  html = dressTables(html)
  html = dressTaskLists(html)
  html = dressAlerts(html)
  if (nums.length && srcId)
    html = linkCites(html, new Set(nums), srcId)
  return diagrams ? asDiagrams(html) : html
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
const ALERTS = { NOTE: 'info', TIP: 'tip', IMPORTANT: 'memo', WARNING: 'warn', CAUTION: 'gotcha', ...PICK }
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
    if (!start.data)
      start.remove()
    /* A <div>, not the blockquote with a class added: the library styles blockquote as a
       pull-quote — italic, and capped at a reading measure — and .callout does not undo
       either, so a panel left as one rendered at 646px inside a 1207px card, in italics.
       This is the markup the guide pages already use for the same recipe, which is the other
       reason to swap: one appearance for one thing. It also makes the pass idempotent for
       free — a second run finds a div and no blockquote to match. */
    const panel = document.createElement('div')
    panel.className = `callout ${ALERTS[kind[1]]}`
    panel.append(...quote.childNodes)
    quote.replaceWith(panel)
  }
  return tpl.innerHTML
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
 * @property {{ text: string, recommended: boolean }[]} options one per checklist item
 */

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
 * @param {string} markdown raw model output
 * @returns {{ rest: string, clarify: Clarify | null }} rest is the markdown without the block
 */
export function stripClarify(markdown) {
  const tokens = marked.lexer(markdown || '')
  const at = tokens.findIndex((t, i) =>
    t.type === 'blockquote' && tokens[after(tokens, i)]?.type === 'list' && clarifyMark.test(lead(t)))
  if (at < 0)
    return { rest: markdown, clarify: null }
  const end = after(tokens, at)
  const opening = lead(tokens[at])
  const kind = clarifyMark.exec(opening)[1]
  return {
    // A top-level token's `raw` is its own slice of the source, so dropping the block's own
    // tokens and joining the rest is how the answer around it survives being read. The range
    // rather than the two ends: the blank line between them is a token too, and leaving it
    // behind puts a gap where the block used to be.
    rest: tokens.filter((_, i) => i < at || i > end).map(t => t.raw).join(''),
    clarify: {
      kind,
      prompt: opening.replace(clarifyMark, '').trim() || LABEL[kind],
      options: tokens[end].items.map(i => ({ text: i.text, recommended: i.checked === true })),
    },
  }
}

/**
 * The question a submitted pick is worth asking again.
 *
 * Empty when nothing was ticked, which needs no handling of its own: `ask()` already returns
 * on a blank question, so an empty submit is a silent no-op rather than a turn with no
 * question in it.
 *
 * @param {Clarify} clarify the block the card rendered
 * @param {FormData} form the card's own form — one "reading" entry per ticked option
 * @returns {string}
 */
export function composeClarify(clarify, form) {
  const picked = form
    .getAll('reading')
    // The [n] belongs to the option as *shown*, pointing at the source behind that reading.
    // Asked back it would be a citation number in a question, which retrieval reads as text.
    .map(text => text.replaceAll(/\[\d+\]/g, '').trim())
    .filter(Boolean)
  if (picked.length === 0)
    return ''
  const joined = picked.join(' ; ')
  // A [!NEXT] item is already a whole question and its prompt is only the heading over them.
  // A [!QUESTION] item is one reading *of the question that was asked*, so it means nothing
  // without that question in front of it.
  return clarify.kind === 'NEXT' ? joined : `${clarify.prompt} ${joined}`
}

/* Runs on the sanitized DOM, never on the markdown: injecting anchors before
   marked would make a "[1]" inside a code fence render as literal HTML. Text
   inside code/pre — or an existing link — is left exactly as written. */
function linkCites(html, valid, srcId) {
  const tpl = document.createElement('template')
  tpl.innerHTML = html

  const walk = document.createTreeWalker(tpl.content, NodeFilter.SHOW_TEXT)
  const hits = []
  while (walk.nextNode()) {
    const n = walk.currentNode
    if (n.parentElement?.closest('code, pre, a'))
      continue
    if (/\[\d+\]/.test(n.nodeValue))
      hits.push(n)
  }

  for (const node of hits) node.replaceWith(split(node.nodeValue, valid, srcId))
  return tpl.innerHTML
}

function split(text, valid, srcId) {
  const frag = document.createDocumentFragment()
  let last = 0
  for (const m of text.matchAll(/\[(\d+)\]/g)) {
    const n = Number(m[1])
    if (!valid.has(n))
      continue // a marker with no source points at nothing
    frag.append(text.slice(last, m.index), cite(n, srcId(n)))
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
