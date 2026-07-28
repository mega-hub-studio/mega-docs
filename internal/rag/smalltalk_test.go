package rag

import "strings"

import "testing"

// The failure this exists to prevent: "xin chào" answered with the no-answer sentence,
// after a full embed + retrieve + completion.
func TestAGreetingIsNotAMissingDocument(t *testing.T) {
	for _, q := range []string{"xin chào", "Xin chào!", "chào bạn", "hello", "Hi", "hey", "thanks", "cảm ơn"} {
		got, ok := smallTalk(q)
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
		if got, ok := smallTalk(q); ok {
			t.Errorf("%q was answered as conversation (%.40q) instead of being retrieved", q, got)
		}
	}
}

// Answering in the language it was asked in, because the app is bilingual and a Vietnamese
// greeting answered in English reads as the wrong product.
func TestConversationAnswersInTheLanguageAsked(t *testing.T) {
	vi, ok := smallTalk("xin chào")
	if !ok {
		t.Fatal("xin chào was not recognised")
	}
	if !strings.Contains(vi, "dẫn nguồn") {
		t.Errorf("Vietnamese greeting answered in the wrong language: %.60q", vi)
	}
	en, ok := smallTalk("hello")
	if !ok {
		t.Fatal("hello was not recognised")
	}
	if !strings.Contains(en, "cited") {
		t.Errorf("English greeting answered in the wrong language: %.60q", en)
	}
}

// Empty and whitespace must fall through to the normal path rather than being greeted.
func TestBlankIsNotConversation(t *testing.T) {
	for _, q := range []string{"", "   ", "\n", "..."} {
		if _, ok := smallTalk(q); ok {
			t.Errorf("%q was treated as conversation", q)
		}
	}
}
