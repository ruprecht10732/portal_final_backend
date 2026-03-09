package agent

import (
	"strings"
	"testing"
)

func TestValidateAppointmentAvailabilityRejectsScheduledWithoutAppointment(t *testing.T) {
	err := validateAppointmentAvailability("Appointment_Scheduled", false)
	if err == nil {
		t.Fatal("expected error when appointment is not available")
	}
	if !strings.Contains(err.Error(), "Appointment_Scheduled") {
		t.Fatalf("expected error to mention Appointment_Scheduled, got %q", err.Error())
	}
}

func TestValidateAppointmentAvailabilityAllowsScheduledWithAppointment(t *testing.T) {
	if err := validateAppointmentAvailability("Appointment_Scheduled", true); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEnforceAppointmentSchedulingConsistencyClearsFalseScheduledState(t *testing.T) {
	callOutcome := "Appointment_Scheduled"
	status := "Appointment_Scheduled"
	result := enforceAppointmentSchedulingConsistency(CallLogResult{
		CallOutcome:   &callOutcome,
		StatusUpdated: &status,
	}, false)

	if result.CallOutcome != nil {
		t.Fatalf("expected scheduled call outcome to be cleared, got %q", *result.CallOutcome)
	}
	if result.StatusUpdated != nil {
		t.Fatalf("expected scheduled status to be cleared, got %q", *result.StatusUpdated)
	}
	if result.Warning == "" {
		t.Fatal("expected warning to be set when scheduled state is normalized")
	}
	if !strings.Contains(result.Warning, "manual follow-up") {
		t.Fatalf("expected warning to mention manual follow-up, got %q", result.Warning)
	}
}

func TestEnforceAppointmentSchedulingConsistencyPreservesScheduledStateWhenAppointmentExists(t *testing.T) {
	callOutcome := "Appointment_Scheduled"
	status := "Appointment_Scheduled"
	result := enforceAppointmentSchedulingConsistency(CallLogResult{
		CallOutcome:   &callOutcome,
		StatusUpdated: &status,
	}, true)

	if result.CallOutcome == nil || *result.CallOutcome != "Appointment_Scheduled" {
		t.Fatalf("expected scheduled call outcome to be preserved, got %#v", result.CallOutcome)
	}
	if result.StatusUpdated == nil || *result.StatusUpdated != "Appointment_Scheduled" {
		t.Fatalf("expected scheduled status to be preserved, got %#v", result.StatusUpdated)
	}
	if result.Warning != "" {
		t.Fatalf("expected no warning when appointment exists, got %q", result.Warning)
	}
}
