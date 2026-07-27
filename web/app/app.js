/* ══ app.js — the shell: what the screens share, and how it is wired ═══════════
   This file is wiring. Every piece of state lives in a composable under use/, every
   screen is a component, and transport/rendering stay in the flat modules they always
   did (chat.js, answer.js, qa.js, library.js, session.js, viewport.js, diagram.js).
   What is left here is the product read out loud:

     ask · regenerate · stop · copy · reset          the conversation
     askBA · baChanged · replay                      the loop that fills the gaps
     setScope · pickScope                            which folder answers
     setMode                                         which screen you are on

   Two modes, one screen:
     DEV  asks the source of truth. When the answer is wrong or missing, one tap files
          the gap as a ticket, with the failed answer attached as evidence.
     BA   works that queue. Confirming an answer writes it into the corpus, where the
          next DEV retrieves it with a citation — and the second time anyone asks, it
          comes from the cache and costs nothing.

   Vue 3.5, Composition API, no bundler: `Vue` is a global from index.html (the build
   that ships the compiler, so in-DOM templates work), and the design-system helpers
   are injected by boot() rather than imported — so the pinned, integrity-checked CDN
   URL stays in exactly one file.
   ═══════════════════════════════════════════════════════════════════════════ */
import { answerHtml, fileName } from "./answer.js";
import { bindViewport } from "./viewport.js";
import { shortDate } from "./library.js";
import { STATUS } from "./qa.js";
import { BaScreen } from "./ba.js";
import { CorpusTree } from "./tree.js";
import { useScope } from "./use/scope.js";
import { useCorpus } from "./use/corpus.js";
import { useRuntime } from "./use/runtime.js";
import { useStatusLine } from "./use/statusline.js";
import { useConversation } from "./use/conversation.js";
import { useQaLoop } from "./use/qaloop.js";
import { useDiagrams } from "./use/diagrams.js";

const MODE_KEY = "ke.mode"; // a BA reopening the app wants the queue, not the prompt

/**
 * Mount the app.
 * @param {{ toast: Function, setMute: Function }} ds 8bit-nes helpers
 */
export function boot(ds) {
  // The chiptune SFX default to on. In a work tool opened on a phone the first send
  // shouldn't make noise — default to silent, but only on a first visit, so a
  // deliberate un-mute is remembered.
  if (localStorage.getItem("nes_mute") === null) ds.setMute(true);

  const app = Vue.createApp({
    setup() {
      const { ref, onMounted, useTemplateRef } = Vue;

      /* ── the DOM the app has to touch directly ── */
      const prompt = useTemplateRef("prompt"); // busy must reach it as an attribute
      const dock = useTemplateRef("dock"); // the keyboard/scroll maths measures it
      const pick = useTemplateRef("pick"); // <details>: closed after a scope is picked
      const zoom = useTemplateRef("zoom"); // <dialog>: the diagram viewer
      const zoomBody = useTemplateRef("zoomBody");

      /* ── state, one concern per composable ── */
      const { scope, setScope } = useScope();
      const corpus = useCorpus();
      const { online, writes, runtime, check, watchNetwork } = useRuntime();
      const loop = useQaLoop({ toast: ds.toast });
      const diagrams = useDiagrams({ zoom, zoomBody });

      // viewport.js binds to a real element, so it can only exist after mount. The
      // conversation is given a function rather than the object, and a scroll before
      // mount is a no-op instead of a crash.
      let view = null;
      const scroll = (opts) => view?.scrollToEnd(opts);

      const chat = useConversation({
        scope,
        prompt,
        scroll,
        toast: ds.toast,
        // Everything an answer can have changed, in one place. Cheap, and cheaper than
        // reasoning about which of them this particular answer touched.
        onSettled: (turn) => {
          if (turn.error) check();
          if (corpus.corpus.value.state !== "ready") corpus.refresh();
          diagrams.loadFor(turn.a);
        },
      });

      const statusLine = useStatusLine({ turns: chat.turns, busy: chat.busy, online, runtime });

      const mode = ref(localStorage.getItem(MODE_KEY) === "ba" ? "ba" : "dev");

      onMounted(() => {
        view = bindViewport(dock.value);
        check();
        corpus.refresh();
        loop.refresh();
        if (chat.turns.value.length) {
          scroll({ force: true });
          diagrams.loadFor(chat.turns.value.map((t) => t.a).join("\n"));
        }
        watchNetwork();
      });

      /* ── intent ── */

      function setMode(next) {
        mode.value = next;
        localStorage.setItem(MODE_KEY, next);
        if (next === "ba") loop.refresh();
      }

      /** Picked from the tree: close the picker too, or the answer arrives behind it. */
      function pickScope(next) {
        setScope(next);
        if (pick.value) pick.value.open = false;
      }

      /** The BA screen moved something. What changed is never only one thing: a confirm
       *  or an import changes the corpus, which invalidates the cache, which empties the
       *  history — so refresh all three instead of reasoning about it. */
      function baChanged(ticket) {
        loop.refresh();
        corpus.refresh();
        if (ticket) chat.markConfirmed(ticket);
      }

      /** Re-ask a cached question. Free — but only in the scope it was answered in: the
       *  same words in another folder are another question, and buying a completion from
       *  a panel labelled "free to repeat" is a broken promise. */
      function replay(entry) {
        setMode("dev");
        setScope(entry.scope || "");
        chat.ask(entry.question);
      }

      /* ── template helpers ── */

      const srcId = (turn, n) => `s${turn.id}-${n}`;

      /** Render one answer. No citation links mid-stream: the list only lands at the
       *  end, and until then a "[1]" has nothing to point at. */
      const renderAnswer = (t) =>
        answerHtml(t.a, {
          diagrams: !t.streaming && diagrams.ready.value,
          // The numbers, not how many: the engine returns only the sources the answer
          // cited and keeps their original n, so [2] can arrive alone.
          nums: t.streaming ? [] : t.citations.map((c) => c.n),
          srcId: (n) => srcId(t, n),
        });

      return {
        // state
        turns: chat.turns,
        busy: chat.busy,
        online,
        writes,
        runtime,
        corpus: corpus.corpus,
        starters: corpus.starters,
        scope,
        mode,
        queue: loop.queue,
        history: loop.history,
        status: STATUS,
        statusLine,
        // conversation
        ask: chat.ask,
        regenerate: chat.regenerate,
        stop: chat.stop,
        reset: chat.reset,
        copy: chat.copy,
        // the loop
        askBA: loop.file,
        baChanged,
        replay,
        // scope, mode
        setScope,
        pickScope,
        setMode,
        // diagrams
        diagramDrawn: diagrams.drawn,
        zoomDiagram: diagrams.open,
        closeZoom: diagrams.close,
        // helpers the markup calls
        answerHtml: renderAnswer,
        short: fileName,
        shortDate,
        srcId,
      };
    },
  });

  // In-browser compiler: don't try to resolve <nes-*> as Vue components. Must be set
  // before mount.
  app.config.compilerOptions.isCustomElement = (tag) => tag.startsWith("nes-");
  app.component("ba-screen", BaScreen);
  app.component("corpus-tree", CorpusTree);
  // The components cannot import toast(): the pinned, integrity-checked CDN URL lives in
  // index.html alone, so the helper is injected the same way boot() receives it.
  app.provide("toast", ds.toast);
  app.mount("#app");
}
