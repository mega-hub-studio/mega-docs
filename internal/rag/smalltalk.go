package rag

import (
	"fmt"
	"regexp"
	"strings"

	"knowledge-engine/internal/db"
)

// Conversational turns that are not questions about the documents.
//
// "xin chào" used to return `NoAnswer` — "Không tìm thấy thông tin này trong tài liệu." —
// after a full embed + retrieve + completion, about 5.7 seconds and a real provider bill,
// to tell a person who said hello that the documents do not cover them. That reads as a
// broken assistant, and it is the first thing anyone types.
//
// It is answered *here*, before retrieval, for three reasons that all matter:
//
//   - It cannot hallucinate. The reply is a constant, so a greeting can never turn into
//     an invented fact about a booking flow — which is what asking the model to "be
//     conversational when the documents are silent" would risk on every question.
//   - It costs nothing: no embedding, no completion, no cache entry. Microseconds.
//   - The grounding rules stay exactly as strict as they were. This is not a relaxation
//     of "answer only from the CONTEXT"; it is the recognition that a greeting was never
//     a question about the CONTEXT, so the rule never applied to it.
//
// What this deliberately does NOT do is answer a real question the documents miss. That
// still returns NoAnswer and still routes to a BA, because a plausible sentence about an
// organisation's own process, written by a model that has never read their documents, is
// the exact failure this product exists to prevent.

// smallTalkKind is what a non-document turn is asking for. Split by kind rather than one
// catch-all reply, because "hello" and "what can you do" want different sentences.
type smallTalkKind int

const (
	notSmallTalk smallTalkKind = iota
	greeting
	thanks
	identity
	capability
	runtimeMeta
	tooVague
)

// Matched against the whole question, lowercased and trimmed of punctuation. Anchored on
// purpose: "chào" alone is a greeting, but "quy trình chào giá" is a real question about a
// pricing flow, and a substring match would swallow it.
//
// Bilingual because the app is (both catalogues live in web/ui/src/lib/i18n.js), and
// because the reply below answers in the language it was asked in.
var smallTalkPatterns = []struct {
	kind smallTalkKind
	re   *regexp.Regexp
}{
	{greeting, regexp.MustCompile(`^(xin ch(a|à)o|ch(a|à)o( b(a|ạ)n| anh| ch(i|ị))?|hi|hello|hey|yo|good (morning|afternoon|evening))$`)},
	{thanks, regexp.MustCompile(`^(c(a|á)m ơn|c(a|ả)m ơn( b(a|ạ)n)?|thanks|thank you|thx|ok|okay|được rồi)$`)},
	{identity, regexp.MustCompile(`^(b(a|ạ)n l(a|à) ai|ai đ(a|ấ)y|who are you|what are you|introduce yourself)$`)},
	{capability, regexp.MustCompile(`^(b(a|ạ)n l(a|à)m đư(o|ợ)c g(i|ì)|gi(u|ú)p g(i|ì)|help|what can you do|how do i use (this|you))$`)},
	// The model is the one piece of runtime the app *already publishes*: /api/health reports
	// it and the status line prints it under every answer. Refusing to name it while it is
	// on screen reads as broken, and it cost a completion each time — measured at 1414ms on
	// the deployed instance for "model gì", with the answer visible two centimetres below.
	{runtimeMeta, regexp.MustCompile(`^((d(u|ù)ng |b(a|ạ)n d(u|ù)ng )?model (g(i|ì)|n(a|à)o)|what model( (are you|do you use))?|which model)$`)},
	// A question with no content word in it — pure interrogative or filler. Retrieval has
	// nothing to match, so the reply used to be the no-answer sentence, which blames the
	// documents for a question that was never asked. Asking back is both cheaper and the
	// only honest response.
	//
	// This list is short and *whole-string* anchored on purpose, and that restraint is the
	// entire correctness argument. A single word is very often a perfectly good query here —
	// "hoàn tiền", "TOP_K", "BA_PASS", "RRF" — because half of retrieval is keyword matching.
	// Guessing at vagueness by length, or by "fewer than three words", would swallow exactly
	// the lookups this engine is best at. Only a question that contains *no* content word at
	// all belongs here.
	{tooVague, regexp.MustCompile(`^(l(a|à)m sao|l(a|à)m th(e|ế) n(a|à)o|th(e|ế) n(a|à)o|nh(u|ư) th(e|ế) n(a|à)o|sao|sao v(a|ậ)y|v(a|ậ)y|v(a|ậ)y (a|à)|(c(a|á)i )?g(i|ì)|c(a|á)i (d|đ)(o|ó)|n(o|ó) l(a|à) g(i|ì)|(the )?what|what\?*|how|how\?*|why|huh|hmm|test|\?+|help me|t(o|ô)i mu(o|ố)n bi(e|ế)t)$`)},
}

