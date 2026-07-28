// The browser driver for the two guide checks — the only file that knows a CLI is involved.
//
// It replaced Playwright, which had been reached through a hardcoded
// `/opt/node22/lib/node_modules/playwright/index.mjs`: a path that existed on one machine.
// When it did not exist the checks *skipped*, silently, and a skipped check reads exactly
// like a passing one. PinchTab is on PATH or it is not, and `pinchtab doctor` says which.
//
// Both checks measure rather than click: almost every question they ask is answered by one
// `eval` returning a plain object, so this exposes `evalJson` as the main verb and keeps
// the handful of real interactions (the language toggle, a keypress) next to it.
//
// Each run owns a browser instance of its own, started by the wrapper on its own port. That
// is not tidiness: PinchTab's commands act on the instance's current tab, so an editor or an
// MCP integration holding the same instance navigates the tab out from under a measurement —
// which reads as `500 resolve current tab url: context canceled` at a random point in the
// run. Measured: shared instance failed 2 of 3 runs, dedicated instance 0 of 3.
import { spawnSync } from 'node:child_process'

const BIN = process.env.PINCHTAB_BIN || 'pinchtab'
// A dedicated instance, set by the wrapper. Without it every command lands on whatever
// shared instance is running — and an editor or an MCP integration holding the same one is
// what turns these checks flaky: `500 resolve current tab url: context canceled`, from a tab
// somebody else navigated. The check owns its browser, the way `chromium.launch()` did.
const SERVER = process.env.PINCHTAB_SERVER || ''

/**
 * Open `bootUrl` and return the verbs the checks need.
 *
 * **Callers: set the viewport, then navigate — in that order.** `<nes-toc>` chooses its shape
 * (the open rail above 80rem, the collapsed bar below) when it upgrades and does not
 * re-choose on resize, so resizing a *loaded* page leaves the component in the shape it
 * picked for the old width and the check reads `rail: true` on a 390px phone. Measured both
 * ways to be sure of it.
 *
 * The boot navigation is not a convenience either: `set viewport` targets the instance's
 * *current tab*, and a fresh instance has none. Called first it answers `409 no current tab`
 * on stderr while the run carries on measuring at the default width — a phone assertion
 * passing at 1440px, which is the exact shape of the bugs this check exists to catch.
 */
