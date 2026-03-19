package service

import (
	"testing"
	"time"

	"portal_final_backend/internal/quotes/repository"

	"github.com/google/uuid"
)

func TestBuildQuoteAnnotationThreadFiltersByItem(t *testing.T) {
	itemID := uuid.New()
	otherItemID := uuid.New()
	now := time.Now().UTC()

	thread := buildQuoteAnnotationThread(itemID, []repository.QuoteAnnotation{
		{QuoteItemID: itemID, AuthorType: "customer", Text: " Vraag ", CreatedAt: now},
		{QuoteItemID: otherItemID, AuthorType: "customer", Text: "Andere", CreatedAt: now},
		{QuoteItemID: itemID, AuthorType: "agent", Text: " Antwoord ", CreatedAt: now.Add(time.Minute)},
	})

	if len(thread) != 2 {
		t.Fatalf("expected 2 thread messages, got %d", len(thread))
	}
	if thread[0].Text != "Vraag" {
		t.Fatalf("expected first message to be trimmed, got %q", thread[0].Text)
	}
	if thread[1].AuthorType != "agent" {
		t.Fatalf("expected second message to keep author type, got %q", thread[1].AuthorType)
	}
}

func TestThreadContainsCustomerQuestion(t *testing.T) {
	if threadContainsCustomerQuestion([]QuoteAnnotationReplyDraftMessage{{AuthorType: "agent", Text: "Antwoord"}}) {
		t.Fatal("expected false when no customer message exists")
	}
	if !threadContainsCustomerQuestion([]QuoteAnnotationReplyDraftMessage{{AuthorType: "customer", Text: "  Vraag?  "}}) {
		t.Fatal("expected true when a customer question exists")
	}
}
