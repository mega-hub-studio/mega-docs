/* ══ app.js — the conversation, and the loop that fills the gaps ═══════════════
   Intent only. Transport lives in chat.js and qa.js, rendering in answer.js,
   viewport plumbing in viewport.js — so what's left here reads like the product:
   ask, regenerate, stop, copy, reset · askBA, confirm, dismiss.

   Two modes, one screen:
     DEV  asks the source of truth. When the answer is wrong or missing, one tap
          files the gap as a ticket, with the failed answer attached as evidence.
     BA   works that queue. Confirming an answer writes it into the corpus, where
          the next DEV retrieves it with a citation — and the second time anyone
          asks, it comes from the cache and costs nothing.

   Vue 3 global build (no bundler): `Vue` is a global from index.html, and the
   design-system helpers are injected by boot() rather than imported, so the
   pinned + integrity-checked CDN URL stays in exactly one file (index.html).
   ═══════════════════════════════════════════════════════════════════════════ */
import { ask as askServer, health } from "./chat.js";
import { answerHtml, fileName } from "./answer.js";
import * as diagram from "./diagram.js";
import { bindViewport } from "./viewport.js";
import { loadCorpus, shortDate } from "./library.js";
import * as session from "./session.js";
import * as qa from "./qa.js";
import { BaScreen } from "./ba.js";
import { CorpusTree } from "./tree.js";

const STARTERS = [
  "How does retrieval stay grounded?",
  "How do I ingest a PDF?",
  "Which env vars change answer quality?",
];

const MODE_KEY = "ke.mode"; // a BA reopening the app wants the queue, not the prompt
const SCOPE_KEY = "ke.scope"; // the folder being asked about outlives a reload, like the thread