export function open(bootUrl) {
  // No session and no agent id. PINCHTAB_SERVER points at an instance started for this run
  // and nobody else, so the anonymous current tab *is* ours — anything that scopes the tab
  // a second time only adds a way to look at the wrong one. Both were tried: a session needs
  // `sessions.agent.enabled` on the server and returns an empty id when it is off, and an
  // agent id makes "current tab" per-agent, which answers `409 no current tab` for every
  // command after a boot nav that created the anonymous one.
  //
  // process.env is passed through rather than a stripped object: the npm launcher resolves
  // its managed Go binary relative to HOME, and an empty environment fails with an
  // "npm rebuild pinchtab" that has nothing to do with the install.
  // The boot navigation creates the tab and, with --print-tab-id, names it — after which
  // every command carries `--tab`. That explicitness is the fix for the failure that took
  // two runs to place: the CLI remembers a *current tab* in its own state directory, so a
  // run that starts a fresh instance on the same port inherits the last run's tab id and
  // every command answers `404 tab … not found` (or `409 no current tab`) against something
  // that no longer exists. Ambient state is fine for a human at a prompt and wrong for a
  // check that has to be repeatable.
  //
  // Retried because this navigation is also what starts the browser, and a cold start loses
  // the race often enough to matter: `500 navigate: context canceled` once, then fine.
  let tab = ''
  for (let i = 0; ; i++) {
    try {
      tab = sh(['nav', bootUrl, '--print-tab-id'], process.env).trim().split(/\s+/).pop()
      if (tab)
        break
      throw new Error('pinchtab nav --print-tab-id printed no id')
    }
    catch (e) {
      if (i === 4)
        throw e
    }
  }
  const run = args => sh([...args, '--tab', tab], process.env)

  /**
   * Viewport, plus the mobile flags Playwright's device presets used to carry — and then
   * `innerWidth` is read back to prove it took.
   *
   * Verified rather than assumed because the failure is silent and wrong in the worst
   * direction: `set viewport` targets the session's current tab, answers `409 no current
   * tab` on stderr when the tab is still registering, and the run continues measuring at
   * the default 1280–1440. Every phone assertion then passes at laptop width, which is
   * exactly the shape of the two bugs this whole check exists to catch.
   */
  function viewport(w, h, { dpr = 1, mobile = false } = {}) {
    const args = ['set', 'viewport', String(w), String(h), '--dpr', String(dpr)]
    if (mobile)
      args.push('--mobile')
    for (let i = 0; i < 6; i++) {
      try {
        run(args)
        if (evalJson(() => innerWidth) === w)
          return
      }
      catch {
        // No tab yet, or an eval issued mid-settle. Both are answered by waiting.
      }
      run(['wait', '300'])
    }
    throw new Error(`pinchtab: viewport stayed at ${evalJson(() => innerWidth)}px, wanted ${w}px`)
  }

  /**
   * Run `fn` in the page and return its value, parsed.
   *
   * The function is serialised and applied to JSON-encoded arguments, so a check can be
   * written as ordinary JavaScript in its own file rather than as a string. Wrapped in an
   * IIFE because PinchTab evaluates in one shared realm: a top-level `const` would collide
   * with the next call.
   *
   * Retried on empty output, which is what an eval issued while the tab is still settling
   * returns — not an error, just nothing. Without this a whole page's measurements are
   * dropped at random and the run fails somewhere unrelated to the guide.
   */
  function evalJson(fn, ...args) {
    const expr = `(() => JSON.stringify((${fn}).apply(null, ${JSON.stringify(args)})))()`
    let out = ''
    for (let i = 0; i < 4; i++) {
      out = run(['eval', expr]).trim()
      if (out)
        break
      run(['wait', '400'])
    }
    try {
      return JSON.parse(out)
    }
    catch {
      throw new Error(`pinchtab eval did not return JSON: ${out.slice(0, 300) || '(empty)'}`)
    }
  }

  return {
    server: SERVER,
    viewport,
    evalJson,

    /**
     * Go to a page and wait for it to settle. `network-idle` rather than a fixed sleep:
     * these pages fetch the design system and three fonts, and every measurement below is
     * a layout measurement — taken mid-load it is a different page's numbers.
     */
    nav(url) {
      run(['nav', url])
      // `main` first, then quiet: the element proves the document parsed, and network-idle
      // proves the design system and its three fonts arrived. Measuring between the two
      // reports an unstyled page's geometry, which is how a layout check invents failures.
      for (const wait of [['wait', 'main'], ['wait', '--load', 'network-idle']]) {
        try {
          run(wait)
        }
        catch {
          // Neither is fatal on its own — the drain below is what reports a page that did
          // not load, and it says which request failed instead of "timed out".
        }
      }
    },

    click(selector) {
      run(['click', selector])
    },

    press(key) {
      run(['press', key])
    },

    /** Last resort, and used only where the component animates. */
    sleep(ms) {
      run(['wait', String(ms)])
    },

    /**
     * Everything the browser complained about since the last drain, then clear it — so a
     * message is attributed to the page that produced it instead of the whole run.
     *
     * This is the half of Playwright these checks actually used: console errors, uncaught
     * exceptions and failed requests. A guide page is static, so any of the three is a
     * defect rather than noise.
     */
    drain(label) {
      const out = []
      // `HH:MM:SS [LEVEL] message`. Only ERROR, matching what the Playwright version
      // reported (`m.type() === "error"`): the design system warns about things that are
      // its business, and a check that fails on those gets disabled.
      for (const line of run(['console', '--limit', '50']).split('\n')) {
        if (line.includes('[ERROR]'))
          out.push(`${label} console: ${line.trim()}`)
      }
      const errs = run(['errors']).trim()
      if (errs && !errs.startsWith('No errors'))
        out.push(`${label} pageerror: ${errs.split('\n')[0]}`)
      // Parsed as JSON rather than matched as text: a URL is part of every line, and
      // `nes-mono-400.woff2` contains "400". The first version of this reported every font
      // as a failed request.
      let net = { entries: [] }
      try {
        net = JSON.parse(run(['network', '--json', '--limit', '200']))
      }
      catch {
        // No capture buffer for this tab yet — nothing loaded, which the caller's own
        // measurements will report far more clearly than a parse error here.
      }
      for (const e of net.entries || []) {
        // The pages are served from a local directory, so a miss is a broken href or a
        // vendored asset that was never fetched. status 0 is a request that never answered.
        if (!e.status || e.status >= 400)
          out.push(`${label} request: ${e.status || 'failed'} ${e.url}`)
      }
      run(['console', '--clear'])
      run(['network', '--clear'])
      return out
    },
  }
}

// The one line PinchTab writes on every single command: a suggestion to create a session,
// which this driver does not use on purpose (see open() — the instance is ours alone, and a
// session scopes "current tab" a second way). Twenty identical copies at the end of a green
// run is a gate that shouts, so it is dropped by name rather than by swallowing stderr —
// everything else it says is a diagnostic and still prints.
const NOISE = /^HINT: no session set/

/**
 * Run the CLI and return its stdout.
 *
 * spawnSync rather than execFileSync because stderr has to be *read* on the way past, and
 * execFileSync only hands it over when the command fails. It fails rarely: PinchTab reports
 * `Error 500: …` on stderr and still exits 0, so an inherited stderr was the only place a
 * real failure ever showed — which is why it is filtered and re-emitted here rather than
 * captured into an error nobody sees. The exit-code path keeps the text too: `e.stderr` was
 * always null under the old default, so a genuine non-zero said `Command failed` and nothing
 * about why.
 */
function sh(args, env) {
  // --server is a global flag, so it goes before the subcommand.
  const argv = SERVER ? ['--server', SERVER, ...args] : args
  const r = spawnSync(BIN, argv, { env, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 })
  const said = (r.stderr || '').split('\n').filter(l => l.trim() && !NOISE.test(l)).join('\n')
  if (said)
    process.stderr.write(`${said}\n`)
  if (r.error || r.status !== 0)
    throw new Error(`pinchtab ${args[0]} failed: ${said || r.error?.message || `exit ${r.status}`}`)
  return r.stdout
}
