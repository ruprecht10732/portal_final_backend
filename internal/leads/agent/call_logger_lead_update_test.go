package agent

import (
	"testing"
)

func strPtr(value string) *string {
	return &value
}

func TestBuildLeadUpdateRequestRequiresAtLeastOneField(t *testing.T) {
	_, _, err := buildLeadUpdateRequest(UpdateLeadDetailsInput{})
	if err == nil {
		t.Fatal("expected error when no lead fields are provided")
	}
}

func TestBuildLeadUpdateRequestNormalizesFields(t *testing.T) {
	req, updatedFields, err := buildLeadUpdateRequest(UpdateLeadDetailsInput{
		FirstName:       strPtr("  Robin "),
		Phone:           strPtr("06 12 34 56 78"),
		ConsumerRole:    strPtr("owner"),
		Street:          strPtr("  Dorpsstraat  "),
		WhatsAppOptedIn: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if req.FirstName == nil || *req.FirstName != "Robin" {
		t.Fatalf("expected trimmed first name, got %#v", req.FirstName)
	}
	if req.Phone == nil || *req.Phone != "+31612345678" {
		t.Fatalf("expected normalized phone, got %#v", req.Phone)
	}
	if req.ConsumerRole == nil || *req.ConsumerRole != "Owner" {
		t.Fatalf("expected normalized consumer role, got %#v", req.ConsumerRole)
	}
	if req.Street == nil || *req.Street != "Dorpsstraat" {
		t.Fatalf("expected trimmed street, got %#v", req.Street)
	}
	if req.WhatsAppOptedIn == nil || !*req.WhatsAppOptedIn {
		t.Fatalf("expected WhatsApp preference to be preserved, got %#v", req.WhatsAppOptedIn)
	}
	if len(updatedFields) != 5 {
		t.Fatalf("expected 5 updated fields, got %d (%v)", len(updatedFields), updatedFields)
	}
}

func TestBuildLeadUpdateRequestRejectsInvalidConsumerRole(t *testing.T) {
	_, _, err := buildLeadUpdateRequest(UpdateLeadDetailsInput{
		ConsumerRole: strPtr("invalid"),
	})
	if err == nil {
		t.Fatal("expected invalid consumer role to fail")
	}
}

func boolPtr(value bool) *bool {
	return &value
}
