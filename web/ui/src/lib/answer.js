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
 * Render one conversation turn. This is `answerHtml` with the two mid-stream rules applied,
 * and it lives here rather than in ChatTurn.vue because both are decisions about what the
 * HTML may contain — which is this file's job — and a component that decides is a branch
 * rule 11 forbids. What they decide:
 *
 *   · no citation links while streaming — the source list only lands at the end, so a "[1]"
 *     mid-stream has nothing to point at
 *   · no diagrams until the renderer has arrived AND the stream has finished — <nes-mermaid>
 *     must never exist before the thing that draws it, and a half-written fence is not a
 *     diagram yet
 *
 * @param {{ a: string, streaming?: boolean, citations?: {n: number}[] }} turn
 * @param {boolean} diagramsReady whether the lazy mermaid chunk has loaded
 * @param {(n: number) => string} srcId maps a citation number to its source element id
 */
export function turnHtml(turn, diagramsReady, srcId) {
  const done = !turn.streaming
  return answerHtml(turn.a, {
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
    code.parentElement.replaceWith(el)
  }
  return tpl.innerHTML
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
