/* ══ answer.js — markdown → safe, cited HTML ══════════════════════════════════
   Hides: marked configuration, DOMPurify, and the "[n]" → <a class="cite"> pass.

     answerHtml(markdown, { count: 2, srcId: (n) => `s7-${n}` })

   `marked` and `DOMPurify` are globals here on purpose — index.html loads them as
   classic scripts so they cost no module graph. This is the only file that knows.
   ═══════════════════════════════════════════════════════════════════════════ */

import DOMPurify from 'dompurify'
import { marked } from 'marked'

import { isDiagram } from './diagram.js'

let configured = false

/**
 * Render one answer.
 * @param {string} markdown  raw model output
 * @param {{ nums?: number[], srcId?: (n: number) => string }} sources
 *   nums  — the citation numbers that exist. NOT a count: the engine returns only
 *   the sources the answer cited, keeping their original numbers, so an answer
 *   that used [2] alone arrives with nums [2] and one source. Comparing against a
 *   length would leave that marker unlinked.
 *   srcId — element id for source n. Omit either and no linking happens (the
 *   right behaviour mid-stream, when the citation list hasn't arrived yet).
 *   diagrams — turn ```mermaid fences into <nes-mermaid>. Off while streaming (half
 *   a graph is a parse error) and off until the renderer is actually loaded, so the
 *   fallback is the code block the model wrote.
 * @returns {string} sanitized HTML
 */
export function answerHtml(markdown, { nums = [], srcId, diagrams = false } = {}) {
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
