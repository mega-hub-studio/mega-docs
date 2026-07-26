// Renders web/*.mmd to web/*.svg, once, so mermaid never reaches a browser.
//
// Mermaid is ~800KB gzipped and 8bit-nes deliberately never bundles it. This
// diagram is fixed — it is not AI output and it does not stream — so paying that
// at runtime buys nothing. Instead it is rendered here and the SVG is committed:
// the page paints instantly, works with JS off, and still works air-gapped.
//
// The theme is not reimplemented. mermaidTheme() from the pinned elements.min.js
// reads the design tokens off the document, so the committed SVG is coloured by
// exactly the same function runtime mermaid would have used.
//
//   node scripts/gen-diagram.mjs            # render every web/*.mmd
//
// Needs mermaid available locally (installed on demand into .cache/, never a repo
// dependency) and the vendored 8bit-nes assets, so run `make vendor` first.
import { createHash } from "node:crypto";
import { createServer } from "node:http";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync, readdirSync, existsSync, mkdirSync } from "node:fs";
import { resolve, join, basename } from "node:path";

// Playwright is usually installed globally rather than in this repo (there is no
// package.json to put it in), so fall back to the global root instead of failing
// with a bare "Cannot find package".
const loadChromium = async () => {
  for (const spec of ["playwright", globalRoot() && join(globalRoot(), "playwright/index.mjs")]) {
    if (!spec) continue;
    try {
      return (await import(spec)).chromium;
    } catch {
      /* try the next one */
    }
  }
  throw new Error("playwright not found — `npm i -g playwright` (Chromium is already present)");
};
function globalRoot() {
  try {
    return execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim();
  } catch {
    return null;
  }
}

const WEB = resolve("web");
const CACHE = resolve(".cache/diagram");
const MERMAID_RANGE = "mermaid@11";

// The vendored tree carries the version, so find it rather than hardcoding it.
const vendorDir = () => {
  const root = join(WEB, "vendor");
  const hit = existsSync(root) && readdirSync(root).find((d) => d.startsWith("8bit-nes@"));
  if (!hit) throw new Error("no vendored 8bit-nes — run `make vendor` first");
  return join(root, hit);
};

// mermaid is a build-time tool, not a dependency of the product: keep it out of
// package.json and out of the repo, and fetch it into .cache on first use.
const mermaidBundle = () => {
  const bundle = join(CACHE, "node_modules/mermaid/dist/mermaid.min.js");
  if (existsSync(bundle)) return bundle;
  mkdirSync(CACHE, { recursive: true });
  writeFileSync(join(CACHE, "package.json"), '{"private":true}\n');
  console.log(`  installing ${MERMAID_RANGE} into .cache (build-time only)…`);
  execFileSync("npm", ["install", MERMAID_RANGE, "--no-audit", "--no-fund", "--silent"], {
    cwd: CACHE,
    stdio: "inherit",
  });
  if (!existsSync(bundle)) throw new Error("mermaid installed but dist/mermaid.min.js is missing");
  return bundle;
};

// Stamped into the SVG so `make check` can tell that a .mmd was edited without
// re-rendering — the one failure mode of committing generated output.
const stamp = (src) => createHash("sha256").update(src).digest("hex").slice(0, 16);

const sources = readdirSync(WEB).filter((f) => f.endsWith(".mmd"));
if (!sources.length) {
  console.log("gen-diagram: no web/*.mmd");
  process.exit(0);
}

const vendor = vendorDir();
const mermaidJs = readFileSync(mermaidBundle(), "utf8");

// The vendored tree is served over a real http origin rather than opened as
// file://, for two reasons: Chromium refuses ES module imports over file://, and
// serving it means the stylesheet's own url("./fonts/…") resolve — so mermaid
// measures its label boxes with the actual NES faces instead of a fallback.
const TYPES = { ".css": "text/css", ".js": "text/javascript", ".woff2": "font/woff2" };
const page$ = `<!doctype html><html><head><meta charset="utf-8">
<link rel="stylesheet" href="/all.min.css">
<script type="module">
  import { mermaidTheme } from "/elements.min.js";
  window.__nesTheme = mermaidTheme;
</script>
</head><body></body></html>`;

