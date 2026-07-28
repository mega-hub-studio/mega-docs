// ESLint for web/ui, from @antfu/eslint-config — measured before it was adopted, and
// adopted when the measurement changed.
//
// When this front end was in-DOM templates plus native modules, this config pulled 255
// packages to report three cosmetic findings, and its best rules (eslint-plugin-vue's)
// did not fire at all without SFCs. The trigger written down at the time was "the day
// TypeScript or .vue files land here". The .vue files landed, so it runs in `make check`
// now, and it earns that place with exactly one rule above all others:
//
//   vue/no-undef-properties — a template that binds a name nothing behind it defines.
//   In Vue that renders blank, with no error and no warning in production. It used to be
//   approximated by a Go regex over module text; this reads a real parse of a real SFC.
//
// That rule is also why oxlint is *not* used here, which is worth writing down because
// oxlint is otherwise the obvious upgrade (Rust, ~50x faster). Measured against oxlint
// 1.76.0: its `vue` plugin has no `no-undef-properties`, no `no-undef-components` and no
// `require-explicit-emits` — the three enforcers of CLAUDE.md rules 11 and 12. A linter
// that cannot run the gate's reason for existing is not a replacement, and running both
// would mean two toolchains and two configs for rules this one already covers, on a tree
// of 28 files where the whole lint takes about two seconds.
//
// TypeScript stays off: one maintainer, and the type surface here is three API shapes
// already described in web/spec.json.
import antfu from '@antfu/eslint-config'

