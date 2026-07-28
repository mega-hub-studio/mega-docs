// Drives every diagram walkthrough on every page — prev/next, keyboard, dots, and which
// node lights up for which step.
//
//   ./scripts/check-walkthroughs.sh      renders the guide, serves it, drives it, cleans up
//
// It exists because `data-focus` matches node *text*: renaming a node in a .mmd file, or
// rewording a step, silently stops the highlight without breaking anything a test or a
// screenshot would notice. Here, a step that lights nothing is a failure.
//
// Buttons are clicked through the DOM rather than
// Playwright's actionability waits: the sticky header and the diagram box both overlap the
// stepper at some scroll positions, and the question here is whether the component works,
// not whether a tap lands.
import { chromium, devices } from "/opt/node22/lib/node_modules/playwright/index.mjs";
const BASE = process.argv[2] || "http://127.0.0.1:8123";
const b = await chromium.launch();
const out = {}, errs = [], fails = [];
const need = (c, why) => { if (!c) fails.push(why); };

for (const [page, ids] of [["index.html", ["hiw"]], ["ba.html", ["gaploop"]],
                           ["dev.html", ["pipe", "specloop", "loops"]]]) {
  for (const w of [390, 1440]) {
    const ctx = w === 390 ? await b.newContext(devices["iPhone 14"])
                          : await b.newContext({ viewport: { width: 1440, height: 900 } });
    const p = await ctx.newPage();
    p.setDefaultTimeout(8000);
    p.on("pageerror", e => errs.push(`${page}@${w}: ${e.message}`));
    await p.goto(`${BASE}/${page}`, { waitUntil: "domcontentloaded" });
    await p.waitForTimeout(1200);
    for (const id of ids) {
      const act = (id, what) => p.evaluate(([id, what]) => {
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
      }, [id, what]);

      const s1 = await act(id, "none");
      const s2 = await act(id, "next");
      await p.evaluate(id => document.querySelector(`nes-walkthrough[for="${id}"] .walkthrough`).focus(), id);
      await p.keyboard.press("ArrowLeft");
      await p.waitForTimeout(150);
      const back = await act(id, "none");
      const last = await act(id, "last");
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
    await ctx.close();
  }
}
console.log(JSON.stringify(out, null, 1));
if (errs.length) console.log("\npage errors:\n" + errs.join("\n"));
if (fails.length) console.log("\n" + fails.join("\n"));
console.log(fails.length || errs.length ? "\nWALKTHROUGHS: FAIL" : "\nWALKTHROUGHS: PASS");
await b.close();