const server = createServer((req, res) => {
  const path = decodeURIComponent(new URL(req.url, "http://x").pathname);
  if (path === "/" || path === "/gen.html") {
    res.writeHead(200, { "Content-Type": "text/html" }).end(page$);
    return;
  }
  const file = join(vendor, path.replace(/^\/+/, ""));
  if (!file.startsWith(vendor) || !existsSync(file)) {
    res.writeHead(404).end("no");
    return;
  }
  const ext = file.slice(file.lastIndexOf("."));
  res.writeHead(200, { "Content-Type": TYPES[ext] || "application/octet-stream" });
  res.end(readFileSync(file));
});
await new Promise((ok) => server.listen(0, "127.0.0.1", ok));
const origin = `http://127.0.0.1:${server.address().port}`;

const chromium = await loadChromium();
const browser = await chromium.launch();
const page = await browser.newPage();
await page.goto(`${origin}/gen.html`);
await page.addScriptTag({ content: mermaidJs });
await page.waitForFunction(() => typeof window.__nesTheme === "function");
await page.evaluate(() => document.fonts.ready);

for (const file of sources) {
  const src = readFileSync(join(WEB, file), "utf8");
  // Strip %% comments but keep %%{init: …}%% — that one is mermaid config, not a
  // note to the reader, and dropping it silently loses the layout tuning.
  const code = src
    .split("\n")
    .filter((l) => !/^%%(?!\{)/.test(l))
    .join("\n")
    .trim();

  const name = basename(file, ".mmd");
  let { svg, fontSize } = await page.evaluate(
    async ([code, id]) => {
      // mermaidTheme() carries the font family but not a size, so mermaid falls back
      // to its own 16px — noticeably larger than --fs-body (13.5px), which made the
      // diagram's text the biggest text on the page and the diagram itself taller
      // than it needed to be. Measure the token and hand it over.
      const probe = document.createElement("div");
      probe.style.fontSize = "var(--fs-body)";
      document.body.appendChild(probe);
      const fontSize = parseFloat(getComputedStyle(probe).fontSize);
      probe.remove();

      // The size that ends up in the SVG's own <style> comes from themeVariables,
      // not the top-level config key, so set it there.
      const cfg = window.__nesTheme();
      window.mermaid.initialize({
        ...cfg,
        fontSize,
        themeVariables: { ...cfg.themeVariables, fontSize: `${fontSize}px` },
      });
      const out = await window.mermaid.render(id, code);
      return { svg: typeof out === "string" ? out : out.svg, fontSize };
    },
    [code, name],
  );

  // Mermaid scopes its own <style> by the svg id — "#howitworks .node rect{…}",
  // specificity 1-1-1 — which outranks the design system's spotlight rule
  // (".mermaid-view .nes-focus rect", 0-2-1). The dim-the-others half of the effect
  // works, because nothing else sets opacity, but the accent outline would never
  // land. So re-assert it here at the same specificity and later in the sheet, kept
  // inside the SVG so it cannot leak to anything else on the page.
  const spotlight =
    `#${name} .nes-focus > :is(rect,polygon,circle,path)` +
    `{stroke:var(--accent);stroke-width:3px;}`;
  const closed = svg.replace("</style>", spotlight + "</style>");
  if (closed === svg) throw new Error(`${name}: no <style> to extend — mermaid output changed`);
  svg = closed;
  const out = `<!-- generated from ${file} by scripts/gen-diagram.mjs — do not edit\n     mmd-sha256: ${stamp(src)} -->\n${svg}\n`;
  writeFileSync(join(WEB, `${name}.svg`), out);
  console.log(`  ✓ ${name}.svg (${(out.length / 1024).toFixed(1)} kB, text ${fontSize}px)`);
}

await browser.close();
server.close();
