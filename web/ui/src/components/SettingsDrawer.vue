<script setup>
import { useTemplateRef } from 'vue'

/* ══ SettingsDrawer.vue — one gear, one panel, everything a reader decides ═══════
   A contract, like every other screen part: what comes in, what goes out, and a template
   over nothing at all — the state belongs to the shell, because the model rides on every
   question `useConversation` sends.

     props   models · picked · current · muted · lang · langs · admin · recall
     emit    pick(name) · mute(bool) · setLang(code) · close

   Every control is a library recipe: `.drawer` on a native <dialog> (so Escape, the focus
   trap and the backdrop are the platform's, not ours), `.eyebrow` for the three section
   labels, `.field`/`.select` for the model, `.segment` for the language, `.switch` for the
   sound, `.datalist` for the read-only rows, `.callout` for the one sentence about /#/admin.
   This file adds no styling of its own beyond the two layout rules `styles.css` names.

   Why a drawer rather than a screen: settings are read *against* what you were doing — you
   pick a stronger model because the answer behind the panel was thin. A route would take that
   away, which is the same reason the BA library's form is not a modal.
   ═══════════════════════════════════════════════════════════════════════════ */
defineProps({
  models: { type: Array, default: () => [] },
  picked: { type: String, default: '' },
  // The picked model's own window and price, already resolved by the composable — zero in
  // either means the operator never said, and then nothing is printed rather than a zero.
  current: { type: Object, default: () => ({}) },
  muted: Boolean,
  lang: { type: String, required: true },
  langs: { type: Array, required: true },
  // Whether this instance has an admin surface at all. False leaves the row out entirely: a
  // link that answers 404 teaches a reader the app is broken.
  admin: Boolean,
  // The thread as the last answer's model read it: { kept, offered }. Zeros mean the
  // conversation has not started, which is a sentence rather than a figure.
  recall: { type: Object, default: () => ({ kept: 0, offered: 0 }) },
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
  <!-- `ref` is bound by the shell, which owns showModal()/close(): the gear that opens this
       lives in the bar, so the element has to be reachable from there. -->
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
      <!-- ── what answers ── -->
      <span class="eyebrow">{{ t('settings.answering') }}</span>

      <label v-if="models.length > 1" class="field">
        <span class="label">{{ t('settings.model') }}</span>
        <!-- A <select>, not a .segment: a model name is 12–20 characters, so three of them
             side by side in a 22rem drawer would each be four characters and an ellipsis. -->
        <select
          class="select" :value="picked"
          @change="$emit('pick', $event.target.value)"
        >
          <option v-for="m in models" :key="m.name" :value="m.name">{{ m.name }}</option>
        </select>
        <span class="hint">{{ t('settings.modelHint') }}</span>
      </label>

      <!-- One model is not a menu of one. Say which it is and move on. -->
      <dl v-else class="datalist">
        <dt>{{ t('settings.model') }}</dt>
        <dd>{{ picked || t('settings.oneModel') }}</dd>
      </dl>

      <!-- The two numbers that decide what the strip may claim. Absent, not zero, when the
           operator never configured them — an unmeasured cost and a cost of nothing are
           different facts, and that rule is older than this panel. -->
      <dl class="datalist">
        <template v-if="current.window">
          <dt>{{ t('settings.window') }}</dt>
          <dd>{{ current.window.toLocaleString() }}</dd>
        </template>
        <dt>{{ t('settings.price') }}</dt>
        <dd v-if="current.price_in || current.price_out">
          {{ current.price_in || 0 }} / {{ current.price_out || 0 }}
        </dd>
        <dd v-else>{{ t('settings.unpriced') }}</dd>
        <dt>{{ t('settings.memory') }}</dt>
        <dd v-if="recall.offered">{{ t('settings.recall', recall) }}</dd>
        <dd v-else>{{ t('settings.recallNone') }}</dd>
      </dl>

      <!-- ── how it reads ── -->
      <span class="eyebrow">{{ t('settings.reading') }}</span>

      <!-- .segment is the library's single-choice control and two locale codes fit it
           exactly, which is why the language moved here from the bar: the bar was carrying a
           control that is used twice a year. -->
      <div class="field">
        <span class="label">{{ t('settings.language') }}</span>
        <div class="segment" role="group" :aria-label="t('settings.language')">
          <button
            v-for="code in langs" :key="code" type="button"
            :aria-pressed="String(code === lang)" @click="$emit('setLang', code)"
          >
            {{ code.toUpperCase() }}
          </button>
        </div>
      </div>

      <label class="switch-row">
        <input
          class="switch" type="checkbox" :checked="!muted"
          @change="$emit('mute', !$event.target.checked)"
        >
        <span>{{ t('settings.sound') }}</span>
      </label>

      <!-- ── what the operator decided ── -->
      <template v-if="admin">
        <span class="eyebrow">{{ t('settings.instance') }}</span>
        <div class="callout memo">
          <span>{{ t('settings.instanceHint') }}</span>
        </div>
        <!-- A link, not a copy: those values live in .env and are read at startup, so
             restating them here would be a second home for a fact that already has one. -->
        <router-link v-slot="{ navigate }" to="/admin" custom>
          <button class="btn ghost sm" type="button" @click="$emit('close'); navigate()">
            <nes-icon name="gear" /> {{ t('settings.openAdmin') }}
          </button>
        </router-link>
      </template>
    </div>
  </dialog>
</template>
