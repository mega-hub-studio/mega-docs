package rag

import "strings"

import "testing"

// The failure this exists to prevent: "xin chào" answered with the no-answer sentence,
// after a full embed + retrieve + completion.
func TestAGreetingIsNotAMissingDocument(t *testing.T) {
	for _, q := range []string{"xin chào", "Xin chào!", "chào bạn", "hello", "Hi", "hey", "thanks", "cảm ơn"} {
		got, ok := smallTalk(q, false)
		if !ok {
			t.Errorf("%q was not recognised as conversation — it would return NoAnswer", q)
			continue
		}
		if strings.TrimSpace(got) == NoAnswer {
			t.Errorf("%q answered with the no-answer sentence", q)
		}
		if got == "" {
			t.Errorf("%q recognised but answered with nothing", q)
		}
	}
}

// The other half, and the more important one: a real question must never be swallowed
// here. "quy trình chào giá" contains "chào"; a substring match would answer a pricing
// question with a greeting, which is a far worse bug than the one this file fixes.
func TestARealQuestionIsNeverAnsweredAsConversation(t *testing.T) {
	for _, q := range []string{
		"quy trình chào giá là gì",
		"chào giá cho khách hàng mới",
		"hello world service trả về gì",
		"ai duyệt booking",            // starts with "ai", which the identity pattern uses
		"ok thì bước tiếp theo là gì", // starts with "ok"
		"help_center.md nói gì về hoàn tiền",
		"what can you do with a cancelled booking",
		"TOP_K mặc định là bao nhiêu",
	} {
		if got, ok := smallTalk(q, false); ok {
			t.Errorf("%q was answered as conversation (%.40q) instead of being retrieved", q, got)
		}
	}
}

// The model is already on the status line under every answer, so refusing to name it is
// incoherent — and it cost a completion: 1414ms measured on the deployed instance.
func TestTheModelQuestionIsAnsweredNotRefused(t *testing.T) {
	for _, q := range []string{"model gì", "model nào", "dùng model gì", "bạn dùng model nào", "what model", "which model", "what model are you"} {
		got, ok := smallTalk(q, false)
		if !ok {
			t.Errorf("%q was not recognised — it returns NoAnswer while the status line shows the answer", q)
			continue
		}
		if !strings.Contains(got, "CHAT_MODEL") {
			t.Errorf("%q did not point at the setting that changes it: %.60q", q, got)
		}
	}
	// But a question about a model *in the documents* is a real question.
	for _, q := range []string{"model dữ liệu booking gồm gì", "what model does the pricing service use"} {
		if _, ok := smallTalk(q, false); ok {
			t.Errorf("%q was swallowed as conversation instead of being retrieved", q)
		}
	}
}

// Answering in the language it was asked in, because the app is bilingual and a Vietnamese
// greeting answered in English reads as the wrong product.
func TestConversationAnswersInTheLanguageAsked(t *testing.T) {
	vi, ok := smallTalk("xin chào", false)
	if !ok {
		t.Fatal("xin chào was not recognised")
	}
	if !strings.Contains(vi, "dẫn nguồn") {
		t.Errorf("Vietnamese greeting answered in the wrong language: %.60q", vi)
	}
	en, ok := smallTalk("hello", false)
	if !ok {
		t.Fatal("hello was not recognised")
	}
	if !strings.Contains(en, "cited") {
		t.Errorf("English greeting answered in the wrong language: %.60q", en)
	}
}

// Empty and whitespace fall through to the normal path — there is nothing to answer and
// nothing to ask about, and the caller already rejects an empty question upstream.
//
// Punctuation with nothing in front of it is a different case and is handled, not dropped:
// "?" and "..." are the vaguest questions there are, and they used to reach retrieval and
// come back as the no-answer sentence. They are asked back now.
func TestBlankFallsThroughButPunctuationIsAskedBack(t *testing.T) {
	for _, q := range []string{"", "   ", "\n", "\t "} {
		if _, ok := smallTalk(q, false); ok {
			t.Errorf("%q was treated as conversation", q)
		}
	}
	for _, q := range []string{"?", "???", "...", "!?"} {
		got, ok := smallTalk(q, false)
		if !ok {
			t.Errorf("%q fell through to retrieval and would return the no-answer sentence", q)
			continue
		}
		if strings.TrimSpace(got) == NoAnswer {
			t.Errorf("%q answered with the no-answer sentence", q)
		}
	}
}

