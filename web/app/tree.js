/* ══ tree.js — the corpus as a tree, and the scope picked from it ══════════════
   One folder of documents is one question's worth of context. This is the control
   that says which: pick "booking/calendar" and the next answer is retrieved from
   that subtree only, cited from it, and cached under it.

   `<nes-tree>` is the design system's own element — expand/collapse, single
   selection, full keyboard nav, ARIA tree semantics. It takes its data as a child
   JSON script and builds its DOM once, in connectedCallback, so this component
   *replaces* the element when the corpus changes rather than mutating it. That is
   the whole reason the markup is created here instead of in index.html.
   ═══════════════════════════════════════════════════════════════════════════ */

/** @typedef {{ path: string, title: string, chunks: number }} Doc */

/**
 * Build `<nes-tree>` nodes from flat document paths.
 *
 * A folder's value is its own path, so selecting it scopes to everything beneath —
 * the same string the server filters on. Only the first level starts expanded: a
 * corpus of a few hundred documents opened all at once is a wall, and the level
 * above is the scope most people want anyway. The exception is the path down to the
 * active scope, which is opened so a restored scope is visible where it lives rather
 * than highlighted inside a collapsed branch.
 *
 * @param {Doc[]} documents
 * @param {string} [scope] the active scope, whose ancestors start expanded
 * @returns {Array<object>} nodes for the element's JSON payload
 */
export function treeNodes(documents, scope = "") {
  const root = { children: new Map() };
  for (const d of documents || []) {
    const segs = String(d.path || "").split("/").filter(Boolean);
    if (!segs.length) continue;
    let node = root;
    segs.forEach((seg, i) => {
      const path = segs.slice(0, i + 1).join("/");
      if (!node.children.has(seg)) {
        node.children.set(seg, { label: seg, value: path, children: new Map() });
      }
      node = node.children.get(seg);
      // The leaf carries the count: a document with no retrievable sections is a
      // failed ingest, and a reader deserves to see that before asking it.
      if (i === segs.length - 1) node.chunks = d.chunks || 0;
    });
  }

  const onScopePath = (value) => scope === value || scope.startsWith(value + "/");
  const out = (node, level) =>
    [...node.children.values()].map((n) => {
      const kids = out(n, level + 1);
      return kids.length
        ? {
            label: n.label,
            value: n.value,
            expanded: level === 1 || onScopePath(n.value),
            children: kids,
          }
        : { label: `${n.label} · ${n.chunks ?? 0}`, value: n.value };
    });
  return out(root, 1);
}

export const CorpusTree = {
  name: "CorpusTree",
  // Inline template: the element and its JSON are built imperatively below, so all
  // this needs is somewhere to put them. (The pinned Vue global build ships the
  // compiler, so a string template costs no build step.)
  template: `<div ref="host" class="tree-host"></div>`,
  props: {
    documents: { type: Array, default: () => [] },
    scope: { type: String, default: "" },
  },
  emits: ["pick"],

  mounted() {
    this.build();
  },

  watch: {
    // The corpus changes when a BA imports or confirms. Rebuilding is the only
    // option — the element ignores a mutated JSON payload — and it is cheap.
    documents() {
      this.build();
    },
    // Reflect a scope cleared from elsewhere (the bar above the prompt). Only a
    // *different* value rebuilds: rebuilding on the value the tree itself just
    // emitted would drop the keyboard position mid-interaction.
    scope(now) {
      if (now !== (this.$refs.host?.firstElementChild?.value ?? "")) this.build();
    },
  },

  methods: {
    build() {
      const host = this.$refs.host;
      if (!host) return;
      const tree = document.createElement("nes-tree");
      tree.setAttribute("aria-label", "Indexed documents");
      if (this.scope) tree.setAttribute("value", this.scope);
      const data = document.createElement("script");
      data.type = "application/json";
      data.textContent = JSON.stringify(treeNodes(this.documents, this.scope));
      // Both children before insertion: the element reads its JSON in
      // connectedCallback, and an empty tree is what it would find otherwise.
      tree.appendChild(data);
      tree.addEventListener("nes:change", (e) => this.$emit("pick", e.detail.value || ""));
      host.replaceChildren(tree);
    },
  },
};
