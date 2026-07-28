// Browser check for the published docs set — the measurements no Go test can make.
//
//   make check-ui        renders, serves, measures, cleans up
//   node scripts/check-docs-ui.mjs http://localhost:8080
//
// It exists because two layout bugs shipped that every other check passed: a table
// whose value columns were four characters wide on a phone (the design system's `th`
// is `nowrap`, so one long row header ate the row), and a numbered list rendered as
// mono uppercase columns (`.steps` in 8bit-nes is a horizontal STAGE 1 · 2 · 3 bar,
// not an instruction list). Both were visible in a screenshot and invisible to
// `go test`, so what a screenshot shows is now measured: characters per box, the gap
// between every pair of blocks, and the chrome of the header.
//
// The browser is driven by PinchTab (scripts/pinchtab.mjs), which replaced Playwright. Not a
// dependency of this product either — a tool, like mermaid — and `make check-ui` skips when
// it is not installed. What changed with it: the old skip condition was a hardcoded
// `/opt/node22/...` path that existed on one machine, so on every other machine this check
// skipped silently, and a skipped check reads exactly like a passing one.
//
// Verifies the restructured four-page docs set: one page per role, each split into
// feature-sized sections. Read on a phone (390) and on a laptop (1440), in both
// languages, with every in-page anchor and cross-page link resolved.
//
// What is deliberately NOT asserted, and why:
//   • <nes-toc> rows are one per h2/h3 and the component owns the shape (collapsed bar
//     under 80rem, open rail above). So the check is "no row *wraps*", measured against
//     the row's own line-height — not "the index fits on one line", which it never does.
//   • inline links inside prose are exempt from WCAG 2.5.5 (the "inline" exception:
//     their height is constrained by the line-height of the text around them).
//
// ── ONE GAP, NAMED ──────────────────────────────────────────────────────────────
// Touch-target size is NOT checked here any more, and it used to be: chrome controls
// (nav, toc, tabs, walkthrough dots, the section finder) were held to 44px on the phone.
//
// That assertion needed a *coarse pointer*, because 8bit-nes sizes its chrome to 32px and
// lifts it to 44px under `@media (pointer: coarse)`. Playwright supplied it through
// `devices["iPhone 14"]`, which enables touch emulation. PinchTab has none: `--mobile` on
// `set viewport` leaves `navigator.maxTouchPoints` at 0, `set media pointer coarse` reports
// "applied" while `matchMedia("(pointer: coarse)")` stays false (CDP emulates a fixed set of
// media features and `pointer` is not in it), and `--touch-events=enabled` through
// `browser.extraFlags` changed nothing. "touch" appears nowhere in its CLI, config or API.
//
// So the rule is real and unchecked. It is guarded by 8bit-nes' own release testing —
// AGENTS.md records that 0.6.1 shipped both touch fixes this repo reported, which is why the
// app owns no coarse-pointer CSS of its own — and by looking at a phone. If PinchTab gains
// touch emulation, the measurement to restore is a `small: [...]` list of chrome controls
// under 44px; see changelog/2026-07-28-pinchtab-checks.md for what it looked like.
import { open } from "./pinchtab.mjs";

const BASE = process.argv[2] || "http://127.0.0.1:8123";
const PAGES = ["index.html", "ba.html", "dev.html", "deploy.html"];

const errs = [];
const pt = open(`${BASE}/${PAGES[0]}`);

const lang = () => document.documentElement.dataset.lang;

const setLang = (want) => {
  if (pt.evalJson(lang) !== want) {
    pt.click("#lang");
    pt.sleep(250);
  }
  return pt.evalJson(lang);
};

