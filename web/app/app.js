/* ══ app.js — the conversation ═════════════════════════════════════════════════
   Intent only. Transport lives in chat.js, rendering in answer.js, viewport
   plumbing in viewport.js — so what's left here reads like the product:
   ask, regenerate, stop, copy, reset.

   Vue 3 global build (no bundler): `Vue` is a global from index.html, and the
   design-system helpers are injected by boot() rather than imported, so the
   pinned + integrity-checked CDN URL stays in exactly one file (index.html).
   ═══════════════════════════════════════════════════════════════════════════ */
import { ask as askServer, healthy } from "./chat.js";
import { answerHtml, fileName } from "./answer.js";
import { bindViewport } from "./viewport.js";
import { loadCorpus, shortDate } from "./library.js";
import * as session from "./session.js";

const STARTERS = [
  "How does retrieval stay grounded?",
  "How do I ingest a PDF?",
  "Which env vars change answer quality?",
];

let seq = 0;
const newTurn = (q) => ({ id: ++seq, q, a: "", citations: [], streaming: true, error: "", ms: 0 });

/**
 * Mount the app.
 * @param {{ toast: Function, setMute: Function }} ds 8bit-nes helpers
 */
export function boot(ds) {
  // The chiptune SFX default to on. In a work tool opened on a phone the first
  // send shouldn't make noise — default to silent, but only on a first visit, so
  // a deliberate un-mute is remembered.
  if (localStorage.getItem("nes_mute") === null) ds.setMute(true);

  const app = Vue.createApp({
    data() {
      const turns = session.load(); // a reload shouldn't lose the thread
      seq = turns.reduce((m, t) => Math.max(m, t.id || 0), 0);
      return {
        turns,
        busy: false,
        online: true,
        starters: STARTERS,
        corpus: { state: "loading", docs: 0, chunks: 0, approved: 0, documents: [] },
        showSources: false,
      };
    },

    mounted() {
      this.view = bindViewport(this.$refs.dock);
      this.checkHealth();
      this.refreshCorpus();
      if (this.turns.length) this.view.scrollToEnd({ force: true });
      addEventListener("online", () => this.checkHealth());
      addEventListener("offline", () => (this.online = false));
    },

    watch: {
      // <nes-chat-prompt> flips its ▶/■ button from attributeChangedCallback, so
      // `busy` has to reach it as an *attribute*. A :busy binding won't do it:
      // once the element sets its own `busy` property, Vue's custom-element
      // heuristic starts writing that property instead, and the callback stops
      // firing — leaving the button stuck on ■.
      busy(on) {
        const el = this.$refs.prompt;
        if (!el) return;
        on ? el.setAttribute("busy", "") : el.removeAttribute("busy");
      },

      // Persisting on every mutation would write once per streamed token; save()
      // debounces, so a deep watcher here is cheap and never misses the last one.
      turns: {
        deep: true,
        handler(turns) {
          turns.length ? session.save(turns) : session.clear();
        },
      },
    },

    methods: {
      /* ── template helpers ── */
      short: fileName,
      shortDate,
      srcId(turn, n) {
        return `s${turn.id}-${n}`;
      },
      answerHtml(t) {
        // No linking mid-stream: the citation list only lands at the end, and
        // until then a "[1]" has nothing to point at.
        return answerHtml(t.a, {
          count: t.streaming ? 0 : t.citations.length,
          srcId: (n) => this.srcId(t, n),
        });
      },

      /* ── actions ── */
      async ask(question) {
        if (!question?.trim() || this.busy) return;
        this.turns.push(newTurn(question.trim()));
        // Stream into the *reactive* turn, not the object just pushed: Vue hands
        // out a proxy per array item, and writing to the raw one updates no DOM.
        const turn = this.turns.at(-1);
        this.view.scrollToEnd({ force: true, smooth: true });
        await this.stream(turn);
      },

      /** Re-run a turn in place — the mobile alternative to retyping. */
      async regenerate(turn) {
        if (this.busy) return;
        Object.assign(turn, { a: "", citations: [], error: "", ms: 0, streaming: true });
        await this.stream(turn);
      },

      stop() {
        this.run?.stop();
      },

      reset() {
        this.stop();
        this.turns = [];
        session.clear();
        this.showSources = false;
        this.$refs.prompt?.focus();
      },

      async refreshCorpus() {
        this.corpus = await loadCorpus();
      },

      async copy(turn) {
        try {
          await navigator.clipboard.writeText(turn.a);
          ds.toast("<b>Copied.</b> Answer on the clipboard.", { accent: "good" });
        } catch {
          ds.toast("<b>Copy blocked.</b> Select the text instead.", { accent: "warn" });
        }
      },

      /* ── plumbing (one place) ── */
      async stream(turn) {
        this.busy = true;
        const started = performance.now();
        this.run = askServer(turn.q, {
          onToken: (t) => {
            turn.a += t;
            this.view.scrollToEnd();
          },
          onCitations: (c) => (turn.citations = c),
        });
        try {
          await this.run.done; // a stop() resolves quietly; only real errors throw
        } catch (e) {
          turn.error = e.message;
          this.checkHealth();
        } finally {
          turn.ms = Math.round(performance.now() - started);
          turn.streaming = false;
          this.busy = false;
          this.run = null;
          if (this.corpus.state !== "ready") this.refreshCorpus();
        }
      },

      async checkHealth() {
        this.online = await healthy();
      },
    },
  });

  // In-browser compiler: don't try to resolve <nes-*> as Vue components. Must be
  // set before mount.
  app.config.compilerOptions.isCustomElement = (tag) => tag.startsWith("nes-");
  app.mount("#app");
}
