package agent

import (
	"testing"

	"portal_final_backend/internal/leads/transport"
)

func assertStringPointerValue(t *testing.T, actual *string, expected string, label string) {
	t.Helper()
	if actual == nil || *actual != expected {
		t.Fatalf("expected %s %q, got %#v", label, expected, actual)
	}
}

func assertFloatPointerValue(t *testing.T, actual *float64, expected float64, label string) {
	t.Helper()
	if actual == nil || *actual != expected {
		t.Fatalf("expected %s %v, got %#v", label, expected, actual)
	}
}

func assertAssigneeID(t *testing.T, actual transport.OptionalUUID, expected string) {
	t.Helper()
	if !actual.Set || actual.Value == nil || actual.Value.String() != expected {
		t.Fatalf("expected parsed assignee ID %q, got %#v", expected, actual)
	}
}

func assertLeadUpdateRequestNormalized(t *testing.T, req transport.UpdateLeadRequest, assigneeID string, latitude float64, longitude float64) {
	t.Helper()
	assertStringPointerValue(t, req.FirstName, "Robin", "firstName")
	assertStringPointerValue(t, req.Phone, "+31612345678", "phone")
	assertAssigneeID(t, req.AssigneeID, assigneeID)
	if req.ConsumerRole == nil || *req.ConsumerRole != "Owner" {
		t.Fatalf("expected normalized consumer role, got %#v", req.ConsumerRole)
	}
	assertStringPointerValue(t, req.Street, "Dorpsstraat", "street")
	assertFloatPointerValue(t, req.Latitude, latitude, "latitude")
	assertFloatPointerValue(t, req.Longitude, longitude, "longitude")
	if req.WhatsAppOptedIn == nil || !*req.WhatsAppOptedIn {
		t.Fatalf("expected WhatsApp preference to be preserved, got %#v", req.WhatsAppOptedIn)
	}
}

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
	const assigneeID = "4f7287df-20b2-4c1d-beb5-e2c0c23c7d29"
	latitude := 52.0907
	longitude := 5.1214
	req, updatedFields, err := buildLeadUpdateRequest(UpdateLeadDetailsInput{
		FirstName:       strPtr("  Robin "),
		Phone:           strPtr("06 12 34 56 78"),
		AssigneeID:      strPtr(assigneeID),
		ConsumerRole:    strPtr("owner"),
		Street:          strPtr("  Dorpsstraat  "),
		Latitude:        &latitude,
		Longitude:       &longitude,
		WhatsAppOptedIn: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	assertLeadUpdateRequestNormalized(t, req, assigneeID, latitude, longitude)
	if len(updatedFields) != 8 {
		t.Fatalf("expected 8 updated fields, got %d (%v)", len(updatedFields), updatedFields)
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

func TestBuildLeadUpdateRequestRejectsInvalidAssigneeID(t *testing.T) {
	_, _, err := buildLeadUpdateRequest(UpdateLeadDetailsInput{
		AssigneeID: strPtr("not-a-uuid"),
	})
	if err == nil {
		t.Fatal("expected invalid assignee ID to fail")
	}
}

func TestBuildLeadUpdateRequestRejectsInvalidLatitude(t *testing.T) {
	latitude := 100.0
	_, _, err := buildLeadUpdateRequest(UpdateLeadDetailsInput{
		Latitude: &latitude,
	})
	if err == nil {
		t.Fatal("expected invalid latitude to fail")
	}
}

func boolPtr(value bool) *bool {
	return &value
}