// Vietnamese if the question carries a Vietnamese-only letter, English otherwise. Crude on
// purpose: the alternative is a language-detection dependency to choose between two
// constant strings, and the only cost of being wrong on a bare "hi" is answering a
// greeting in English.
var viLetters = regexp.MustCompile(`[ăâđêôơưàáảãạằắẳẵặầấẩẫậèéẻẽẹềếểễệìíỉĩịòóỏõọồốổỗộờớởỡợùúủũụừứửữựỳýỷỹỵ]`)

// smallTalk reports the reply for a conversational turn, and whether this was one.
//
// The replies name what the assistant is for and how to get a good answer out of it,
// because a greeting is the one moment someone is definitely reading. Nothing here claims
// anything about the corpus's *contents* — the first screen already lists those, measured,
// and a sentence invented here could disagree with it.
//
// inThread says there is a conversation behind this question, and it changes exactly one
// verdict. Vagueness is a property of a question *plus* what came before it: "cái đó" or a
// bare "how?" carries no content word, but the turn above it does, so asking back is the
// wrong answer — the rewrite in memory.go resolves it against that turn and retrieval runs
// on the result. Every other kind is unaffected: "cảm ơn" is thanks on the tenth message
// as much as on the first.
func smallTalk(question string, inThread bool) (string, bool) {
	raw := strings.ToLower(strings.TrimSpace(question))
	if raw == "" {
		return "", false
	}
	q := strings.TrimRight(raw, " .!?…,;:")
	kind := smallTalkKindOf(q)
	if kind == notSmallTalk || (kind == tooVague && inThread) {
		return "", false
	}
	return smallTalkReply(kind, viLetters.MatchString(raw)), true
}

// smallTalkKindOf classifies a question already lowercased and stripped of its trailing
// punctuation.
//
// Trimming that punctuation is what lets "hello!" match "hello" — but it also eats a
// question that is *only* punctuation, and "?" on its own left an empty string that fell
// through to retrieval and came back as the no-answer sentence. Punctuation with nothing in
// front of it is the vaguest question there is.
func smallTalkKindOf(q string) smallTalkKind {
	if q == "" {
		return tooVague
	}
	for _, p := range smallTalkPatterns {
		if p.re.MatchString(q) {
			return p.kind
		}
	}
	return notSmallTalk
}