const measure = () => {
  const doc = document.documentElement;
  const seen = (e) => e.offsetParent !== null;
  const lang = doc.dataset.lang;
  const secs = [...document.querySelectorAll("main > section")];
  return {
    hScroll: doc.scrollWidth - innerWidth,
    sections: secs.length,
    ids: secs.map(s => s.id).filter(Boolean),
    // A section whose heading in the reading language is missing reads as an
    // untitled slab — the one bilingual mistake that is invisible in the other half.
    untitled: secs.filter(s => !s.querySelector(`h2[lang="${lang}"]`)).length,
    // Sub-modules: h3 inside a section, per section, in the reading language.
    subs: secs.map(s => s.querySelectorAll(`:scope > h3[lang="${lang}"]`).length),
    toc: (() => {
      const t = document.querySelector("nes-toc");
      const rows = [...t.querySelectorAll("nav.outline a")];
      const wrapped = rows.filter(a => {
        const lh = Number.parseFloat(getComputedStyle(a).lineHeight) || 16;
        return a.getBoundingClientRect().height > lh * 1.6;
      }).map(a => a.textContent.trim());
      return {
        rail: t.dataset.rail !== undefined,
        rows: rows.length,
        wrapped,
        // The collapsed bar names the section you are in; blank means the rebuild
        // dropped it (a bug this repo has already had once).
        now: t.querySelector(".toc-now")?.textContent.trim() || "",
        // Every row must point at an id that exists, or the index is decoration.
        dead: rows.map(a => a.getAttribute("href").slice(1))
          .filter(id => !document.getElementById(id)),
      };
    })(),
    // Wide content must scroll inside its own box, never widen the page.
    clipped: [...document.querySelectorAll("main .terminal, main table, main pre, main .datalist, main .mermaid-view")]
      .filter(e => e.scrollWidth > e.clientWidth + 1)
      .filter(e => getComputedStyle(e).overflowX === "visible")
      .map(e => `${e.tagName}.${e.className}`.slice(0, 40)),
    diagrams: [...document.querySelectorAll("main svg.flowchart")]
      .map(s => ({ nodes: s.querySelectorAll(".node").length,
                   w: Math.round(s.getBoundingClientRect().width),
                   fits: s.getBoundingClientRect().width <= s.parentElement.clientWidth + 1 })),
    links: [...new Set([...document.querySelectorAll("main a[href], .pages a[href]")]
      .filter(seen).map(a => a.getAttribute("href"))
      .filter(h => h.startsWith("./") || h.startsWith("#")))],
    anchors: [...document.querySelectorAll("[id]")].map(e => e.id),
    lang: doc.lang,
    // ── what the phone screenshots showed and the checks above did not ──
    // A box too narrow to hold words. `.table th` is nowrap, so one long row header
    // used to leave ~4 characters for each value column and every cell wrapped one
    // word per line. Measured in characters, because px means nothing without a size.
    // Every recipe on these pages that holds a sentence. `.datalist` was added after a
    // screenshot showed its value column at fourteen characters on a phone — the same
    // failure as the tables, in a recipe the first version of this check did not look at.
    narrow: [...document.querySelectorAll(
      "main .table td, main .table th, main .step > .body, main .datalist dd, main .timeline p",
    )]
      .filter(seen).filter(e => e.textContent.trim().length > 12)
      .map(e => ({ txt: e.textContent.trim().slice(0, 24),
                   ch: Math.round(e.getBoundingClientRect().width /
                       (Number.parseFloat(getComputedStyle(e).fontSize) * 0.55)) }))
      .filter(x => x.ch < 18),
    // Blocks stuck to each other, and how many distinct gaps the page uses. One rhythm
    // means a small set of deliberate values, not four accidental ones plus zero.
    rhythm: (() => {
      const gaps = {}, touching = [];
      for (const sec of document.querySelectorAll("main > section")) {
        const kids = [...sec.children].filter(seen);
        for (let i = 1; i < kids.length; i++) {
          const g = Math.round(kids[i].getBoundingClientRect().top -
                               kids[i - 1].getBoundingClientRect().bottom);
          gaps[g] = (gaps[g] || 0) + 1;
          if (g < 8) touching.push(`${kids[i - 1].tagName} → ${kids[i].tagName} = ${g}px`);
        }
      }
      return { gaps: Object.keys(gaps).map(Number).sort((a, b) => a - b), touching };
    })(),
    // The design system's `.steps` is a horizontal STAGE 1 · 2 · 3 progression, not a
    // numbered instruction list: using it for one renders mono uppercase columns four
    // characters wide. `.step` blocks are the recipe on these pages.
    stepsMisuse: document.querySelectorAll("main .steps > li").length,
    // Below 40rem a table row is a block, and each value carries its column heading.
    stacked: (() => {
      const row = [...document.querySelectorAll("main .table > tbody > tr")].filter(seen)[0];
      if (!row) return null;
      return { display: getComputedStyle(row).display,
               labelled: document.querySelectorAll(`main .table td[data-label-${lang}]`).length };
    })(),
    // The section finder: present, thumb-sized, on the toggle's row, and in the
    // reading language. Its popup must also be above the sticky index — that is a
    // stacking bug you can see and cannot tap.
    find: (() => {
      const box = document.getElementById("find");
      const input = box?.querySelector("input");
      if (!input) return { present: false, hidden: box?.hidden ?? null };
      const r = input.getBoundingClientRect();
      const toggle = document.getElementById("lang").getBoundingClientRect();
      return { present: true, h: Math.round(r.height), ph: input.placeholder,
               opts: box.querySelector("nes-input-menu")?.opts?.length ?? 0,
               sameRow: Math.abs(box.getBoundingClientRect().top - toggle.top) < 8,
               aboveToc: Number.parseInt(getComputedStyle(document.querySelector(".bar")).zIndex, 10) >
                         Number.parseInt(getComputedStyle(document.querySelector("nes-toc")).zIndex, 10) };
    })(),
  };
};

