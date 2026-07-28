// Drives every diagram walkthrough on every page — prev/next, keyboard, dots, and which
// node lights up for which step.
//
//   ./scripts/check-walkthroughs.sh      renders the guide, serves it, drives it, cleans up
//
// It exists because `data-focus` matches node *text*: renaming a node in a .mmd file, or
// rewording a step, silently stops the highlight without breaking anything a test or a
// screenshot would notice. Here, a step that lights nothing is a failure.
//
// Buttons are clicked through the DOM rather than through the driver's own click, and that
// choice outlived Playwright: the sticky header and the diagram box both overlap the stepper
// at some scroll positions, and the question here is whether the *component* works, not
// whether a tap lands. So every action below is one `evalJson` that clicks and measures in
// the same turn — no actionability waits to satisfy, and no scroll position to arrange.
//
// The one exception is ArrowLeft, which has to be a real key event: the point of that step is
// that the component's keyboard handler works, and dispatching a synthetic event would test
// this file's idea of the handler rather than the handler.
import { open } from "./pinchtab.mjs";
const BASE = process.argv[2] || "http://127.0.0.1:8123";
const PAGES = [["index.html", ["hiw"]], ["ba.html", ["gaploop"]],
               ["dev.html", ["pipe", "specloop", "loops"]]];
const out = {}, errs = [], fails = [];
const need = (c, why) => { if (!c) fails.push(why); };
const pt = open(`${BASE}/${PAGES[0][0]}`);

/**
 * One step of one walkthrough: do `what`, then report everything about where it landed.
 * Runs in the page, so the click and the measurement cannot disagree.
 */
const step = (id, what) => {
  const el = document.querySelector(`nes-walkthrough[for="${id}"]`);
  if (what === "next") el.querySelector(".wt-foot .btn:not(.ghost)").click();
  if (what === "prev") el.querySelector(".wt-foot .btn.ghost").click();
  if (what === "last") [...el.querySelectorAll(".wt-dot")].pop().click();
  const svg = document.getElementById(id);
  return {
    title: el.querySelector(".wt-title").textContent,
    count: el.querySelector(".wt-count").textContent,
    bodyLangs: [...el.querySelectorAll(".wt-body [lang]")]
      .filter(e => e.offsetParent !== null).map(e => e.getAttribute("lang")),
    prevOff: el.querySelector(".wt-foot .btn.ghost").disabled,
    nextOff: el.querySelector(".wt-foot .btn:not(.ghost)").disabled,
    dot: [...el.querySelectorAll(".wt-dot")].findIndex(d => d.getAttribute("aria-current") === "true"),
    dots: el.querySelectorAll(".wt-dot").length,
    lit: [...svg.querySelectorAll(".nes-focus")].map(n => n.textContent.trim().slice(0, 22)),
    hasFocus: svg.querySelector(".mermaid-view").classList.contains("has-focus"),
    folded: !document.getElementById(`${id}-steps`),
  };
};

for (const [page, ids] of PAGES) {
  for (const w of [390, 1440]) {
    // Viewport before navigation: the components on these pages choose a shape when they
    // upgrade and do not re-choose on resize. 390×664 at dpr 3 is Playwright's iPhone 14
    // preset, minus the touch emulation PinchTab has no way to supply.
    pt.viewport(w, w === 390 ? 664 : 900, { dpr: w === 390 ? 3 : 1, mobile: w === 390 });
    pt.nav(`${BASE}/${page}`);
    // <nes-walkthrough> reads its steps once at upgrade time, from a classic inline script.
    // The driver's nav already waits for `main` and for the network to go quiet; this covers
    // the gap between "the script ran" and "the component finished upgrading".
    pt.sleep(1200);
    for (const id of ids) {
      const act = what => pt.evalJson(step, id, what);

      const s1 = act("none");
      const s2 = act("next");
      pt.evalJson((id) => {
        document.querySelector(`nes-walkthrough[for="${id}"] .walkthrough`).focus();
        return true;
      }, id);
      pt.press("ArrowLeft");
      pt.sleep(150);
      const back = act("none");
      const last = act("last");
      const key = `${page}#${id}@${w}`;
      out[key] = { steps: s1.dots, count: s1.count, s1: s1.title, lit1: s1.lit,
                   s2: s2.title, lit2: s2.lit, last: last.title, litLast: last.lit,
                   langs: s1.bodyLangs };
      need(s1.folded, `${key}: source markup not folded away`);
      need(s1.prevOff === true, `${key}: PREV enabled on step 1`);
      need(s1.dot === 0 && s2.dot === 1, `${key}: dots do not follow (${s1.dot} → ${s2.dot})`);
      need(s2.title !== s1.title, `${key}: NEXT did not advance`);
      need(back.title === s1.title, `${key}: ArrowLeft did not go back`);
      need(last.nextOff === true, `${key}: NEXT enabled on the last step`);
      need(last.dot === s1.dots - 1, `${key}: last dot not current`);
      for (const [n, s] of [["1", s1], ["2", s2], ["last", last]]) {
        need(s.lit.length > 0, `${key} step ${n}: "${s.title}" lights no node`);
        need(s.hasFocus, `${key} step ${n}: .has-focus missing`);
        need(/^\d+ \/ \d+$/.test(s.count), `${key} step ${n}: counter is ${JSON.stringify(s.count)}`);
      }
      // One language visible in the body. The pages ship English and switch with a
      // CSS-only toggle, so a fresh load is English by definition.
      need(s1.bodyLangs.length > 0 && s1.bodyLangs.every(l => l === "en"),
        `${key}: body shows langs ${s1.bodyLangs}`);
    }
    errs.push(...pt.drain(`${page}@${w}`));
  }
}
console.log(JSON.stringify(out, null, 1));
if (errs.length) console.log("\npage errors:\n" + errs.join("\n"));
if (fails.length) console.log("\n" + fails.join("\n"));
console.log(fails.length || errs.length ? "\nWALKTHROUGHS: FAIL" : "\nWALKTHROUGHS: PASS");
// The browser instance belongs to the wrapper that started it; it stops it on the way out.
process.exit(fails.length || errs.length ? 1 : 0);