func smallTalkReply(kind smallTalkKind, vi bool) string {
	switch kind {
	case greeting:
		if vi {
			return "Chào bạn. Tôi trả lời dựa trên tài liệu nội bộ đã được duyệt của tổ chức bạn, " +
				"và **mọi câu đều dẫn nguồn** về đúng file đã nói ra điều đó.\n\n" +
				"Hỏi hiệu quả nhất là dùng đúng từ có trong tài liệu — mã lỗi, tên config, mã quy " +
				"định — vì một nửa việc truy xuất là khớp từ khoá. Bạn cũng có thể bấm một tài liệu " +
				"ở màn hình đầu để hỏi tài liệu đó nói về gì.\n\n" +
				"Nếu tài liệu chưa có câu trả lời, tôi sẽ nói thẳng là chưa có, chứ không đoán — " +
				"và bạn gửi được câu hỏi đó cho BA ngay trong màn hình trả lời."
		}
		return "Hello. I answer from your organisation's own approved documents, and **every claim " +
			"is cited** back to the file that said it.\n\n" +
			"The best questions use the words the documents use — an error code, a config key, a " +
			"rule id — because half of retrieval is keyword matching. You can also tap a document " +
			"on the first screen to ask what it covers.\n\n" +
			"When the documents do not answer something I will say so rather than guess, and you " +
			"can send that question to a BA from the answer itself."
	case thanks:
		if vi {
			return "Rất vui được giúp. Cứ hỏi tiếp khi cần."
		}
		return "Glad to help. Ask whenever you need to."
	case identity:
		if vi {
			return "Tôi là trợ lý tra cứu trên **tài liệu của chính tổ chức bạn** — kỹ thuật, sản " +
				"phẩm, nghiệp vụ, hỗ trợ. Tôi chỉ trả lời từ những gì đã được index và duyệt, và dẫn " +
				"nguồn từng câu.\n\n" +
				"Điều đó cũng có nghĩa: tôi không phải một chatbot kiến thức chung. Câu nào tài liệu " +
				"không có, tôi nói là không có."
		}
		return "I am a lookup assistant over **your organisation's own documents** — engineering, " +
			"product, business and support. I answer only from what has been indexed and approved, " +
			"and I cite every claim.\n\n" +
			"Which also means: I am not a general-knowledge chatbot. If the documents do not cover " +
			"something, I say so."
	case capability:
		if vi {
			return "Ba việc:\n\n" +
				"1. **Trả lời có dẫn nguồn** từ tài liệu đã duyệt — mỗi `[n]` là link về đúng file.\n" +
				"2. **Giới hạn theo thư mục** khi bạn chỉ muốn câu trả lời trong một phạm vi.\n" +
				"3. **Chuyển câu hỏi cho BA** khi tài liệu còn thiếu — BA trả lời, câu đó được ghi " +
				"thành tài liệu mới và index lại, nên lần sau ai hỏi cũng có.\n\n" +
				"Hỏi lại đúng một câu đã hỏi thì miễn phí: câu trả lời được cache theo corpus."
		}
		return "Three things:\n\n" +
			"1. **Cited answers** from approved documents — each `[n]` links to the file it came from.\n" +
			"2. **Scoped answers**, when you want only one folder to answer.\n" +
			"3. **A route to a BA** when the documents are missing something — the BA's answer is " +
			"written back as a document and indexed, so the next person just gets it.\n\n" +
			"Asking the exact same question again is free: answers are cached against the corpus."
	case runtimeMeta:
		// Pointing at the status line rather than repeating the name: the model is
		// config (CHAT_MODEL), it reaches the UI through /api/health, and a second
		// spelling of it here is a second thing to keep in step with the first.
		if vi {
			return "Model đang dùng được ghi ở **status line** dưới đáy màn hình, cạnh số token " +
				"và chi phí của câu trả lời vừa rồi — nó đọc từ `/api/health`, nên luôn là model " +
				"instance này thực sự gọi, không phải một cái tên viết cứng ở đâu đó.\n\n" +
				"Đổi model là đổi `CHAT_MODEL` trong `.env` rồi restart. Lưu ý: model nằm trong " +
				"cache signature, nên đổi model làm mất hiệu lực toàn bộ câu trả lời đã cache — " +
				"đúng như vậy, vì một câu trả lời do model khác sinh ra là một câu trả lời khác."
		}
		return "The model in use is on the **status line** at the bottom of the screen, next to " +
			"the token count and cost of the last answer — it comes from `/api/health`, so it is " +
			"always the model this instance actually calls rather than a name written down " +
			"somewhere.\n\n" +
			"To change it, set `CHAT_MODEL` in `.env` and restart. Note that the model is part of " +
			"the cache signature, so changing it invalidates every cached answer — which is " +
			"correct: an answer produced by a different model is a different answer."
	case tooVague:
		// Ask back, and make the asking useful: say what would make it answerable, in the
		// vocabulary this corpus rewards. A bare "could you clarify?" spends a turn and
		// teaches nothing; naming the shape of a good question means the next attempt lands.
		if vi {
			return "Bạn muốn biết cụ thể điều gì? Câu vừa rồi chưa đủ để tôi biết nên tìm ở đâu.\n\n" +
				"Hiệu quả nhất là nêu **đúng từ có trong tài liệu**: một mã lỗi, tên config, mã quy " +
				"định, hoặc tên nghiệp vụ. Ví dụ:\n\n" +
				"- *thay vì* \"làm sao\" → \"làm sao huỷ booking đã thanh toán\"\n" +
				"- *thay vì* \"cái đó\" → \"quy trình hoàn tiền cho booking huỷ muộn\"\n\n" +
				"Chưa biết bắt đầu từ đâu thì bấm một tài liệu ở màn hình đầu — tôi sẽ nói tài liệu " +
				"đó bao gồm những gì, rồi bạn hỏi sâu vào đúng phần cần."
		}
		return "Could you be more specific? There is not enough in that to know where to look.\n\n" +
			"What works best is **the words the documents use**: an error code, a config key, a " +
			"rule id, or the name of the process. For example:\n\n" +
			"- *instead of* \"how\" → \"how do I cancel a paid booking\"\n" +
			"- *instead of* \"that thing\" → \"the refund process for a late cancellation\"\n\n" +
			"If you are not sure where to start, tap a document on the first screen — I will tell " +
			"you what it covers, and you can go deeper from there."
	case notSmallTalk:
		return ""
	}
	return ""
}

