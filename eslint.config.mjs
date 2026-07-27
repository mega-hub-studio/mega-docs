// Optional JS linter, run on demand: `make lint-js`.
//
// This file is tracked; the dependencies are not. There is no package.json and no
// node_modules in this repo, because the product needs neither — the front end is served
// as-is from the Go binary. `make lint-js` fetches eslint through `npx --yes` into npm's
// own cache, so the net exists when someone wants it and costs nothing when they don't.
//
// Why it is not part of `make check`: measured on this tree, @antfu/eslint-config pulls
// 255 packages and 113 MB to report three findings (two unused capturing groups and a
// `parseFloat` — all fixed), and its best rules, eslint-plugin-vue's reactivity-loss
// checks, do not fire at all without SFCs. The architecture rules that *do* catch this
// codebase's real mistakes are enforced for free in web/frontend_test.go. The day
// TypeScript or .vue files land here, promote this to the gate.
//
// Stylistic rules stay off on purpose. This repo uses semicolons and double quotes; a
// reformat of every file is a diff nobody can review for correctness, and style was never
// the thing going wrong.
import antfu from "@antfu/eslint-config";

export default antfu(
  {
    stylistic: false,
    formatters: false,
    typescript: false,
    vue: true,
    jsonc: false,
    yaml: false,
    markdown: false,
    ignores: ["web/vendor/**", "_site/**", "bin/**"],
  },
  {
    // Vue, marked and DOMPurify are globals from index.html: classic scripts, so the
    // pinned + integrity-checked CDN URLs live in exactly one file.
    languageOptions: { globals: { Vue: "readonly", marked: "readonly", DOMPurify: "readonly" } },
    rules: {
      // Documentation style, not facts: this repo's comments explain *why*, and a
      // "@returns description" rule rewards restating the type.
      "jsdoc/require-returns-description": "off",
      "jsdoc/no-multi-asterisks": "off",
      // Import order and template literals: both are formatting with a linter's voice.
      "perfectionist/sort-imports": "off",
      "prefer-template": "off",
    },
  },
);
