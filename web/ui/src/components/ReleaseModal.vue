<script setup>
/* ══ ReleaseModal.vue — what changed, behind the version badge ════════════════════
   A contract: four props, no emits, no logic. `useRelease` decided every branch — the
   fetch, the grouping and the order — so this file is a v-for over facts.

   It renders the *body* of the dialog, not the dialog. The `<dialog class="modal">` lives in
   App.vue for the same reason the diagram viewer's does: the shell owns dialogs, so the
   composable that calls showModal() binds a real element rather than reaching through a
   component instance for its root node. That keeps `useRelease({ dialog })` symmetric with
   `useDiagrams({ zoom, zoomBody })`, and it is why there is no ref in this file.
   ═══════════════════════════════════════════════════════════════════════════ */
import { useT } from '../composables/lang.js'

defineProps({
  groups: { type: Array, required: true }, // [{ kind, notes }], already ordered
  // The releases behind this one, each already grouped by the same function: [{ version,
  // date, groups }]. Closed by default — what a reader came for is the version they are on,
  // and eight releases of history opened on a phone buries it.
  past: { type: Array, default: () => [] },
  meta: { type: Object, required: true }, // version · date · previous
  loading: Boolean,
  error: { type: String, default: '' },
})

// composables/lang.js is the only door to i18n — never `$t`, which would bypass the pinned
// global scope that file exists to guarantee.
const { t } = useT()
</script>

<template>
  <div class="rel-body">
    <!-- The identity of what is being described, before the list of what it contains: the
         tag, the day it was cut, and what it is measured against. `previous` is empty for a
         first release, and then it says so rather than printing "since ". -->
    <p class="rel-meta">
      <span class="badge" data-accent="good">{{ meta.version }}</span>
      <span v-if="meta.date" class="rel-date">{{ meta.date }}</span>
      <span class="rel-since">{{
        meta.previous ? t('release.since', { v: meta.previous }) : t('release.first')
      }}</span>
    </p>

    <p v-if="loading" class="rel-state">
      <span class="spinner" aria-hidden="true" />{{ t('release.loading') }}
    </p>
    <p v-else-if="error" class="rel-state err">{{ t('release.failed', { err: error }) }}</p>
    <p v-else-if="!groups.length" class="rel-state">
      {{ t('release.empty', { file: 'web/release.json' }) }}
    </p>

    <section v-for="g in groups" :key="g.kind" class="rel-group">
      <h3 class="eyebrow">{{ t(`release.kind.${g.kind}`) }}</h3>
      <ul class="rel-list">
        <li v-for="n in g.notes" :key="n.commit">
          <!-- The scope first when there is one: it is what tells a reader whether a line
               is about the part they care about, and most subjects do not repeat it. -->
          <b v-if="n.scope" class="rel-scope">{{ n.scope }}</b>
          <span class="rel-subject">{{ n.subject }}</span>
          <!-- The sha is the line's provenance, and the reason no prose copy of this exists:
               every entry can be traced to the commit it was generated from. -->
          <code class="rel-commit">{{ n.commit }}</code>
        </li>
      </ul>
    </section>

    <!-- What changed before this version. `<details>` because the disclosure is the
         platform's — a keyboard-reachable summary, the open/closed state, and the arrow all
         come free, and a component holding a `shown` ref would be state the shell does not
         own. One per release rather than one for all of them: a reader who was away for two
         versions opens two, not a wall. -->
    <details v-for="r in past" :key="r.version" class="rel-past">
      <summary>
        <span class="badge">{{ r.version }}</span>
        <span v-if="r.date" class="rel-date">{{ r.date }}</span>
      </summary>
      <section v-for="g in r.groups" :key="g.kind" class="rel-group">
        <h3 class="eyebrow">{{ t(`release.kind.${g.kind}`) }}</h3>
        <ul class="rel-list">
          <li v-for="n in g.notes" :key="n.commit">
            <b v-if="n.scope" class="rel-scope">{{ n.scope }}</b>
            <span class="rel-subject">{{ n.subject }}</span>
            <code class="rel-commit">{{ n.commit }}</code>
          </li>
        </ul>
      </section>
    </details>
  </div>
</template>