// ── a question about the library, rather than about what is in it ──────────────
//
// Same file as small talk because it is the same move: a turn recognised *before*
// retrieval and answered without buying anything. It is not the same reply, though, and
// the difference is the whole reason this exists. Small talk answers with a constant, so
// it cannot be wrong. This answers from rows, so it cannot be *stale* — which matters,
// because the one thing a constant could never say is what changed yesterday.
//
// The failure it replaces was silent. Retrieval ranks by meaning and keywords, and
// "recently" carries neither, so "các QA đã chốt confirm gần đây" embedded to nothing in
// particular, matched whatever happened to rank, and came back with citations under it.
// Confident and arbitrary is the worst shape an answer can have here.
//
// This does not contradict the note above about not describing the corpus. That note is
// about a sentence *invented* here disagreeing with the measured first screen. These rows
// are that same measurement, read at the same moment — nothing is invented, so there is
// nothing to disagree with.
type corpusAsk int

const (
	notCorpusAsk corpusAsk = iota
	recentDocs
	recentQA
)

// recentLimit is how many rows a recency answer shows. Ten because the question is "what
// changed", not "list the library" — the library screen already lists the library, and a
// fiftieth-newest document answers nobody's question about what is new.
const recentLimit = 10

// Whole-string anchored, exactly like smallTalkPatterns above and for exactly that reason:
// "quy trình hoàn tiền gần đây có đổi gì không" is a real question about the documents, and
// a substring match on "gần đây" would swallow it and answer with a file listing.
//
// Diacritics are optional the same way they are up there — people type "tai lieu nao moi
// cap nhat" — but nothing here is loosened past a whole utterance.
var corpusAskPatterns = []struct {
	ask corpusAsk
	re  *regexp.Regexp
}{
	{recentQA, regexp.MustCompile(`^(c(a|á)c |nh(u|ữ)ng )?(qa|q&a|c(a|â)u h(o|ỏ)i|c(a|â)u tr(a|ả) l(o|ờ)i)( n(a|à)o)?( (d|đ)(a|ã)( (d|đ)ư(o|ợ)c)?)? (ch(o|ố)t|confirm(ed)?|duy(e|ệ)t)( confirm(ed)?)?( r(o|ồ)i)?( g(a|ầ)n (d|đ)(a|â)y| m(o|ớ)i nh(a|ấ)t| m(o|ớ)i)?$`)},
	{recentQA, regexp.MustCompile(`^((recently |newly )?confirmed (qa|q&a|answers)|which (qa|answers) (were|was) confirmed( recently)?|what (qa|answers) (were|was) confirmed( recently)?)$`)},
	{recentDocs, regexp.MustCompile(`^(c(a|á)c |nh(u|ữ)ng )?t(a|à)i li(e|ệ)u( n(a|à)o)?( (m(o|ớ)i|v(u|ừ)a))? (c(a|ậ)p nh(a|ậ)t|thay (d|đ)(o|ổ)i|s(u|ử)a)( g(a|ầ)n (d|đ)(a|â)y| m(o|ớ)i nh(a|ấ)t)?$`)},
	{recentDocs, regexp.MustCompile(`^((g(a|ầ)n (d|đ)(a|â)y )?c(o|ó) t(a|à)i li(e|ệ)u n(a|à)o m(o|ớ)i( kh(o|ô)ng)?|t(a|à)i li(e|ệ)u n(a|à)o m(o|ớ)i nh(a|ấ)t)$`)},
	{recentDocs, regexp.MustCompile(`^(which documents were updated( recently)?|what documents (changed|were updated)( recently)?|(recently )?updated documents|what'?s new( in the library)?|what is new( in the library)?)$`)},
}

