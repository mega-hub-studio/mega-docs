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
import { bindViewport } from "./viewport.js";
import { loadCorpus, shortDate } from "./library.js";
import * as session from "./session.js";
import * as qa from "./qa.js";

const STARTERS = [
  "How does retrieval stay grounded?",
  "How do I ingest a PDF?",
  "Which env vars change answer quality?",
];

const MODE_KEY = "ke.mode"; // a BA reopening the app wants the queue, not the prompt

/* What each transition means, said once. A confirm is the only one worth
   celebrating: it is the moment a gap became part of the documents. */
const TOASTS = {
  draft: (t) => `<b>Draft saved.</b> Ticket #${t.id} is not published yet.`,
  confirm: (t) => `<b>In the knowledge base.</b> ${t.doc} — the next question retrieves it.`,
  reject: (t) => `<b>Dismissed #${t.id}.</b> It stays on the list, with your reason.`,
};

let seq = 0;
const newTurn = (q) => ({
  id: ++seq, q, a: "", citations: [], streaming: true, error: "", ms: 0,
  cached: false,
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
        showSources: false,

        /* ── the QA loop ── */
        mode: localStorage.getItem(MODE_KEY) === "ba" ? "ba" : "dev",
        writes: false, // does this instance allow a BA to confirm at all
        unlocked: !!qa.pass(),
        passInput: "",
        queue: { tickets: [], open: 0, answered: 0, confirmed: 0, rejected: 0 },
        drafts: {}, // ticket id → the answer being typed, kept out of the server copy
        working: 0, // id of the ticket currently being published
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

      /** Re-run a turn in place — the mobile alternative to retyping. Always spends
       *  a real call: the user asked again because the cached answer was wrong. */
      async regenerate(turn) {
        if (this.busy) return;
        Object.assign(turn, { a: "", citations: [], error: "", ms: 0, streaming: true, cached: false });
        await this.stream(turn, { fresh: true });
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

      async refreshQueue() {
        try {
          this.queue = await qa.queue();
          // Seed each editor from the server's copy, without letting a refresh
          // overwrite an answer someone is halfway through typing.
          for (const t of this.queue.tickets) {
            if (this.drafts[t.id] === undefined) this.drafts[t.id] = t.answer || "";
          }
          this.history = await qa.history();
        } catch {
          /* the badge and the queue simply stay as they were */
        }
      },

      unlock() {
        qa.setPass(this.passInput.trim());
        this.unlocked = !!qa.pass();
        this.passInput = "";
      },

      /** draft · confirm · reject — one path, so every outcome is handled once. */
      async move(ticket, action) {
        this.working = ticket.id;
        const answer = (this.drafts[ticket.id] || "").trim();
        try {
          const updated = await qa.act(ticket.id, action, { answer, note: answer });
          Object.assign(ticket, updated);
          this.drafts[ticket.id] = updated.answer || "";
          ds.toast(TOASTS[action](updated), { accent: action === "reject" ? "warn" : "good" });
          // A confirm changes the corpus: the answer count, the cache, and what the
          // next question retrieves all move with it.
          if (action === "confirm") {
            this.refreshCorpus();
            this.markConfirmed(updated);
          }
          this.refreshQueue();
        } catch (e) {
          if (e instanceof qa.WrongPass) {
            this.unlocked = false;
            ds.toast(`<b>Locked.</b> ${e.message}`, { accent: "crit" });
          } else {
            ds.toast(`<b>${e.message}</b>`, { accent: "crit" });
          }
        } finally {
          this.working = 0;
        }
      },

      /** Reflect a confirm on the DEV side without a reload. */
      markConfirmed(ticket) {
        for (const t of this.turns) {
          if (t.ticket?.id === ticket.id) t.ticket = ticket;
        }
      },

      /** Re-ask a cached question. Free — that is the whole point of the panel. */
      replay(question) {
        this.setMode("dev");
        this.ask(question);
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
          onToken: (t) => {
            turn.a += t;
            this.view.scrollToEnd();
          },
          onCitations: (c) => (turn.citations = c),
          onDone: ({ cached }) => (turn.cached = cached),
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
        const { online, writes } = await health();
        this.online = online;
        this.writes = writes;
      },
    },
  });

  // In-browser compiler: don't try to resolve <nes-*> as Vue components. Must be
  // set before mount.
  app.config.compilerOptions.isCustomElement = (tag) => tag.startsWith("nes-");
  app.mount("#app");
}
