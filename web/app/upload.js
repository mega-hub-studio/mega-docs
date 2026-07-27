/* ══ upload.js — importing documents into the corpus ══════════════════════════
   Hides: multipart assembly, the password header, and the one response shape the
   app must not treat as a plain failure — a 400 whose body still lists which file
   was rejected and why.

     const { uploaded, failed, chunks } = await send(fileList);

   Same password as a confirm, because both change what every reader is told. The
   filter runs here too, so dropping a folder of PDFs costs one message instead of
   one upload.
   ═══════════════════════════════════════════════════════════════════════════ */

import { pass, setPass, WrongPass } from "./qa.js";

/** What the file picker offers, and what a drop is filtered against. */
export const ACCEPT = ".md,.markdown,.txt";

const EXTS = [".md", ".markdown", ".txt"];

/** Split a drop into what can be sent and what cannot, so the UI can say both. */
export function sort(files) {
  const ok = [];
  const rejected = [];
  for (const f of [...files]) {
    (EXTS.some((e) => f.name.toLowerCase().endsWith(e)) ? ok : rejected).push(f);
  }
  return { ok, rejected };
}

/**
 * Import documents. Resolves with the per-file outcome even when the server
 * refused every one of them — the caller renders the same list either way.
 * @throws {WrongPass} when the password is missing, wrong, or writes are off
 * @returns {Promise<{uploaded: {path: string, chunks: number}[], failed: {name: string, error: string}[], chunks: number}>}
 */
export async function send(files) {
  const body = new FormData();
  for (const f of files) body.append("files", f, f.name);

  let res;
  try {
    res = await fetch("/api/documents", { method: "POST", headers: { "X-BA-Pass": pass() }, body });
  } catch {
    throw new Error("Can't reach the server");
  }
  if (res.status === 401 || res.status === 403) {
    setPass("");
    throw new WrongPass((await res.text()).trim() || "Not allowed");
  }
  // 400 with a JSON body means "nothing usable", and it still names each file.
  // Throwing that away would leave the user with "Server error 400" for a batch
  // whose problem is one line long.
  if (res.headers.get("Content-Type")?.includes("application/json")) {
    return res.json();
  }
  throw new Error((await res.text()).trim() || `Server error ${res.status}`);
}
