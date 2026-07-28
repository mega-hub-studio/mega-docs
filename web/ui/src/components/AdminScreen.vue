<script setup>
/* ══ AdminScreen.vue — what this instance actually is ════════════════════════════
   A contract, and a short one: three tabs, and not a single control that changes
   anything. Every knob is read once at startup, so the thing an operator could not do
   before this screen was *see* one — the effective value lived in .env, in
   internal/config's defaults, and in whatever the shell already had, and which of the
   three won was a guess. The `source` column is that answer.

   Documents are never touched from here. Import, remove, confirm and dismiss stay in
   BaScreen, where the person doing that work already is; two screens with the same button
   is one button and one lie about which of them matters.

   The tabs are `<nes-tabs>`, which owns its own selection — so this file holds no tab
   state and the composable holds none either. Runtime and Usage arrive as props because
   the shell already fetched them for the status line and the queue; only the settings list
   is this screen's own request.
   ═══════════════════════════════════════════════════════════════════════════ */
import { toast } from '8bit-nes'
import { onMounted } from 'vue'
import { useAdmin } from '../composables/admin.js'

// The props are read by the template only — Runtime and Usage render straight from what the
// shell already fetched — so there is nothing for the script to bind them to.
defineProps({
  online: Boolean,
  writes: Boolean,
  runtime: { type: Object, required: true }, // model + window + prices, from /api/health
  corpus: { type: Object, required: true }, // docs · chunks · approved, from /api/corpus
  history: { type: Array, default: () => [] }, // answers still free to replay
})

const {
  unlocked,
  passInput,
  unlocking,
  unlockError,
  absent,
  grouped,
  fetchSettings,
  unlock,
} = useAdmin({ toast })

// A stored password from an earlier tab load means the list can be fetched without asking
// again — and the same call is what discovers an instance with no admin surface at all.
onMounted(fetchSettings)
</script>

<template>
  <main class="ba admin">
    <!-- No ADMIN_PASS on this instance. Not a locked door — there is no password that
         would open it, so offering a form would be inviting a retry that cannot work. -->
    <div v-if="absent" class="callout gotcha" role="alert">
      <b>This instance has no admin screen.</b> Set <code>ADMIN_PASS</code> in
      <code>.env</code> and restart. Until then the route is not registered, the same way an
      unset <code>BA_PASS</code> removes the write surface instead of opening it.
    </div>

    <form v-else-if="!unlocked" class="card" data-accent="purple" @submit.prevent="unlock">
      <div class="head"><span class="title">Unlock admin</span></div>
      <label class="field">
        <span class="label">Admin password</span>
        <input
          v-model="passInput" class="input" type="password"
          autocomplete="current-password" placeholder="ADMIN_PASS"
        >
        <span class="hint">
          Its own password, not the BA one: this screen reports which secrets exist.
          Kept for this tab only.
        </span>
      </label>
      <div v-if="unlockError" class="callout crit" role="alert">{{ unlockError }}</div>
      <button class="btn" type="submit" :disabled="unlocking">
        {{ unlocking ? 'CHECKING…' : 'UNLOCK' }}
      </button>
    </form>

    <nes-tabs v-else>
      <section data-label="Settings" selected>
        <!-- Said once, at the top, rather than badged onto all nineteen rows: they are all
             read at startup, so "restart to change" is a property of the screen, not of a
             row. EMBED_DIM is the one that costs more than a restart. -->
        <div class="callout memo">
          <b>Everything here is read at startup.</b> Changing one means editing
          <code>.env</code> and restarting. <code>EMBED_DIM</code> costs more than that —
          a different width means re-embedding the whole corpus.
        </div>

        <section v-for="g in grouped" :key="g.name">
          <h2 class="eyebrow">{{ g.name }}</h2>
          <dl class="datalist">
            <template v-for="s in g.rows" :key="s.name">
              <dt><code>{{ s.name }}</code></dt>
              <dd>
                <span :class="s.secret ? 'badge clear' : 'val'">{{ s.value }}</span>
                <!-- The column this screen exists for. `.env` and the shell are
                     indistinguishable to os.Getenv once the file is loaded, so the server
                     records which keys the file supplied and reports it here. -->
                <span class="badge" :class="s.source === 'default' ? 'todo' : 'good'">
                  {{ s.source }}</span>
              </dd>
            </template>
          </dl>
        </section>
      </section>

      <section data-label="Runtime">
        <div class="stats">
          <div class="stat" data-accent="blue">
            <div class="n">{{ corpus.docs }}</div><div class="l">Documents</div>
          </div>
          <div class="stat" data-accent="blue">
            <div class="n">{{ corpus.chunks }}</div><div class="l">Sections</div>
          </div>
          <div class="stat" data-accent="good">
            <div class="n">{{ corpus.approved }}</div><div class="l">BA-confirmed</div>
          </div>
        </div>
        <dl class="datalist">
          <dt>Chat model</dt><dd>{{ runtime.model }}</dd>
          <dt>Context window</dt><dd>{{ runtime.window }} tokens</dd>
          <dt>Price in / out</dt>
          <dd>{{ runtime.priceIn }} / {{ runtime.priceOut }} per 1M tokens</dd>
          <dt>Server</dt>
          <dd>
            <span class="badge" :class="online ? 'good' : 'crit'">
              {{ online ? 'reachable' : 'unreachable' }}</span>
          </dd>
          <dt>Writes</dt>
          <dd>
            <span class="badge" :class="writes ? 'good' : 'todo'">
              {{ writes ? 'BA can publish' : 'read-only' }}</span>
          </dd>
        </dl>
      </section>

      <section data-label="Usage">
        <p>
          What has been asked and is still free to replay — usage of the corpus, not of
          people. An instance with no accounts cannot report the second and should not
          pretend to.
        </p>
        <div v-if="history.length" class="table-wrap">
          <table class="table">
            <thead>
              <tr><th>Question</th><th>Hits</th><th>Scope</th></tr>
            </thead>
            <tbody>
              <tr v-for="h in history" :key="h.question + h.scope">
                <td>{{ h.question }}</td>
                <td>{{ h.hits }}</td>
                <td><code>{{ h.scope || 'whole corpus' }}</code></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty">
          <span class="icon">◈</span>
          <span class="title">Nothing cached yet</span>
          <p>A question answered twice is answered from here the second time, free.</p>
        </div>
      </section>
    </nes-tabs>
  </main>
</template>
