<script setup>
import { useTemplateRef } from 'vue'

/* ══ SettingsDrawer.vue — one gear, one panel, almost no words ═══════════════════
   A contract, like every other screen part: what comes in, what goes out, and a template
   over nothing at all — the state belongs to the shell, because the model rides on every
   question `useConversation` sends.

     props   models · picked · current · muted · lang · langs · admin · recall
     emit    pick(name) · mute(bool) · setLang(code) · close

   ── why it is icons and not sentences ──
   The first version read as three paragraphs: "Rides on every question. This instance refuses
   any other." · "Chưa cấu hình giá" · "Bắt đầu từ câu hỏi đầu tiên của bạn". In Vietnamese
   those wrapped to three lines each against a mono uppercase label as wide as the value
   column, so a panel of six facts was a wall of prose with a ragged right edge — and every
   line of it had to be *read* to find the one control anybody came for.
   Now each row is an icon and a value on one grid: one column for the glyph, one for the
   number, so the eye reads down two straight edges and the words are in `title` for whoever
   wants them. Unknown is `—`, not a sentence about being unknown.

   Every part is the library's: `.drawer` on a native <dialog> (Escape, focus trap and the
   backdrop are the platform's), `.select`, `.segment`, `.switch`, `.meter` — 0.14.0's cell bar,
   which is how the thread's depth became something to glance at rather than parse — plus
   `nes-icon` for every label. `styles.css` adds the two-column grid and nothing else.
   ═══════════════════════════════════════════════════════════════════════════ */
defineProps({
  models: { type: Array, default: () => [] },
  picked: { type: String, default: '' },
  // The picked model's own window and price, already resolved by the composable — zero in
  // either means the operator never said, and then the row shows a dash rather than a zero.
  current: { type: Object, default: () => ({}) },
  muted: Boolean,
  lang: { type: String, required: true },
  langs: { type: Array, required: true },
  // Whether this instance has an admin surface at all. False leaves the row out entirely: a
  // link that answers 404 teaches a reader the app is broken.
  admin: Boolean,
  // The thread as the last answer's model read it: { kept, offered }. Zeros mean the
  // conversation has not started, and then the meter has nothing to draw.
  recall: { type: Object, default: () => ({ kept: 0, offered: 0 }) },
  // What the engine is tuned to: sections per answer, the window share a thread may take, and
  // how many answers the cache holds. Read-only here on purpose — all three are read at startup
  // from .env, and a panel that let you type into a value it cannot change would be lying.
  engine: { type: Object, default: () => ({}) },
  // The two halves of "which system is this", and they answer different questions (rule 25):
  // `release` is the git tag — what changed — and `version` the commit the binary was built
  // from — which bytes are running. Either can be empty and then it simply is not shown: no
  // tag means no release was cut, and no commit means a build with no VCS stamp.
  version: { type: String, default: '' },
  release: { type: String, default: '' },
  t: { type: Function, required: true },
})

defineEmits(['pick', 'mute', 'setLang', 'close'])

// The shell owns the gear that opens this, so it needs a handle — and the handle is shaped
// like the element's own API on purpose: `showModal()` and `close()`, nothing else. The
// composable that calls them cannot tell whether it is holding a <dialog> or this component,
// which is what keeps the imperative part in one place instead of leaking a `.dialog` field
// into the layer above.
const dialog = useTemplateRef('dialog')
defineExpose({
  showModal: () => dialog.value?.showModal(),
  close: () => dialog.value?.close(),
})
</script>

