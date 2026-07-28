/* ══ i18n.js — the two catalogues, and which one a reader gets ═════════════════
   Plumbing: message data and one storage read. No Vue and no vue-i18n import, so this
   file still runs in a bare console and rule 9 holds — the vue-i18n *instance* is built in
   main.js (wiring) and reached through composables/lang.js (`useT`).

   The key is `lang`, which is the same key the guide's pages already write. That is the
   whole integration: switch language in the app and the published guide opens in it, and
   the reverse, because both surfaces read one string out of localStorage rather than each
   keeping its own idea of the reader.

   Keys are grouped by the screen they appear on, not by word, so a translator reads them
   in the order a user meets them. A missing key falls back to English rather than
   rendering the key itself — a blank or `empty.title` on screen is worse than a word in
   the wrong language.
   ═══════════════════════════════════════════════════════════════════════════ */

export const LANGS = ['en', 'vi']
const KEY = 'lang' // shared with the guide's pages — see the note above

/**
 * The language to start in: what was chosen last, else what the browser asks for, else
 * English. Never throws — Safari in private mode denies localStorage.
 * @returns {'en'|'vi'}
 */
export function preferredLang() {
  let stored = null
  try {
    stored = localStorage.getItem(KEY)
  }
  catch {}
  if (LANGS.includes(stored))
    return stored
  return navigator.language?.toLowerCase().startsWith('vi') ? 'vi' : 'en'
}

/** Remember the choice. Silent on failure, for the same reason. */
export function storeLang(lang) {
  try {
    localStorage.setItem(KEY, lang)
  }
  catch {}
}

export const messages = {
  en: {
    app: {
      brand: 'mega-docs',
      online: 'Server online',
      offline: 'Server unreachable',
      ask: 'ASK',
      ba: 'BA',
      admin: 'ADMIN',
      mode: 'Mode',
      newQuestion: 'New question',
      language: 'Language',
    },
    empty: {
      title: 'Ask the source of truth',
      documents: '{n} documents',
      sections: '{n} retrievable sections',
      cited: 'every claim cited',
      // `make ingest DOCS=./docs` is a command, so it is not translated — only the prose
      // around it is.
      nothingIndexed: 'Nothing is indexed yet — run {cmd}, then ask.',
      unavailable: 'Can\'t read the index. Check the server, then reload.',
      fallback: 'Grounded answers from approved docs — every claim cited.',
      filter: 'Filter documents…',
      filterLabel: 'Filter the indexed documents',
      showing: 'Showing {shown} of {total} indexed documents',
      askWhat: 'Ask what {title} covers',
      moreMatch: '{n} more match — type to narrow.',
      noMatch: 'No indexed document matches “{q}”.',
      askedBefore: 'Asked before ({n}) — free to repeat',
      wholeCorpus: 'whole corpus',
      freeRepeats: '{n} free repeats so far',
      withBa: 'Questions with a BA ({n})',
    },
  },
  vi: {
    app: {
      brand: 'mega-docs',
      online: 'Server đang chạy',
      offline: 'Không kết nối được server',
      ask: 'HỎI',
      ba: 'BA',
      admin: 'ADMIN',
      mode: 'Chế độ',
      newQuestion: 'Câu hỏi mới',
      language: 'Ngôn ngữ',
    },
    empty: {
      title: 'Hỏi nguồn sự thật',
      documents: '{n} tài liệu',
      sections: '{n} mục truy xuất được',
      cited: 'mọi câu đều dẫn nguồn',
      nothingIndexed: 'Chưa index gì cả — chạy {cmd}, rồi hỏi.',
      unavailable: 'Không đọc được index. Kiểm tra server rồi tải lại.',
      fallback: 'Câu trả lời dựa trên tài liệu đã duyệt — mọi câu đều dẫn nguồn.',
      filter: 'Lọc tài liệu…',
      filterLabel: 'Lọc các tài liệu đã index',
      showing: 'Đang hiện {shown} trong {total} tài liệu đã index',
      askWhat: 'Hỏi {title} nói về gì',
      moreMatch: 'Còn {n} kết quả nữa — gõ để lọc hẹp lại.',
      noMatch: 'Không có tài liệu nào khớp “{q}”.',
      askedBefore: 'Đã hỏi trước đó ({n}) — hỏi lại miễn phí',
      wholeCorpus: 'toàn bộ corpus',
      freeRepeats: 'đã hỏi lại miễn phí {n} lần',
      withBa: 'Câu hỏi đang chờ BA ({n})',
    },
  },
}