let seq = 0;
const newTurn = (q, scope) => ({
  id: ++seq, q, scope, a: "", citations: [], streaming: true, error: "", ms: 0,
  cached: false, in: 0, out: 0,
  ticket: null, // the gap filed from this answer, once there is one
});

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
        // Which part of the corpus questions are answered from. "" is all of it, and
        // is what every question was before this control existed.
        scope: localStorage.getItem(SCOPE_KEY) || "",

        /* ── the QA loop. Everything here is rendered by *both* screens — the
           header badge, and the "questions with a BA" list on the ASK screen. What
           only BA mode touches lives in ba.js. ── */
        mode: localStorage.getItem(MODE_KEY) === "ba" ? "ba" : "dev",
        writes: false, // does this instance allow a BA to confirm at all
        // What the bottom strip reports. Filled by checkHealth; zeros stay hidden.
        runtime: { model: "", window: 0, priceIn: 0, priceOut: 0 },
        // Flipped once the mermaid renderer is in the page. Until then a diagram
        // answer shows the fenced code the model wrote, which still reads.
        mermaidReady: false,
        queue: { tickets: [], open: 0, answered: 0, confirmed: 0, rejected: 0 },
        history: [],
        status: qa.STATUS,
      };
    },

    mounted() {
      this.view = bindViewport(this.$refs.dock);
      this.checkHealth();
      this.refreshCorpus();
      this.refreshQueue();
      if (this.turns.length) this.view.scrollToEnd({ force: true });
      addEventListener("online", () => this.checkHealth());
      addEventListener("offline", () => (this.online = false));
    },

    computed: {
      /** The bottom strip, assembled from what is actually known.
       *
       *  Nothing here is estimated. A field whose input is missing is left empty and
       *  the markup drops it, because an unmeasured cost and a cost of nothing look
       *  the same on screen and are not the same fact. */
      statusLine() {
        const t = this.turns.at(-1);
        const state = !this.online ? "error"
          : this.busy ? "running"
          : t?.error ? "error"
          : t ? "done"
          : "queued";
        const label = { error: "ERROR", running: "ASKING", done: "READY", queued: "IDLE" }[state];
        const s = {
          state,
          label: this.online ? label : "OFFLINE",
          tokens: "", refs: 0, elapsed: "", cost: "", costTitle: "",
        };
        if (!t || t.streaming) return s;

        s.refs = t.citations.length;
        if (t.ms) s.elapsed = t.ms >= 1000 ? (t.ms / 1000).toFixed(1) + "s" : t.ms + "ms";

        const total = (t.in || 0) + (t.out || 0);
        if (total) {
          s.tokens = total.toLocaleString() + " tok";
          // Only claim a share of the window when the operator said how big it is.
          if (this.runtime.window > 0) {
            s.tokens += " \u00b7 " + Math.round((total / this.runtime.window) * 100) + "%";
          }
        }
        if (t.cached) {
          s.cost = "cached \u00b7 free";
          s.costTitle = "Served from the answer cache \u2014 no completion was bought";
        } else if (total && (this.runtime.priceIn || this.runtime.priceOut)) {
          const usd = ((t.in || 0) * this.runtime.priceIn + (t.out || 0) * this.runtime.priceOut) / 1e6;
          // Four decimals: one internal question costs a fraction of a cent, and
          // rounding it to $0.00 hides the only number anyone would act on.
          s.cost = "$" + usd.toFixed(4);
          s.costTitle = t.in + " in + " + t.out + " out at $" + this.runtime.priceIn + " / $" + this.runtime.priceOut + " per 1M";
        }
        return s;
      },
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
          diagrams: !t.streaming && this.mermaidReady,
          // The numbers, not how many: the engine returns only the sources the
          // answer cited and keeps their original n, so [2] can arrive alone.
          nums: t.streaming ? [] : t.citations.map((c) => c.n),
          srcId: (n) => this.srcId(t, n),
        });
      },

      /* ── actions ── */
      async ask(question) {
        if (!question?.trim() || this.busy) return;
        this.turns.push(newTurn(question.trim(), this.scope));
        // Stream into the *reactive* turn, not the object just pushed: Vue hands
        // out a proxy per array item, and writing to the raw one updates no DOM.
        const turn = this.turns.at(-1);
        this.view.scrollToEnd({ force: true, smooth: true });
        await this.stream(turn);
      },

      /** Re-run a turn in place — the mobile alternative to retyping. Always spends
       *  a real call: the user asked again because the cached answer was wrong. */
      async regenerate(turn) {
        if (this.busy) return;
        Object.assign(turn, { a: "", citations: [], error: "", ms: 0, streaming: true, cached: false, in: 0, out: 0 });
        await this.stream(turn, { fresh: true });
      },

      stop() {
        this.run?.stop();
      },

      /** Narrow the next question to one folder or document — or, with "", widen it
       *  back to everything. Stored, because a reader working through one area asks
       *  several questions about it and a reload should not silently widen them. */
      /** Picked from the tree: close the picker too, or the answer arrives behind it. */
      pickScope(scope) {
        this.setScope(scope);
        if (this.$refs.pick) this.$refs.pick.open = false;
      },

      setScope(scope) {
        this.scope = scope || "";
        this.scope ? localStorage.setItem(SCOPE_KEY, this.scope) : localStorage.removeItem(SCOPE_KEY);
      },

      reset() {
        this.stop();
        this.turns = [];
        session.clear();
        this.$refs.prompt?.focus();
      },

      async refreshCorpus() {
        this.corpus = await loadCorpus();
      },

      /* ── the QA loop ── */

      setMode(mode) {
        this.mode = mode;
        localStorage.setItem(MODE_KEY, mode);
        if (mode === "ba") this.refreshQueue();
      },

      /** Report the gap this answer just proved. The failed answer travels with it. */
      async askBA(turn) {
        try {
          const ticket = await qa.file(turn.q, turn.error || turn.a);
          turn.ticket = ticket;
          ds.toast(
            `<b>Ticket #${ticket.id}.</b> A BA will answer this, and the answer joins the documents.`,
            { accent: "good" },
          );
          this.refreshQueue();
        } catch (e) {
          ds.toast(`<b>Couldn't file it.</b> ${e.message}`, { accent: "crit" });
        }
      },

      /** The queue and the history belong to the shell because both screens show
       *  them. Failures stay silent: a stale badge beats an error banner over a
       *  working conversation. */
      async refreshQueue() {
        try {
          this.queue = await qa.queue();
          this.history = await qa.history();
        } catch {
          /* the badge, the queue and the history stay as they were */
        }
      },

      /** The BA screen moved something. What changed is never only one thing: a
       *  confirm or an import changes the corpus, which invalidates the cache, which
       *  empties the history — so refresh all three instead of reasoning about it. */
      baChanged(ticket) {
        this.refreshQueue();
        this.refreshCorpus();
        if (ticket) this.markConfirmed(ticket);
      },

      /** Reflect a confirm on the DEV side without a reload. */
      markConfirmed(ticket) {
        for (const t of this.turns) {
          if (t.ticket?.id === ticket.id) t.ticket = ticket;
        }
      },

      /** Re-ask a cached question. Free — but only in the scope it was answered in:
       *  the same words in another folder are another question, and buying a
       *  completion from a panel labelled "free to repeat" is a broken promise. */
      replay(entry) {
        this.setMode("dev");
        this.setScope(entry.scope || "");
        this.ask(entry.question);
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
      async stream(turn, { fresh = false } = {}) {
        this.busy = true;
        const started = performance.now();
        this.run = askServer(turn.q, {
          fresh,
          // The turn's own scope, not the current one: a regenerate must re-ask the
          // question that was asked, even if the reader has since picked elsewhere.
          scope: turn.scope || "",
          onToken: (t) => {
            turn.a += t;
            this.view.scrollToEnd();
          },
          onCitations: (c) => (turn.citations = c),
          onDone: ({ cached, in: tin, out }) => Object.assign(turn, { cached, in: tin, out }),
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
          // Fetch the diagram renderer only now, and only if this answer drew one.
          // Flipping the flag re-renders the answer, which is when the fence turns
          // into <nes-mermaid> — so the element never exists before mermaid does.
          if (!this.mermaidReady && diagram.hasDiagram(turn.a)) {
            diagram.ready().then((ok) => (this.mermaidReady = ok));
          }
        }
      },

      async checkHealth() {
        const h = await health();
        this.online = h.online;
        this.writes = h.writes;
        this.runtime = { model: h.model, window: h.window, priceIn: h.priceIn, priceOut: h.priceOut };
      },
    },
  });

  // In-browser compiler: don't try to resolve <nes-*> as Vue components. Must be
  // set before mount.
  app.config.compilerOptions.isCustomElement = (tag) => tag.startsWith("nes-");
  app.component("ba-screen", BaScreen);
  app.component("corpus-tree", CorpusTree);
  // ba.js cannot import toast(): the pinned, integrity-checked CDN URL lives in
  // index.html alone, so the helper is injected the same way boot() receives it.
  app.provide("toast", ds.toast);
  app.mount("#app");
}