// An unclear question must be asked back, never answered with the no-answer sentence: that
// sentence blames the documents for a question nobody finished asking.
func TestAVagueQuestionIsAskedBackNotRefused(t *testing.T) {
	for _, q := range []string{
		"làm sao", "làm thế nào", "thế nào", "cái đó", "cái gì", "gì", "sao",
		"?", "???", "how", "what", "why", "huh", "help me", "tôi muốn biết",
	} {
		got, ok := smallTalk(q, false)
		if !ok {
			t.Errorf("%q was not recognised as vague — it returns NoAnswer instead of asking back", q)
			continue
		}
		if strings.TrimSpace(got) == NoAnswer {
			t.Errorf("%q answered with the no-answer sentence", q)
		}
		// It has to *ask*, not just decline, and it has to show what a good question looks
		// like — a bare "please clarify" spends a turn and teaches nothing.
		if !strings.Contains(got, "?") {
			t.Errorf("%q did not ask anything back: %.60q", q, got)
		}
	}

	// The same words with a conversation behind them are not vague at all: the turn above
	// carries the content word this one is missing, so asking back answers a question
	// nobody asked. They fall through to the rewrite instead, which resolves them against
	// that turn — that is the whole of what conversation memory changes here.
	for _, q := range []string{"cái đó", "sao", "why", "?", "how"} {
		if _, ok := smallTalk(q, true); ok {
			t.Errorf("%q was asked back inside a thread; the rewrite never got to resolve it", q)
		}
	}
	// And nothing else moved with it. A greeting on the tenth message is still a greeting,
	// answered for free — routing these to retrieval would buy a completion to say hello.
	for _, q := range []string{"cảm ơn", "hello", "model gì", "bạn là ai"} {
		if _, ok := smallTalk(q, true); !ok {
			t.Errorf("%q stopped being conversational inside a thread; it costs a completion now", q)
		}
	}
}

// The half that decides whether this is an improvement or a regression. A single word is
// very often the BEST query here — half of retrieval is keyword matching — so a
// length-based or word-count-based guess at vagueness would swallow exactly the lookups
// this engine is strongest at.
func TestAShortRealQueryIsStillRetrieved(t *testing.T) {
	for _, q := range []string{
		"hoàn tiền",           // one content word, a real business term
		"TOP_K",               // a config key
		"BA_PASS",             // another
		"RRF",                 // an acronym from the documents
		"E_BOOKING_LOCKED",    // an error code
		"booking",             // a domain noun
		"làm sao huỷ booking", // starts with a filler, then says what it wants
		"thế nào là chốt giá", // same shape in the other order
		"cái đó trong booking-list_v2.md",
		"what is RRF",
		"how do I cancel a paid booking",
	} {
		if got, ok := smallTalk(q, false); ok {
			t.Errorf("%q was intercepted (%.40q) instead of being retrieved — a real lookup was swallowed", q, got)
		}
	}
}

// asked normalises exactly as Answer() does, so this test and the product classify the same
// string — the two must not be able to drift.
func asked(q string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(q)), " .!?…,;:")
}

// The half that decides whether this is an improvement or a regression, and it is the same
// half as the greeting test above: a real question about the documents must never be read as
// a question about the library. "quy trình hoàn tiền gần đây có đổi gì không" is about a
// refund flow — answering it with a file listing is a worse bug than the one being fixed.
func TestARealQuestionIsNotMistakenForARecencyOne(t *testing.T) {
	for _, q := range []string{
		"tài liệu nào mới cập nhật",
		"Tài liệu nào mới cập nhật?",
		"tai lieu nao moi cap nhat",
		"tài liệu nào vừa thay đổi",
		"có tài liệu nào mới không",
		"which documents were updated recently",
		"what's new in the library",
	} {
		if ask, _ := corpusAskOf(asked(q)); ask != recentDocs {
			t.Errorf("%q should list the recently updated documents", q)
		}
	}
	for _, q := range []string{
		"các QA đã chốt gần đây",
		"các QA đã chốt confirm gần đây",
		"câu hỏi nào đã được duyệt gần đây",
		"cac qa da chot gan day",
		"recently confirmed answers",
		"which answers were confirmed recently",
	} {
		if ask, _ := corpusAskOf(asked(q)); ask != recentQA {
			t.Errorf("%q should list the recently confirmed answers", q)
		}
	}
	for _, q := range []string{
		"có bao nhiêu decision",
		"how many decision",
		"liệt kê tất cả decision",
		"có bao nhiêu tài liệu",
	} {
		if ask, _ := corpusAskOf(asked(q)); ask != countDocs {
			t.Errorf("%q should be counted from the library, not retrieved", q)
		}
	}
	// Two guards, and this half tests the first: a category is at most three words, so a
	// question that counts something *inside* the documents never reaches the store at all.
	// The second guard is the data, and it is in the pipeline test — a term matching no
	// document is not a category whatever its shape.
	for _, q := range []string{
		"có bao nhiêu ngày để hoàn tiền",
		"bao nhiêu ngày sau khi huỷ thì được hoàn tiền",
		"how many days before a deposit is forfeited",
	} {
		if ask, _ := corpusAskOf(asked(q)); ask == countDocs {
			t.Errorf("%q counts something inside the documents and must be retrieved, not listed", q)
		}
	}
	for _, q := range []string{
		"quy trình hoàn tiền gần đây có đổi gì không",
		"has the refund flow changed recently",
		"tài liệu nào nói về hoàn tiền",
		"which document covers the deposit rule",
		"cái nào mới nhất",         // no content word and no "tài liệu": vague, not a library question
		"cập nhật booking thế nào", // "cập nhật" about a process, not about the library
		"booking",
		"what changed in the refund rules",
	} {
		if got, _ := corpusAskOf(asked(q)); got != notCorpusAsk {
			t.Errorf("%q was read as a library question (%d) instead of being retrieved", q, got)
		}
	}
}
