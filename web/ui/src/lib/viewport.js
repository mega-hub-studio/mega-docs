/* ══ viewport.js — the mobile viewport plumbing ════════════════════════════════
   Hides: ResizeObserver, visualViewport, the scroll listener, and the two CSS
   custom properties the layout reads (--dock-h, --kb).

     const view = bindViewport(dockEl);
     view.scrollToEnd();                 // follows the stream, unless the reader
     view.scrollToEnd({ force: true });  // scrolled up to re-read something

   The app never asks "is the user at the bottom" or "how tall is the keyboard".
   ═══════════════════════════════════════════════════════════════════════════ */

/** Below this many px from the bottom, we're "following" the answer. */
const FOLLOW_SLOP = 120;

/**
 * @param {HTMLElement} dock the fixed prompt container
 * @returns {{ scrollToEnd: (opts?: { force?: boolean, smooth?: boolean }) => void }}
 */
export function bindViewport(dock) {
  let following = true;

  trackDockHeight(dock);
  trackKeyboard();

  addEventListener(
    "scroll",
    () => {
      const gap = document.documentElement.scrollHeight - (scrollY + innerHeight);
      following = gap < FOLLOW_SLOP;
    },
    { passive: true },
  );

  return {
    scrollToEnd({ force = false, smooth = false } = {}) {
      if (force) following = true;
      else if (!following) return;
      requestAnimationFrame(() =>
        scrollTo({ top: document.body.scrollHeight, behavior: smooth ? "smooth" : "instant" }),
      );
    },
  };
}

/* --dock-h: the prompt's real height. main's bottom padding and the anchor
   scroll-padding are calc'd off it, so both track the textarea as it grows
   instead of reserving a guessed constant. */
function trackDockHeight(dock) {
  const set = () => css("--dock-h", `${dock.offsetHeight}px`);
  new ResizeObserver(set).observe(dock);
  set();
}

/* --kb: how much of the viewport the on-screen keyboard covers. iOS doesn't
   resize the layout viewport, so a `position: fixed` dock would sit *behind* the
   keyboard; visualViewport is the only reliable read. Stays 0px everywhere else,
   which makes the transform in styles.css a no-op on desktop. */
function trackKeyboard() {
  const vv = visualViewport;
  if (!vv) return;
  const set = () => css("--kb", `${Math.max(0, innerHeight - vv.height - vv.offsetTop)}px`);
  vv.addEventListener("resize", set);
  vv.addEventListener("scroll", set);
  set();
}

function css(prop, value) {
  document.documentElement.style.setProperty(prop, value);
}
