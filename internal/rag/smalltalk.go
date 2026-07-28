package rag

import (
	"regexp"
	"strings"
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
func smallTalk(question string) (string, bool) {
	raw := strings.ToLower(strings.TrimSpace(question))
	if raw == "" {
		return "", false
	}
	q := strings.TrimRight(raw, " .!?…,;:")
	vi := viLetters.MatchString(raw)
	// Trimming the trailing punctuation is what lets "hello!" match "hello" — but it also
	// eats a question that is *only* punctuation, and "?" on its own left an empty string
	// that fell through to retrieval and came back as the no-answer sentence. Punctuation
	// with nothing in front of it is the vaguest question there is.
	if q == "" {
		return smallTalkReply(tooVague, vi), true
	}
	for _, p := range smallTalkPatterns {
		if !p.re.MatchString(q) {
			continue
		}
		return smallTalkReply(p.kind, vi), true
	}
	return "", false
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
