/* ══ use/nestree.js — driving <nes-tree>, and shaping the corpus for it ════════
   The behaviour behind the corpus tree: turn flat document paths into nodes, and keep the
   element in sync with them.

   The element is the design system's own — expand/collapse, single selection, full
   keyboard nav, ARIA tree semantics — and it builds its DOM once, in connectedCallback,
   from a child JSON script. So "keeping it in sync" means *replacing* it, which is why
   this is imperative code behind a composable rather than a template.
   ═══════════════════════════════════════════════════════════════════════════ */

/** @typedef {{ path: string, title: string, chunks: number }} Doc */

/**
 * Build `<nes-tree>` nodes from flat document paths.
 *
 * A folder's value is its own path, so selecting it scopes to everything beneath — the
 * same string the server filters on. Only the first level starts expanded: a corpus of a
 * few hundred documents opened at once is a wall, and the level above is the scope most
 * people want anyway. The exception is the path down to the active scope, which is opened
 * so a restored scope is visible where it lives rather than highlighted inside a collapsed
 * branch.
 *
 * @param {Doc[]} documents
 * @param {string} [scope] the active scope, whose ancestors start expanded
 * @returns {Array<object>} nodes for the element's JSON payload
 */
export function treeNodes(documents, scope = "") {
  const root = { children: new Map() };
  for (const d of documents || []) {
    const segs = String(d.path || "")
      .split("/")
      .filter(Boolean);
    if (!segs.length) continue;
    let node = root;
    segs.forEach((seg, i) => {
      const path = segs.slice(0, i + 1).join("/");
      if (!node.children.has(seg)) {
        node.children.set(seg, { label: seg, value: path, children: new Map() });
      }
      node = node.children.get(seg);
      // The leaf carries the count: a document with no retrievable sections is a failed
      // ingest, and a reader deserves to see that before asking it.
      if (i === segs.length - 1) node.chunks = d.chunks || 0;
    });
  }

  const onScopePath = (value) => scope === value || scope.startsWith(value + "/");
  const out = (node, level) =>
    [...node.children.values()].map((n) => {
      const kids = out(n, level + 1);
      return kids.length
        ? { label: n.label, value: n.value, expanded: level === 1 || onScopePath(n.value), children: kids }
        : { label: `${n.label} · ${n.chunks ?? 0}`, value: n.value };
    });
  return out(root, 1);
}

/**
 * Mount a `<nes-tree>` inside host, and rebuild it when the corpus or the scope changes.
 *
 * @param {{ host: import("vue").Ref<Element>, documents: () => object[],
 *   scope: () => string, onPick: (scope: string) => void }} deps
 */
export function useNesTree({ host, documents, scope, onPick }) {
  const { onMounted, watch } = Vue;

  function build() {
    if (!host.value) return;
    const tree = document.createElement("nes-tree");
    tree.setAttribute("aria-label", "Indexed documents");
    if (scope()) tree.setAttribute("value", scope());
    const data = document.createElement("script");
    data.type = "application/json";
    data.textContent = JSON.stringify(treeNodes(documents(), scope()));
    // Both children before insertion: the element reads its JSON in connectedCallback,
    // and an empty tree is what it would find otherwise.
    tree.appendChild(data);
    tree.addEventListener("nes:change", (e) => onPick(e.detail.value || ""));
    host.value.replaceChildren(tree);
  }

  onMounted(build);

  // The corpus changes when a BA imports or confirms. Rebuilding is the only option — the
  // element ignores a mutated JSON payload — and it is cheap.
  watch(documents, build);

  // Reflect a scope cleared from elsewhere (the picker above the prompt). Only a
  // *different* value rebuilds: rebuilding on the value the tree itself just emitted would
  // drop the keyboard position mid-interaction.
  watch(scope, (now) => {
    if (now !== (host.value?.firstElementChild?.value ?? "")) build();
  });
}
