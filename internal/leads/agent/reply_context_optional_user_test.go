package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/platform/apperr"
)

const (
	unauthorizedLookupMessage = "invalid credentials"
	expectedNilErrorFormat    = "expected nil error, got %v"
)

type stubReplyUserReader struct {
	profile *ports.ReplyUserProfile
	err     error
}

func (s stubReplyUserReader) GetUserProfile(context.Context, uuid.UUID) (*ports.ReplyUserProfile, error) {
	return s.profile, s.err
}

func TestResolveAppointmentAssigneeNameIgnoresUnauthorizedLookup(t *testing.T) {
	name, err := resolveAppointmentAssigneeName(context.Background(), stubReplyUserReader{err: apperr.Unauthorized(unauthorizedLookupMessage)}, uuid.New())
	if err != nil {
		t.Fatalf(expectedNilErrorFormat, err)
	}
	if name != "" {
		t.Fatalf("expected empty name, got %q", name)
	}
}

func TestWhatsAppReplyLoadRequesterIgnoresUnauthorizedLookup(t *testing.T) {
	agent := &ReplyAgent{userReader: stubReplyUserReader{err: apperr.Unauthorized(unauthorizedLookupMessage)}}

	requester, err := agent.loadRequester(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf(expectedNilErrorFormat, err)
	}
	if requester != nil {
		t.Fatalf("expected nil requester, got %#v", requester)
	}
}

func TestEmailReplyLoadRequesterIgnoresUnauthorizedLookup(t *testing.T) {
	agent := &ReplyAgent{userReader: stubReplyUserReader{err: apperr.Unauthorized(unauthorizedLookupMessage)}}

	requester, err := agent.loadRequester(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf(expectedNilErrorFormat, err)
	}
	if requester != nil {
		t.Fatalf("expected nil requester, got %#v", requester)
	}
}

func TestWhatsAppReplyLoadRequesterPreservesOtherErrors(t *testing.T) {
	agent := &ReplyAgent{userReader: stubReplyUserReader{err: errors.New("boom")}}

	_, err := agent.loadRequester(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