// corpusAskOf reports which library question this is, on a string already lowercased and
// stripped of trailing punctuation — the same input smallTalkKindOf takes, so the two
// classifiers cannot disagree about what the question even was.
func corpusAskOf(q string) corpusAsk {
	for _, p := range corpusAskPatterns {
		if p.re.MatchString(q) {
			return p.ask
		}
	}
	return notCorpusAsk
}

// answeredEarly is every turn whose reply is known before retrieval starts: a greeting, a
// vague question asked back, and a question about the library. One door rather than two,
// because they are one idea from Answer()'s side — nothing is bought, and the reply is
// already here — and because Answer() is at gocyclo's ceiling, where a second branch would
// cost the next feature its own.
//
// Both classifiers read the same normalised string, so they cannot disagree about what the
// question even was.
func (e *Engine) answeredEarly(question string, inThread bool, onToken func(string)) bool {
	if reply, ok := smallTalk(question, inThread); ok {
		onToken(reply)
		return true
	}
	asked := strings.TrimRight(strings.ToLower(strings.TrimSpace(question)), " .!?…,;:")
	return e.recent(asked, question, onToken)
}

// recent answers a question about the library from its rows, and reports whether this was
// one.
//
// A store that will not read falls through to retrieval rather than failing the question.
// Retrieval answers this badly — which is the entire reason this function exists — but badly
// beats an error on the one screen somebody is looking at.
func (e *Engine) recent(asked, question string, onToken func(string)) bool {
	ask := corpusAskOf(asked)
	if ask == notCorpusAsk {
		return false
	}
	docs, err := e.store.RecentDocuments(ask.prefix(), recentLimit)
	if err != nil {
		return false
	}
	onToken(renderRecent(docs, ask, viLetters.MatchString(question)))
	return true
}

// prefix is the folder a library question is about. A confirmed answer is a document under
// qa/ and nothing else is, so the filter is the same string a citation prints — there is no
// second place that records which documents came from the QA loop.
func (a corpusAsk) prefix() string {
	if a == recentQA {
		return "qa"
	}
	return ""
}

// renderRecent writes the answer: a lead sentence, a table, and one line saying where the
// numbers came from.
//
// That last line is not decoration. Every other answer on this screen carries [n] citations,
// so an answer with none reads as ungrounded unless it says what it *is* — and "read from
// the library" is a stronger provenance than a citation, not a weaker one.
func renderRecent(docs []db.Document, ask corpusAsk, vi bool) string {
	if len(docs) == 0 {
		switch {
		case ask == recentQA && vi:
			return "Chưa có câu trả lời nào được chốt vào thư viện."
		case ask == recentQA:
			return "No answers have been confirmed into the library yet."
		case vi:
			return "Thư viện chưa có tài liệu nào."
		default:
			return "The library has no documents yet."
		}
	}

	var b strings.Builder
	switch {
	case ask == recentQA && vi:
		fmt.Fprintf(&b, "**%d câu trả lời được chốt gần nhất**\n\n", len(docs))
	case ask == recentQA:
		fmt.Fprintf(&b, "**The %d most recently confirmed answers**\n\n", len(docs))
	case vi:
		fmt.Fprintf(&b, "**%d tài liệu được cập nhật gần nhất**\n\n", len(docs))
	default:
		fmt.Fprintf(&b, "**The %d most recently updated documents**\n\n", len(docs))
	}

	if vi {
		b.WriteString("| # | Tài liệu | Loại | Cập nhật |\n|---|---|---|---|\n")
	} else {
		b.WriteString("| # | Document | Kind | Updated |\n|---|---|---|---|\n")
	}
	for i, d := range docs {
		kind := d.Kind
		if kind == "" {
			kind = "—"
		}
		fmt.Fprintf(&b, "| %d | `%s` | %s | %s |\n", i+1, d.Path, kind, when(d.UpdatedAt))
	}

	if vi {
		b.WriteString("\nĐọc trực tiếp từ thư viện, mới nhất trước — không phải kết quả tìm kiếm, " +
			"nên không có dẫn nguồn và cũng không được cache.")
	} else {
		b.WriteString("\nRead straight from the library, newest first — not a search result, so " +
			"there are no citations and nothing here is cached.")
	}
	return b.String()
}

// when trims SQLite's `datetime('now')` to the minute. Seconds on a document's update time
// is a precision nobody asked for and one more column's worth of width on a phone.
func when(ts string) string {
	if len(ts) >= 16 {
		return ts[:16]
	}
	return ts
}
