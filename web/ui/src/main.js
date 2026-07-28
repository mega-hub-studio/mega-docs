/* ══ main.js — the entry: three lines of setup and a mount ══════════════════════
   Everything the browser needs is imported here, in load order: the design system's
   stylesheet (which pulls in its own fonts), its custom elements, this app's layout CSS,
   and the shell. Vite bundles and content-hashes all of it, so the page it produces names
   no version and fetches nothing from a CDN.
   ═══════════════════════════════════════════════════════════════════════════ */
// One import of the design system, for both halves of what it is: `setMute` is a helper,
// and the module's side effect is defining <nes-icon>, <nes-chat-prompt>, <nes-tree> and
// <nes-mermaid>. Importing it twice would say otherwise.
import { setMute } from "8bit-nes";
import { createApp } from "vue";
import App from "./App.vue";

import "8bit-nes/all.css";
import "./styles.css";

// The chiptune SFX default to on. In a work tool opened on a phone the first send
// shouldn't make noise — default to silent, but only on a first visit, so a deliberate
// un-mute is remembered.
if (localStorage.getItem("nes_mute") === null) setMute(true);

const app = createApp(App);
// No isCustomElement here: templates are compiled at *build* time, so that decision
// lives in vite.config.js. Setting it at runtime would be too late — the compiled render
// functions would already be resolving <nes-icon> as a Vue component.
app.mount("#app");
