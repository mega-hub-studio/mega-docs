<script setup>
/* ══ BaScreen.vue — the BA screen ════════════════════════════════════════════════
   A contract: what comes in, what goes out, what it is made of. It owns no logic —
   anything with a branch lives in a composable: the gate (one password, two actions),
   the importer (files, progress, results), the tickets (four states, one path).

     props   writes · online · queue · documents   the ASK screen renders these too, so
             they belong to the shell rather than here
     emit    changed(ticket|null)  something moved: the shell refreshes queue, corpus and
             history, and updates the turn badge when a ticket came with it

   Going back to the chat is a <router-link>, not an emit: it is a change of address, and
   the shell stopped being the thing that knows which screen is showing.

   Everything visible is a design-system recipe — .segment, .stat, .card, .field,
   .textarea, .badge, .callout, .pbar, .datalist — so this screen owns no component
   styling of its own.
   ═══════════════════════════════════════════════════════════════════════════ */
import { toast } from '8bit-nes'
import { useGate } from '../composables/gate.js'
import { useTickets } from '../composables/tickets.js'
import ImportPanel from './ImportPanel.vue'
import LibraryPanel from './LibraryPanel.vue'
import TicketCard from './TicketCard.vue'

const props = defineProps({
  writes: Boolean, // does this instance allow a BA to publish at all
  online: Boolean, // unreachable is not read-only, and must not read as it
  queue: { type: Object, required: true }, // the ASK screen lists it too
  documents: { type: Array, default: () => [] },
})

const emit = defineEmits(['changed'])

const gate = useGate({ toast })
const { unlocked, passInput, unlocking, unlockError, unlock } = gate

const { drafts, working, editing, armed, arm, edit, cancel, move, remove } = useTickets({
  tickets: () => props.queue.tickets,
  toast,
  onMoved: ticket => emit('changed', ticket),
  onLocked: gate.fail,
})

// The importer is ImportPanel's own concern — it is the only thing that renders it, and
// a composable belongs to whoever shows its state. What comes back here is what the
// shell has to know about: a file landed, or the password was refused.
const importRefused = e => gate.fail(e, 'The server refused the password: ')
</script>

<template>
  <main class="ba">
    <!-- The pipeline, as counts rather than a decorative stage bar: a queue has tickets
         in every state at once, so there is no single "current" step to mark. These
         three numbers are the state, in the order work moves. -->
    <div class="stats">
      <div class="stat" data-accent="gold">
        <div class="n">{{ queue.open }}</div><div class="l">Open</div>
      </div>
      <div class="stat" data-accent="blue">
        <div class="n">{{ queue.answered }}</div><div class="l">Drafts</div>
      </div>
      <div class="stat" data-accent="good">
        <div class="n">{{ queue.confirmed }}</div><div class="l">In knowledge</div>
      </div>
    </div>

    <!-- Unreachable is not read-only. Both arrive as writes:false, and saying "set
         BA_PASS" to someone whose Wi-Fi dropped sends them to fix the server. -->
    <div v-if="!online" class="callout gotcha" role="alert">
      <b>Can't reach the server.</b> The queue below is the last state this page saw.
      Nothing can be published until it answers again.
    </div>

    <!-- Read-only instance: say so before an answer gets typed with nowhere to go. -->
    <div v-else-if="!writes" class="callout gotcha" role="alert">
      <b>This instance is read-only.</b> Set <code>BA_PASS</code> in <code>.env</code> and
      restart to answer tickets here. Until then the queue is visible but nothing can be
      published.
    </div>

    <!-- One line of orientation for a BA's first visit. The empty state covers the
         "nothing waiting" case, so this only shows when there is work. -->
    <div v-if="queue.tickets.length" class="callout memo">
      <b>A DEV files what the documents couldn't answer.</b> Your answer becomes a document
      in <code>qa/</code>, indexed and cited by name — so the next person who asks
      gets it, with a source. Publishing is reversible: correct it, take it back to a
      draft, or remove it, from the ticket itself.
    </div>

    <!-- The password is the permission. Reads are open; confirming an answer into the
         corpus is not, because this app has no accounts.
         Spelled out rather than chained to the callouts above: a v-else-if here would
         silently change meaning the next time something is inserted between them. -->
    <form
      v-if="online && writes && !unlocked" class="card" data-accent="gold"
      @submit.prevent="unlock"
    >
      <div class="head"><span class="title">Unlock writes</span></div>
      <label class="field">
        <span class="label">BA password</span>
        <input
          v-model="passInput" class="input" type="password"
          autocomplete="current-password" placeholder="BA_PASS"
        >
        <span class="hint">Kept for this tab only. Reading never needs it.</span>
      </label>
      <!-- The password is checked against the server before this form goes away, so a
           typo is answered here rather than surviving until the first upload and looking
           like a broken import. -->
      <div v-if="unlockError" class="callout crit" role="alert">{{ unlockError }}</div>
      <button class="btn" type="submit" :disabled="unlocking">
        {{ unlocking ? 'CHECKING…' : 'UNLOCK' }}
      </button>
    </form>

    <ImportPanel
      v-if="online && writes && unlocked" :documents="documents"
      @indexed="$emit('changed', null)" @locked="importRefused"
    />

    <!-- The library is shown to a locked screen too, because reading what is indexed needs
         no password (invariant 2) — `writes` is what decides whether it offers a way to
         change anything, so a read-only instance lists the documents and no buttons. -->
    <LibraryPanel
      v-if="online" :documents="documents" :writes="writes && unlocked"
      @changed="$emit('changed', null)" @locked="importRefused"
    />

    <div v-if="!queue.tickets.length" class="empty">
      <span class="icon">◈</span>
      <span class="title">Nothing waiting</span>
      <p>When the documents can't answer a question, the DEV who hit it files it here.</p>
      <!-- .btn is written for a <button>, and an <a> would arrive underlined, so the link
           renders none of its own element: only the navigation. -->
      <router-link v-slot="{ navigate }" to="/ask" custom>
        <button class="btn ghost sm" @click="navigate">ASK A QUESTION</button>
      </router-link>
    </div>

    <TicketCard
      v-for="t in queue.tickets" :key="t.id"
      :ticket="t" :unlocked="unlocked" :writes="writes" :working="working"
      :draft="drafts[t.id] ?? ''" :editing="editing" :armed="armed"
      @update:draft="drafts[t.id] = $event" @move="move(t, $event)"
      @edit="edit(t)" @cancel="cancel(t)" @arm="arm(t, $event)" @remove="remove(t)"
    />
  </main>
</template>