const out = {};
// 390×664 at dpr 3 with mobile emulation is Playwright's "iPhone 14" preset, spelled out.
// The viewport is set *before* each navigation, never after: <nes-toc> picks rail-or-bar when
// it upgrades and does not re-pick on resize, so a resized page reports the wrong shape.
//
// 1920 is here because --col has a rung there and nothing was measuring above it: the page
// stopped growing at 960px and every check ran at 1440, so a wider column could widen a
// table past its wrapper, or push the rail off, and both would have passed. It is the
// cheapest rung to add — the measurements below are the ones already written.
for (const [width, height, mobile] of [[390, 664, true], [1440, 900, false], [1920, 1080, false]]) {
  for (const page of PAGES) {
    pt.viewport(width, height, { dpr: mobile ? 3 : 1, mobile });
    pt.nav(`${BASE}/${page}`);
    const r = {};
    for (const want of ["en", "vi"]) r[want] = { got: setLang(want), ...measureNow() };
    out[`${page}@${width}`] = r;
    errs.push(...pt.drain(`${page}@${width}`));
  }
}

function measureNow() {
  return pt.evalJson(measure);
}

// Link resolution over the union of what every page declares. Anchors are read from
// the EN render because the toc slugs the reading language's headings; the manual
// section ids the cross-page links use are language-independent by design.
const anchors = {};
for (const page of PAGES) anchors[page] = new Set(out[`${page}@1440`].en.anchors);
const broken = [];
for (const page of PAGES) {
  for (const href of out[`${page}@1440`].en.links) {
    const [file, frag] = href.startsWith("#")
      ? [page, href.slice(1)]
      : [href.replace(/^\.\//, "").split("#")[0], href.split("#")[1]];
    if (!anchors[file]) { broken.push(`${page}: no such page ${file}`); continue; }
    if (frag && !anchors[file].has(frag)) broken.push(`${page} → ${href}`);
  }
}
// Every page must be reachable from every other, or a role lands in a dead end.
for (const page of PAGES) {
  const to = new Set(out[`${page}@1440`].en.links.map(h => h.replace(/^\.\//, "").split("#")[0]));
  for (const other of PAGES) {
    if (other !== page && !to.has(other)) broken.push(`${page} does not link ${other}`);
  }
}

const fails = [];
const need = (c, why) => { if (!c) fails.push(why); };
need(errs.length === 0, `console/network: ${errs.slice(0, 5)}`);
need(broken.length === 0, `links: ${broken.join(" · ")}`);
for (const [key, r] of Object.entries(out)) {
  const phone = key.endsWith("@390");
  for (const lang of ["en", "vi"]) {
    const o = r[lang];
    need(o.got === lang, `${key}: language toggle stuck at ${o.got}`);
    need(o.lang === lang, `${key}: html[lang]=${o.lang} in ${lang}`);
    need(o.hScroll <= 1, `${key} ${lang}: page scrolls sideways by ${o.hScroll}px`);
    need(o.untitled === 0, `${key} ${lang}: ${o.untitled} section(s) with no ${lang} h2`);
    need(o.clipped.length === 0, `${key} ${lang}: unscrollable overflow in ${o.clipped}`);
    need(o.sections >= 6, `${key} ${lang}: only ${o.sections} sections`);
    need(o.toc.rows >= o.sections, `${key} ${lang}: toc lists ${o.toc.rows} of ${o.sections} sections`);
    need(o.toc.wrapped.length === 0, `${key} ${lang}: toc rows wrap: ${o.toc.wrapped}`);
    need(o.toc.dead.length === 0, `${key} ${lang}: toc rows point nowhere: ${o.toc.dead}`);
    need(o.toc.rail === !phone, `${key} ${lang}: toc shape rail=${o.toc.rail}`);
    if (phone) need(o.toc.now !== "", `${key} ${lang}: collapsed toc bar names no section`);
    // No 44px assertion here — see ONE GAP, NAMED at the top of this file. The rule still
    // holds; nothing in reach can emulate the coarse pointer it depends on.
    need(o.diagrams.every(d => d.nodes >= 5 && d.fits),
      `${key} ${lang}: diagrams ${JSON.stringify(o.diagrams)}`);
    need(o.narrow.length === 0,
      `${key} ${lang}: text boxes under 18 characters wide: ${JSON.stringify(o.narrow)}`);
    need(o.rhythm.touching.length === 0,
      `${key} ${lang}: blocks with no gap: ${o.rhythm.touching.slice(0, 4)}`);
    need(o.rhythm.gaps.length <= 3,
      `${key} ${lang}: ${o.rhythm.gaps.length} different gaps (${o.rhythm.gaps}) — one rhythm, not five`);
    need(o.stepsMisuse === 0,
      `${key} ${lang}: ${o.stepsMisuse} <li> under .steps — that recipe is a stage bar; use .step blocks`);
    need(o.find.present && o.find.opts >= 5 && o.find.sameRow && o.find.aboveToc,
      `${key} ${lang}: section finder ${JSON.stringify(o.find)}`);
    need(o.find.ph === (lang === "vi" ? "Tìm mục trong trang…" : "Find a section…"),
      `${key} ${lang}: finder placeholder is ${JSON.stringify(o.find.ph)}`);
    if (phone) {
      need(o.stacked && o.stacked.display === "block" && o.stacked.labelled > 0,
        `${key} ${lang}: table rows not stacked on a phone: ${JSON.stringify(o.stacked)}`);
      // `o.find.h` is still measured and still printed in the finder failure above — it is
      // the height a coarse-pointer check would have asserted, kept as the diagnostic it can
      // still be without the emulation to judge it.
    } else {
      need(o.stacked === null || o.stacked.display === "table-row",
        `${key} ${lang}: table rows stacked on a laptop: ${JSON.stringify(o.stacked)}`);
    }
  }
}
// One rendered diagram per page that claims to visualise its flow.
for (const [key, n] of [["index.html@390", 1], ["ba.html@390", 1], ["dev.html@390", 1]]) {
  need(out[key].en.diagrams.length >= n, `${key}: no rendered diagram`);
}

console.log(JSON.stringify(Object.fromEntries(Object.entries(out).map(([k, v]) =>
  [k, { sections: v.en.sections, ids: v.en.ids, subs: v.en.subs, toc: v.en.toc,
        diagrams: v.en.diagrams, hScroll: v.en.hScroll }])), null, 1));
if (fails.length) console.log("\n" + fails.join("\n"));
console.log(fails.length ? "\nDOCS: FAIL" : "\nDOCS: PASS");
// No teardown here: the browser instance belongs to the wrapper that started it, and it stops
// it on the way out — including when this exits non-zero.
process.exit(fails.length ? 1 : 0);
