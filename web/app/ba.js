/* ══ ba.js — the BA screen: the ticket queue and the import card ═══════════════
   Its own component because its state is its own: thirteen of the shell's reactive keys
   belonged only to this screen and shared one object with the chat thread, so the import
   progress bar and the conversation could collide by name.

   In-DOM template (#ba-screen in index.html), not an SFC — the pinned Vue global build
   ships the compiler, so a component costs no build step here.

   Crossings, kept to three:
     props   writes · online · queue · documents   all of which the ASK screen also
             renders, so they belong to the shell rather than here
     emit    changed(ticket|null)  something moved: the shell refreshes queue, corpus and
             history, and updates the turn badge when a ticket came with it
     inject  toast                 the design-system helper, provided by boot()
   ═══════════════════════════════════════════════════════════════════════════ */
import * as qa from "./qa.js";
import * as upload from "./upload.js";
import { shortDate } from "./library.js";

/* What each transition means, said once. A confirm is the only one worth celebrating:
   it is the moment a gap became part of the documents. */
const TOASTS = {
  draft: (t) => `<b>Draft saved.</b> Ticket #${t.id} is not published yet.`,
  confirm: (t) => `<b>In the knowledge base.</b> ${t.doc} — the next question retrieves it.`,
  reject: (t) => `<b>Dismissed #${t.id}.</b> It stays on the list, with your reason.`,
};

export const BaScreen = {
  name: "BaScreen",
  template: "#ba-screen",
  props: {
    writes: Boolean, // does this instance allow a BA to publish at all
    online: Boolean, // unreachable is not read-only, and must not read as it
    queue: { type: Object, required: true }, // the ASK screen lists it too
    documents: { type: Array, default: () => [] },
  },
  emits: ["changed", "ask"], // "ask" = take me back to the chat
  inject: ["toast"],

  setup(props, { emit }) {
    const { ref, reactive, computed, watch, inject } = Vue;
    const toast = inject("toast");

    const drafts = reactive({}); // ticket id → the answer being typed, not the server's copy
    const working = ref(0); // id of the ticket currently being published
    const unlocked = ref(!!qa.pass());
    const passInput = ref("");
    const unlocking = ref(false);
    const unlockError = ref("");

    /* ── importing documents ── */
    const importDir = ref(""); // the folder this batch lands in — the scope a reader browses
    const importing = ref(false);
    const progress = ref({ done: 0, total: 0 }); // real counts: the bar must not invent a position
    const dragging = ref(false); // a drop target that doesn't light up reads as inert
    const imported = ref(null); // {uploaded, failed, chunks} from the last import

    /** The folders that already exist, so the picker suggests the structure rather than
     *  inviting a fourth spelling of "engineering". */
    const folders = computed(() => upload.folders(props.documents));

    // Seed each editor from the server's copy, and keep doing it as the queue refreshes —
    // without overwriting an answer someone is halfway through typing.
    watch(
      () => props.queue,
      (q) => {
        for (const t of q.tickets) {
          if (drafts[t.id] === undefined) drafts[t.id] = t.answer || "";
        }
      },
      { immediate: true },
    );

    /** A refused password has to say so *in the form*, not only in a toast: the thing
     *  being read is the card that just vanished. */
    function fail(e, prefix = "") {
      if (e instanceof qa.WrongPass) {
        unlocked.value = false;
        if (prefix) unlockError.value = prefix + e.message;
        toast(`<b>Locked.</b> ${e.message}`, { accent: "crit" });
      } else {
        toast(`<b>${e.message}</b>`, { accent: "crit" });
      }
    }

    /** Check the password before saying it worked. Storing it unchecked is how a typo
     *  used to survive until the first upload, and then look like a broken import rather
     *  than a wrong password. */
    async function unlock() {
      const candidate = passInput.value.trim();
      if (!candidate) return;
      unlockError.value = "";
      unlocking.value = true;
      try {
        if (!(await upload.verify(candidate))) {
          unlockError.value = "That password does not open the gate. Reads still work.";
          return;
        }
        qa.setPass(candidate);
        unlocked.value = !!qa.pass();
        passInput.value = "";
      } catch (e) {
        unlockError.value = e.message;
      } finally {
        unlocking.value = false;
      }
    }

    /** draft · confirm · reject — one path, so every outcome is handled once. */
    async function move(ticket, action) {
      working.value = ticket.id;
      const answer = (drafts[ticket.id] || "").trim();
      try {
        const updated = await qa.act(ticket.id, action, { answer, note: answer });
        drafts[ticket.id] = updated.answer || "";
        toast(TOASTS[action](updated), { accent: action === "reject" ? "warn" : "good" });
        // The ticket the shell holds is now stale whichever action ran, so one event
        // covers all three. The ticket rides along only for a confirm, which is the one
        // the chat thread has to reflect.
        emit("changed", action === "confirm" ? updated : null);
      } catch (e) {
        fail(e);
      } finally {
        working.value = 0;
      }
    }

    /** Import .md/.txt straight into the corpus. Same gate as a confirm. */
    async function importDocs(files) {
      const { ok, rejected } = upload.sort(files);
      dragging.value = false;
      if (!ok.length) {
        toast(`<b>Nothing to import.</b> Only ${upload.ACCEPT} — convert a PDF first.`, { accent: "warn" });
        return;
      }
      importing.value = true;
      imported.value = null;
      progress.value = { done: 0, total: ok.length };
      try {
        const r = await upload.send(ok, importDir.value, (done, total) => {
          progress.value = { done, total };
        });
        // A file the browser filtered never reached the server, but to the person who
        // dropped it there is one list, so they arrive in one.
        r.failed = [...r.failed, ...rejected.map((f) => ({ name: f.name, error: `not ${upload.ACCEPT}` }))];
        imported.value = r;
        if (r.uploaded.length) {
          toast(`<b>${r.uploaded.length} document(s) indexed.</b> ${r.chunks} sections — ask about them now.`, {
            accent: "good",
          });
          emit("changed", null);
        } else {
          toast("<b>Nothing was indexed.</b> See the list below.", { accent: "crit" });
        }
      } catch (e) {
        fail(e, "The server refused the password: ");
      } finally {
        importing.value = false;
      }
    }

    /** The file picker and a drop end in the same place. */
    function pickDocs(e) {
      importDocs(e.target.files);
      e.target.value = ""; // so picking the same file twice still fires
    }

    return {
      // state the template renders
      drafts,
      working,
      unlocked,
      passInput,
      unlocking,
      unlockError,
      accept: upload.ACCEPT,
      importDir,
      importing,
      progress,
      dragging,
      imported,
      folders,
      status: qa.STATUS,
      // actions
      unlock,
      move,
      importDocs,
      pickDocs,
      shortDate,
    };
  },
};
