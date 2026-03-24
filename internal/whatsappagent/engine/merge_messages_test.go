package engine

import (
	"testing"
	"time"
)

func TestMergeTrailingUserMessages(t *testing.T) {
	now := time.Now()
	later := now.Add(2 * time.Second)

	tests := []struct {
		name     string
		input    []ConversationMessage
		wantLen  int
		wantLast string
	}{
		{
			name:    "nil input",
			input:   nil,
			wantLen: 0,
		},
		{
			name:     "single user message unchanged",
			input:    []ConversationMessage{{Role: "user", Content: "hallo"}},
			wantLen:  1,
			wantLast: "hallo",
		},
		{
			name: "no trailing streak (ends with assistant)",
			input: []ConversationMessage{
				{Role: "user", Content: "a"},
				{Role: "assistant", Content: "b"},
			},
			wantLen:  2,
			wantLast: "b",
		},
		{
			name: "two consecutive user messages merged",
			input: []ConversationMessage{
				{Role: "assistant", Content: "hoi"},
				{Role: "user", Content: "maak een lead aan", SentAt: &now},
				{Role: "user", Content: "service type is timmerwerken", SentAt: &later},
			},
			wantLen:  2, // assistant + merged user
			wantLast: "maak een lead aan\n\nservice type is timmerwerken",
		},
		{
			name: "three consecutive user messages merged",
			input: []ConversationMessage{
				{Role: "user", Content: "eerste bericht", SentAt: &now},
				{Role: "user", Content: "tweede bericht"},
				{Role: "user", Content: "derde bericht", SentAt: &later},
			},
			wantLen:  1,
			wantLast: "eerste bericht\n\ntweede bericht\n\nderde bericht",
		},
		{
			name: "only last streak merged, earlier consecutive users untouched",
			input: []ConversationMessage{
				{Role: "user", Content: "old1"},
				{Role: "user", Content: "old2"},
				{Role: "assistant", Content: "reply"},
				{Role: "user", Content: "new1"},
				{Role: "user", Content: "new2"},
			},
			wantLen:  4, // user, user, assistant, merged
			wantLast: "new1\n\nnew2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeTrailingUserMessages(tt.input)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d; messages: %+v", len(got), tt.wantLen, got)
			}
			if tt.wantLast != "" && got[len(got)-1].Content != tt.wantLast {
				t.Fatalf("last content = %q, want %q", got[len(got)-1].Content, tt.wantLast)
			}
		})
	}
}

func TestMergeTrailingUserMessagesPreservesSentAt(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 10, 0, 3, 0, time.UTC)

	msgs := []ConversationMessage{
		{Role: "user", Content: "a", SentAt: &t1},
		{Role: "user", Content: "b", SentAt: &t2},
	}
	got := MergeTrailingUserMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("expected 1 merged message, got %d", len(got))
	}
	if got[0].SentAt == nil || !got[0].SentAt.Equal(t2) {
		t.Fatalf("SentAt = %v, want %v", got[0].SentAt, t2)
	}
}