<template>
  <dialog ref="dialog" class="drawer" @close="$emit('close')">
    <div class="head">
      <span class="title">{{ t('settings.title') }}</span>
      <button
        class="btn ghost xs icon" type="button" :aria-label="t('settings.close')"
        @click="$emit('close')"
      >
        <nes-icon name="close" />
      </button>
    </div>

    <div class="drawer-body">
      <!-- ── what answers ──
           `cpu` for the model, `layers` for the window, `bolt` for the price: three glyphs
           that are all in the pinned icons.d.ts, which is the only thing that answers whether
           a name exists — one the release does not have renders an empty box in silence. -->
      <div class="set-group">
        <div class="set-row">
          <nes-icon name="cpu" :title="t('settings.model')" />
          <!-- A <select> only when there is a choice. One model is not a menu of one, so it
               reads as what it is: a name. -->
          <select
            v-if="models.length > 1" class="select" :value="picked"
            :aria-label="t('settings.model')" @change="$emit('pick', $event.target.value)"
          >
            <option v-for="m in models" :key="m.name" :value="m.name">{{ m.name }}</option>
          </select>
          <b v-else>{{ picked || '—' }}</b>
        </div>

        <div class="set-row">
          <nes-icon name="layers" :title="t('settings.window')" />
          <!-- `128k`, not `128,000`: one character of unit says "a magnitude of tokens" where a
               label would have said it in three words, and it can never wrap. -->
          <span>{{ current.window ? `${Math.round(current.window / 1000)}k` : '—' }}</span>
        </div>

        <div class="set-row">
          <nes-icon name="bolt" :title="t('settings.price')" />
          <!-- The `$` is what tells this row apart from the one above it at a glance, which is
               the whole job a label used to do here. -->
          <span v-if="current.price_in || current.price_out">
            ${{ current.price_in || 0 }} / ${{ current.price_out || 0 }}
          </span>
          <span v-else>—</span>
        </div>

        <!-- The thread, as depth rather than as a sentence: `.meter` is 0.14.0's cell bar, so
             "three of eight turns survived the window" is eight boxes with three lit. The
             figure stays beside it for anyone who wants the number, and both are absent before
             a conversation has any turns to trim. -->
        <div class="set-row">
          <nes-icon name="chat" :title="t('settings.memory')" />
          <span v-if="recall.offered" class="set-meter">
            <span class="meter" :aria-label="t('settings.recall', recall)">
              <span
                v-for="i in Math.min(recall.offered, 8)" :key="i"
                class="cell" :class="{ on: i <= recall.kept }"
              />
            </span>
            <small>{{ recall.kept }}/{{ recall.offered }}</small>
          </span>
          <span v-else>—</span>
        </div>
      </div>

      <!-- ── how it reads ── -->
      <div class="set-group">
        <div class="set-row">
          <nes-icon name="globe" :title="t('settings.language')" />
          <div class="segment" role="group" :aria-label="t('settings.language')">
            <button
              v-for="code in langs" :key="code" type="button"
              :aria-pressed="String(code === lang)" @click="$emit('setLang', code)"
            >
              {{ code.toUpperCase() }}
            </button>
          </div>
        </div>

        <!-- The icon is the state as well as the label: a muted instance shows the muted
             glyph, so the row says which way the switch is without a word beside it. -->
        <label class="set-row">
          <nes-icon :name="muted ? 'mute' : 'volume'" :title="t('settings.sound')" />
          <input
            class="switch" type="checkbox" :checked="!muted"
            :aria-label="t('settings.sound')" @change="$emit('mute', !$event.target.checked)"
          >
        </label>
      </div>

      <!-- ── what the engine is tuned to ──
           `grid` for sections per answer, `chat` again for the thread's share — the same glyph
           as the memory row above because it is the same subject, one measured and one
           configured — and `database` for the cached rows. Three numbers, no labels: the panel
           is where you check what this instance is, and /#/admin is where the other sixteen
           knobs and their provenance live. -->
      <div v-if="engine.topK" class="set-group">
        <div class="set-row">
          <nes-icon name="grid" :title="t('settings.topK')" />
          <span>{{ engine.topK }}</span>
        </div>
        <div class="set-row">
          <nes-icon name="chat" :title="t('settings.threadShare')" />
          <span>{{ Math.round(engine.threadShare * 100) }}%</span>
        </div>
        <div class="set-row">
          <nes-icon name="database" :title="t('settings.cacheKeep')" />
          <span>{{ engine.cacheKeep }}</span>
        </div>
      </div>

      <!-- ── what the operator decided ──
           A link, not a copy: those values live in .env and are read at startup, so restating
           them here would be a second home for a fact that already has one. -->
      <div v-if="admin" class="set-group">
        <router-link v-slot="{ navigate }" to="/admin" custom>
          <button
            class="btn ghost sm set-wide" type="button" :title="t('settings.instanceHint')"
            @click="$emit('close'); navigate()"
          >
            <nes-icon name="sliders" /> {{ t('settings.instance') }}
          </button>
        </router-link>
      </div>
    </div>

    <!-- ── which system this is ──
         Outside `.drawer-body`, which is the whole point: the body is `flex: 1` with its own
         overflow, so this is the panel's last row and never scrolls away. The one fact you open
         this panel to confirm must not be the one that left the screen.
         It is here rather than only in the status line because this is a *modal* <dialog> — it
         covers the status line, so while the panel is open the version is otherwise unreadable.
         Right-aligned: the rows above are read down their left edge, and this is a stamp rather
         than a control, so it sits out of that column.
         No `nes-icon`: the library ships no `branch` or `tag` glyph, and a name a release does
         not have renders an empty box in silence — the same call StatusLine.vue already made. -->
    <div class="set-foot" :title="t('settings.version')">
      <b v-if="release">{{ release }}</b>
      <span v-if="version">@{{ version }}</span>
      <span v-if="!release && !version">—</span>
    </div>
  </dialog>
</template>
