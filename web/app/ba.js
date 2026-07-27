/* ══ ba.js — the BA screen ═════════════════════════════════════════════════════
   A headless component: it owns no logic. Its whole job is to declare what it needs from
   the shell, compose the three behaviours behind it, and hand the result to the template.
   Anything with a branch in it lives in use/ — the gate (one password, two actions), the
   importer (files, progress, results), the tickets (four states, one path).

   Read as: what comes in, what goes out, what it is made of.

     props   writes · online · queue · documents   the ASK screen renders these too, so
             they belong to the shell rather than here
     emit    changed(ticket|null)  something moved: the shell refreshes queue, corpus and
             history, and updates the turn badge when a ticket came with it
     emit    ask                   take me back to the chat

   In-DOM template (#ba-screen in index.html), not an SFC — the pinned Vue global build
   ships the compiler, so a component costs no build step here.
   ═══════════════════════════════════════════════════════════════════════════ */
import { STATUS } from "./qa.js";
import { shortDate } from "./library.js";
import { useToast } from "./use/toast.js";
import { useGate } from "./use/gate.js";
import { useImporter } from "./use/importer.js";
import { useTickets } from "./use/tickets.js";

export const BaScreen = {
  name: "BaScreen",
  template: "#ba-screen",
  props: {
    writes: Boolean, // does this instance allow a BA to publish at all
    online: Boolean, // unreachable is not read-only, and must not read as it
    queue: { type: Object, required: true }, // the ASK screen lists it too
    documents: { type: Array, default: () => [] },
  },
  emits: ["changed", "ask"],

  setup(props, { emit }) {
    const toast = useToast();
    const gate = useGate({ toast });
    const tickets = useTickets({
      tickets: () => props.queue.tickets,
      toast,
      onMoved: (ticket) => emit("changed", ticket),
      onLocked: gate.fail,
    });
    const importer = useImporter({
      documents: () => props.documents,
      toast,
      onIndexed: () => emit("changed", null),
      onLocked: (e) => gate.fail(e, "The server refused the password: "),
    });

    return { ...gate, ...tickets, ...importer, status: STATUS, shortDate };
  },
};
