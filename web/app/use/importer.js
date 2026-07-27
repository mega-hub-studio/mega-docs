/* ══ use/importer.js — dropping documents into the corpus ══════════════════════
   The whole import, minus the markup: which files were accepted, where they land, how far
   along it is, and what came back.

   Two things here are load-bearing rather than decorative:

     progress   real counts, because one request per file is what makes the bar *true*.
                A single POST for the batch could only be animated by guessing, and a bar
                that invents its position claims "nearly done" while the last file has
                not started.
     imported   partial success, reported. Eight files where one is a PDF is seven
                indexed and the eighth named — in one list, because to the person who
                dropped them there was one list.
   ═══════════════════════════════════════════════════════════════════════════ */
import * as upload from "../upload.js";

/**
 * @param {{ documents: () => object[], toast: Function, onLocked: (e: Error) => void,
 *   onIndexed: () => void }} deps
 *   documents is a getter, not an array: the folder suggestions follow the corpus, which
 *   the shell replaces wholesale after every import.
 */
export function useImporter({ documents, toast, onLocked, onIndexed }) {
  const { ref, computed } = Vue;

  const importDir = ref(""); // the folder this batch lands in — the scope a reader browses
  const importing = ref(false);
  const progress = ref({ done: 0, total: 0 });
  const dragging = ref(false); // a drop target that doesn't light up reads as inert
  const imported = ref(null); // {uploaded, failed, chunks} from the last import

  /** The folders that already exist, so the picker suggests the structure rather than
   *  inviting a fourth spelling of "engineering". */
  const folders = computed(() => upload.folders(documents()));

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
      r.failed = [...r.failed, ...rejected.map((f) => ({ name: f.name, error: `not ${upload.ACCEPT}` }))];
      imported.value = r;
      if (r.uploaded.length) {
        toast(`<b>${r.uploaded.length} document(s) indexed.</b> ${r.chunks} sections — ask about them now.`, {
          accent: "good",
        });
        onIndexed();
      } else {
        toast("<b>Nothing was indexed.</b> See the list below.", { accent: "crit" });
      }
    } catch (e) {
      onLocked(e);
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
    accept: upload.ACCEPT,
    importDir,
    importing,
    progress,
    dragging,
    imported,
    folders,
    importDocs,
    pickDocs,
  };
}
