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
// TypeScript stays off: one maintainer, and the type surface here is three API shapes
// already described in web/spec.json. Stylistic rules stay off too — this tree uses
// semicolons and double quotes, and a reformat of every file is a diff nobody can review
// for correctness.
import antfu from "@antfu/eslint-config";

export default antfu({
  vue: true,
  typescript: false,
  // The formatter layer would rewrite every file; the repo's Go side is gofmt-clean and
  // its JS side is consistent by hand. Correctness rules only.
  stylistic: false,
  formatters: false,
  ignores: ["dist/**", "node_modules/**"],
}, {
  rules: {
    // The reason this config is in the gate.
    "vue/no-undef-properties": "error",
    // A component that reads a prop it never declared is the same class of bug seen from
    // the other side.
    "vue/require-explicit-emits": "error",
    // Custom elements from the design system are not unknown components.
    "vue/no-undef-components": ["error", { ignorePatterns: ["nes-.*"] }],
  },
}, {
  files: ["**/*.vue"],
  rules: {
    // The design system's elements carry hyphens and its own event names carry colons;
    // neither is a Vue naming mistake.
    "vue/attribute-hyphenation": "off",
    "vue/v-on-event-hyphenation": "off",
    // The formatting rules eslint-plugin-vue applies regardless of `stylistic: false`.
    // Same reason gofumpt is off on the Go side: they would rewrite every template for
    // taste, and a reformat of every file is a diff nobody can review for correctness.
    // `v-html` deliberately sits last in the attribute list it appears in, next to the
    // comment explaining why it is safe.
    "vue/singleline-html-element-content-newline": "off",
    "vue/attributes-order": "off",
    "vue/max-attributes-per-line": "off",
  },
}, {
  rules: {
    // This repo's JSDoc is prose: it says why a function exists and what breaks without
    // it, in the same voice as the Go comments next door. A required "@returns
    // <description>" on top of a typed @returns is boilerplate, and the multi-asterisk
    // rule fights the ══ banner headers every file in this tree opens with.
    "jsdoc/require-returns-description": "off",
    "jsdoc/no-multi-asterisks": "off",
    // package.json key order is npm's business, not a correctness question.
    "jsonc/sort-keys": "off",
  },
});
