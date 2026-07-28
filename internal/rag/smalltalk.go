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
	q := strings.ToLower(strings.TrimSpace(question))
	q = strings.TrimRight(q, " .!?…,;:")
	if q == "" {
		return "", false
	}
	vi := viLetters.MatchString(q)
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
	case notSmallTalk:
		return ""
	}
	return ""
}
