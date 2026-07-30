/* ══ highlight.js — real syntax highlighting, arriving after the block is already drawn ══
   Hides: shiki, its engine, and which grammars exist. Nothing else imports it.

     await paint(document)   // upgrades every <nes-code data-lang> not yet upgraded

   ── why this is an upgrade and not a renderer ──
   `<nes-code>` (8bit-nes) already draws the block the instant `dressCode` creates it: the
   frame, the filename header, a working COPY button, and a first pass of colour from the
   library's own `highlightCode`. That pass is one regex with a fixed JS keyword list, so in a
   Go or SQL answer it colours comments, strings and numbers and leaves every keyword plain.
   This file replaces the *contents of the <pre>* once a real grammar has arrived, and touches
   nothing else — the header and the button are the library's and stay the library's.

   That split is what makes the cost honest: an answer with no code fetches nothing, and an
   answer with code is readable before shiki lands. If shiki never lands — offline, or a
   grammar this file does not carry — the vendor's colours stay. There is no empty frame and no
   error, the same trade `diagram.js` makes for mermaid.

   Two facts from the vendor's source make the in-place swap safe, and both were read out of the
   installed bytes rather than assumed — first on 0.14.0, re-read on the 0.15.0 bump and
   unchanged, along with the reason this file exists at all: `<nes-code>` still has no `lang`
   attribute and still highlights with one JS-keyword regex, so nothing upstream does this yet:
     · its escaper only encodes & < >, so `pre.textContent` is byte-for-byte the original
       source — that is where the text to re-highlight comes from.
     · it renders once behind a `_done` flag, with no MutationObserver and no observed
       attributes, so nothing re-renders underneath this and undoes it.

   Colours come from the app's palette, not shiki's: `createCssVariablesTheme` emits
   `style="color:var(--shiki-token-…)"` and `styles.css` binds those names to the same tokens
   the library's own `.t-*` classes use. Shiki ships no colour of its own here.
   ══════════════════════════════════════════════════════════════════════════════════════ */

/* The grammars this app is willing to fetch, one static import each.
   A map rather than `import(`@shikijs/langs/${lang}`)`: a fully dynamic specifier into
   node_modules is not something the bundler can follow, so it would either fail the build or
   ship every grammar. Written out, Rollup gives each one its own chunk and fetches only the
   languages an answer actually contains.
   A fence in a language absent from this map is not a failure — it keeps the vendor's colours,
   which is why the list can stay short and grow when an answer needs it to. */
const LANGS = {
  bash: () => import('@shikijs/langs/shellscript'),
  css: () => import('@shikijs/langs/css'),
  go: () => import('@shikijs/langs/go'),
  html: () => import('@shikijs/langs/html'),
  javascript: () => import('@shikijs/langs/javascript'),
  json: () => import('@shikijs/langs/json'),
  shellscript: () => import('@shikijs/langs/shellscript'),
  sql: () => import('@shikijs/langs/sql'),
  typescript: () => import('@shikijs/langs/typescript'),
  vue: () => import('@shikijs/langs/vue'),
  yaml: () => import('@shikijs/langs/yaml'),
}

/** What a fence label means, for the spellings a model actually writes. */
const ALIAS = { js: 'javascript', jsonc: 'json', sh: 'shellscript', shell: 'shellscript', ts: 'typescript', yml: 'yaml' }

const THEME = 'css-variables'

let loading = null // one in-flight load, however many blocks arrive at once
const loaded = new Set() // grammars already registered on the highlighter

/**
 * The language to highlight a block as, or "" when this app carries no grammar for it.
 * @param {string} label the fence's own word, already lowercased by `dressCode`
 * @returns {string}
 */
export function language(label) {
  const name = ALIAS[label] ?? label
  return name in LANGS ? name : ''
}

/**
 * The highlighter, built once. Resolves to null when shiki cannot be reached, so every caller
 * degrades to the colours already on screen instead of throwing into a render.
 * @returns {Promise<object|null>}
 */
function core() {
  loading ??= build().catch(() => {
    loading = null // a later answer may succeed where this one failed
    return null
  })
  return loading
}

async function build() {
  const [{ createCssVariablesTheme, createHighlighterCore }, { createJavaScriptRegexEngine }]
    = await Promise.all([import('shiki/core'), import('shiki/engine/javascript')])
  return createHighlighterCore({
    themes: [createCssVariablesTheme({ name: THEME })],
    langs: [],
    // The JavaScript engine, not Oniguruma: the WASM one is ~150 KB gzipped against ~2 KB for
    // this, for the same grammars. `forgiving` is what keeps that trade safe — the JS engine is
    // strict by default and *throws* on a pattern it cannot compile, which would take out an
    // answer's whole render over one exotic grammar rule. Forgiving skips the rule instead, so
    // the worst case is a token left uncoloured.
    engine: createJavaScriptRegexEngine({ forgiving: true }),
  })
}

/**
 * Upgrade every code block under `root` that has a grammar and has not been upgraded yet.
 *
 * Safe to call repeatedly: `data-hl` marks a block done, which matters because the answer's
 * HTML is rebuilt on every render and the same turn can be painted more than once.
 *
 * @param {ParentNode} root
 * @returns {Promise<void>}
 */
export async function paint(root) {
  const blocks = [...root.querySelectorAll('nes-code[data-lang]:not([data-hl])')]
    .filter(b => b.dataset.lang in LANGS)
  if (blocks.length === 0)
    return

  const hl = await core()
  if (!hl)
    return

  const want = [...new Set(blocks.map(b => b.dataset.lang))].filter(l => !loaded.has(l))
  const grammars = await Promise.all(want.map(l => LANGS[l]().catch(() => null)))
  for (const [i, g] of grammars.entries()) {
    if (!g)
      continue
    await hl.loadLanguage(g)
    loaded.add(want[i])
  }

  for (const block of blocks) repaint(block, hl)
}

/* Replaces the <pre>'s children and nothing else. The default `structure` is deliberate: its
   `inline` form joins lines with <br>, which renders correctly but empties `pre.textContent` of
   newlines — and that string is the only copy of the source this element still holds. Keeping
   real newlines is what lets a second paint read the code back unchanged. */
function repaint(host, hl) {
  const pre = host.querySelector('.codeblock > pre')
  const lang = host.dataset.lang
  if (!pre || !loaded.has(lang))
    return
  const tpl = document.createElement('template')
  tpl.innerHTML = hl.codeToHtml(pre.textContent, { lang, theme: THEME })
  const code = tpl.content.querySelector('code')
  if (!code)
    return
  pre.innerHTML = code.innerHTML
  host.dataset.hl = '1'
}