export default antfu({
  vue: true,
  typescript: false,
  // Formatting is enforced now. Two things changed: the tree is small enough that one
  // mechanical reformat is a reviewable commit of its own, and an editor saving a file
  // should not be able to produce a diff. `.editorconfig` carries the same values to
  // editors so they agree before ESLint is ever asked.
  //
  // This is eslint-stylistic, not Prettier: already installed, and it reflows *nothing* —
  // only semicolons, quotes and trailing commas change, so the diff stays reviewable.
  // Prettier and oxfmt were both considered and skipped for that reason: either adds a
  // second config whose printWidth would rewrap every hand-wrapped comment block in the
  // tree, and both would then have to be kept in agreement with this file.
  stylistic: {
    indent: 2,
    quotes: 'single',
    semi: false,
  },
  // Still off: this is the layer that runs Prettier *through* ESLint for css/html/md. The
  // docs pages are Go templates with `<% %>` delimiters, which no HTML formatter parses,
  // and web/*.svg is generated.
  formatters: false,
  ignores: ['dist/**', 'node_modules/**'],
}, {
  rules: {
    // The reason this config is in the gate.
    'vue/no-undef-properties': 'error',
    // A component that reads a prop it never declared is the same class of bug seen from
    // the other side.
    'vue/require-explicit-emits': 'error',
    // Three families of tag this rule cannot see a definition for, and all three are
    // real: `nes-*` are the design system's custom elements, and `i18n-t` and `router-*`
    // are vue-i18n's and vue-router's own components, registered globally by `app.use()`
    // in main.js. Everything else that is undefined is the bug this rule exists to catch.
    'vue/no-undef-components': ['error', { ignorePatterns: ['nes-.*', 'i18n-t', 'router-.*'] }],
  },
}, {
  files: ['**/*.vue'],
  rules: {
    // The design system's elements carry hyphens and its own event names carry colons;
    // neither is a Vue naming mistake.
    'vue/attribute-hyphenation': 'off',
    'vue/v-on-event-hyphenation': 'off',
    // One order for every template, so a diff never argues about where an attribute goes.
    // eslint-plugin-vue's default order is already what a hand-written template here
    // converges on — structure first (v-for, v-if), then identity (id, ref, key), then
    // two-way binding, then plain attributes, then events, and CONTENT (v-html / v-text)
    // last, which is exactly what the comment beside the one `v-html` in this tree relies
    // on. It is also what the attribute-ordering plugin in the sibling project's Prettier
    // setup spells out by hand, so this gets the same result with no new dependency.
    'vue/attributes-order': 'error',
    // Left off: both rewrap markup for taste rather than correctness, and a template
    // reflowed to one-attribute-per-line is a diff nobody can review against the design
    // system's own examples.
    'vue/singleline-html-element-content-newline': 'off',
    'vue/max-attributes-per-line': 'off',
  },
}, {
  rules: {
    // ── Cherry-picked correctness, not a category sweep ──
    // antfu already turns on 221 rules on a .js file here, including every `no-eval`-family
    // ban, eqeqeq, prefer-const, no-console, and 36 of the rules a stricter shared config
    // would list. What follows is only what is genuinely absent *and* can catch something
    // in this codebase — each line earns its place or it is not here.
    //
    // Rejected from that same list, with the reason, so it is not re-litigated: the whole
    // `typescript/*` family (21 rules, inert without TS — and six of them are inert even in
    // a TS project unless it runs `--type-aware`); `oxc/*` (an oxlint-only plugin); every
    // `no-restricted-imports` ban on luxon / lodash / dayjs / vue-i18n (none installed);
    // and `@/`-alias enforcement (this tree imports by relative path on purpose, is four
    // layers deep at most, and web/frontend_test.go reads those exact paths to enforce the
    // layer rules).
    'no-else-return': 'error', // one less level of nesting, and the early return is the point
    'no-return-assign': 'error', // `return x = y` is a typo more often than an intent
    'unicorn/prefer-array-find': 'error', // .filter()[0] scans the whole array to use one item
    'unicorn/prefer-array-flat-map': 'error', // .map().flat() allocates twice
    'unicorn/prefer-date-now': 'error', // new Date().getTime() → Date.now()
    'unicorn/prefer-math-min-max': 'error', // the ternary form is where an operator gets flipped
    'unicorn/prefer-regexp-test': 'error', // .match() builds a result object to answer a boolean
    'unicorn/prefer-structured-clone': 'error', // a JSON round-trip silently drops undefined/Date
    'unicorn/no-await-in-promise-methods': 'error', // await inside Promise.all defeats it
    // Not addable without a new dependency, and not worth one: antfu 6 ships a *lite*
    // import plugin (five rules — `no-self-import` and `newline-after-import` are not in
    // it) and loads no promise plugin at all, so `import/no-self-import`,
    // `import/newline-after-import` and `promise/param-names` would each cost a package.
    // `unicorn/error-message` and `import/no-duplicates` are already on, so they are not
    // repeated here.
    // Deliberately not taken from that list either: `unicorn/explicit-length-check` and
    // `unicorn/prefer-ternary` (taste), `unicorn/no-negated-condition` (this tree's guard
    // clauses read better negated), `unicorn/prefer-set-has` (no hot loop over an array
    // literal here), `sort-imports` (antfu's `import/order` already sorts), and
    // `import/no-default-export` (every .vue file is a default export by contract).

    // This repo's JSDoc is prose: it says why a function exists and what breaks without
    // it, in the same voice as the Go comments next door. A required "@returns
    // <description>" on top of a typed @returns is boilerplate, and the multi-asterisk
    // rule fights the ══ banner headers every file in this tree opens with.
    'jsdoc/require-returns-description': 'off',
    'jsdoc/no-multi-asterisks': 'off',
    // package.json key order is npm's business, not a correctness question.
    'jsonc/sort-keys': 'off',
    // The JS half of CLAUDE.md rule 24, and the counterpart of `godox` on the Go side. A
    // deferred-work marker in a bundle is debt with no owner and no date: this tree has
    // none, so the rule is free today and stays that way. What such a note would have said
    // belongs in `changelog/`, dated and beside the decision — or it gets done in the same
    // change. (The banned words live in `terms` below, never in a comment, or this rule
    // reports its own configuration — which is exactly what happened when it was written.)
    'no-warning-comments': ['error', { location: 'anywhere', terms: ['todo', 'fixme', 'xxx', 'hack'] }],
  },
})
