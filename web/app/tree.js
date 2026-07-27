/* ══ tree.js — the corpus tree, as a component ══════════════════════════════════
   Headless, like ba.js: a host element and a call. The nodes, the rebuild rules and the
   element's own quirks live in use/nestree.js — this file exists so the shell can write
   <corpus-tree :documents :scope @pick> and stop thinking about it.

   One folder of documents is one question's worth of context, and this is the control that
   says which: pick "booking/calendar" and the next answer is retrieved from that subtree
   only, cited from it, and cached under it.
   ═══════════════════════════════════════════════════════════════════════════ */
import { useNesTree } from "./use/nestree.js";

export const CorpusTree = {
  name: "CorpusTree",
  // Inline template: the element is built imperatively, so all this needs is somewhere to
  // put it. (The pinned Vue global build ships the compiler, so a string template costs no
  // build step.)
  template: `<div ref="host" class="tree-host"></div>`,
  props: {
    documents: { type: Array, default: () => [] },
    scope: { type: String, default: "" },
  },
  emits: ["pick"],

  setup(props, { emit }) {
    useNesTree({
      host: Vue.useTemplateRef("host"),
      documents: () => props.documents,
      scope: () => props.scope,
      onPick: (scope) => emit("pick", scope),
    });
    return {};
  },
};
