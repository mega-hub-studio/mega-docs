/* ══ ba.js — the BA screen: the ticket queue and the import card ═══════════════
   Its own component because its state is its own: 13 of the app's 22 reactive keys
   belonged only to this screen, sharing one object with the chat thread. Now the
   import progress bar and the conversation cannot collide.

   In-DOM template (#ba-screen in index.html), not an SFC — the pinned Vue global
   build ships the compiler, so components cost no build step here.

   Crossings, kept to three:
     props   writes · online · queue · documents   all of which the ASK screen also
             renders, so they belong to the shell rather than here
     emit    changed(ticket|null)  something moved: the shell refreshes queue, corpus
             and history, and updates the turn badge when a ticket came with it
     inject  toast                 the design-system helper, provided by boot()
   ═══════════════════════════════════════════════════════════════════════════ */
import * as qa from "./qa.js";
import * as upload from "./upload.js";
import { shortDate } from "./library.js";

/* What each transition means, said once. A confirm is the only one worth
   celebrating: it is the moment a gap became part of the documents. */
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

  data() {
    return {
      drafts: {}, // ticket id → the answer being typed, kept out of the server copy
      working: 0, // id of the ticket currently being published
      unlocked: !!qa.pass(),
      passInput: "",
      unlocking: false,
      unlockError: "",

      /* ── importing documents ── */
      accept: upload.ACCEPT,
      importDir: "", // the folder this batch lands in — the scope a reader browses
      importing: false,
      progress: { done: 0, total: 0 }, // real counts — the bar must not invent a position
      dragging: false, // a drop target that doesn't light up reads as inert
      imported: null, // {uploaded, failed, chunks} from the last import
      status: qa.STATUS,
    };
  },

  /* Seed each editor from the server's copy, and keep doing it as the queue
     refreshes — without overwriting an answer someone is halfway through typing. */
  watch: {
    queue: {
      immediate: true,
      handler(q) {
        for (const t of q.tickets) {
          if (this.drafts[t.id] === undefined) this.drafts[t.id] = t.answer || "";
        }
      },
    },
  },

  computed: {
    /** The folders that already exist, so the import picker suggests the structure
     *  rather than inviting a fourth spelling of "engineering". */
    folders() {
      return upload.folders(this.documents);
    },
  },

  methods: {
    shortDate,

    /** Check the password before saying it worked. Storing it unchecked is how a
     *  typo used to survive until the first upload, and then look like a broken
     *  import rather than a wrong password. */
    async unlock() {
      const candidate = this.passInput.trim();
      if (!candidate) return;
      this.unlockError = "";
      this.unlocking = true;
      try {
        if (!(await upload.verify(candidate))) {
          this.unlockError = "That password does not open the gate. Reads still work.";
          return;
        }
        qa.setPass(candidate);
        this.unlocked = !!qa.pass();
        this.passInput = "";
      } catch (e) {
        this.unlockError = e.message;
      } finally {
        this.unlocking = false;
      }
    },

    /** draft · confirm · reject — one path, so every outcome is handled once. */
    async move(ticket, action) {
      this.working = ticket.id;
      const answer = (this.drafts[ticket.id] || "").trim();
      try {
        const updated = await qa.act(ticket.id, action, { answer, note: answer });
        this.drafts[ticket.id] = updated.answer || "";
        this.toast(TOASTS[action](updated), { accent: action === "reject" ? "warn" : "good" });
        // The ticket the shell holds is now stale whichever action ran, so one event
        // covers all three. The ticket rides along only for a confirm, which is the
        // one the chat thread has to reflect.
        this.$emit("changed", action === "confirm" ? updated : null);
      } catch (e) {
        this.fail(e);
      } finally {
        this.working = 0;
      }
    },

    /** Import .md/.txt straight into the corpus. Same gate as a confirm. */
    async importDocs(files) {
      const { ok, rejected } = upload.sort(files);
      this.dragging = false;
      if (!ok.length) {
        this.toast(`<b>Nothing to import.</b> Only ${upload.ACCEPT} — convert a PDF first.`, { accent: "warn" });
        return;
      }
      this.importing = true;
      this.imported = null;
      this.progress = { done: 0, total: ok.length };
      try {
        const r = await upload.send(ok, this.importDir, (done, total) => {
          this.progress = { done, total };
        });
        // A file the browser filtered never reached the server, but to the person
        // who dropped it there is one list, so they arrive in one.
        r.failed = [...r.failed, ...rejected.map((f) => ({ name: f.name, error: `not ${upload.ACCEPT}` }))];
        this.imported = r;
        if (r.uploaded.length) {
          this.toast(
            `<b>${r.uploaded.length} document(s) indexed.</b> ${r.chunks} sections — ask about them now.`,
            { accent: "good" },
          );
          this.$emit("changed", null);
        } else {
          this.toast("<b>Nothing was indexed.</b> See the list below.", { accent: "crit" });
        }
      } catch (e) {
        this.fail(e, "The server refused the password: ");
      } finally {
        this.importing = false;
      }
    },

    /** The file picker and a drop end in the same place. */
    pickDocs(e) {
      this.importDocs(e.target.files);
      e.target.value = ""; // so picking the same file twice still fires
    },

    /** A refused password has to say so *in the form*, not only in a toast: the
     *  thing being read is the card that just vanished. */
    fail(e, prefix = "") {
      if (e instanceof qa.WrongPass) {
        this.unlocked = false;
        if (prefix) this.unlockError = prefix + e.message;
        this.toast(`<b>Locked.</b> ${e.message}`, { accent: "crit" });
      } else {
        this.toast(`<b>${e.message}</b>`, { accent: "crit" });
      }
    },
  },
};
