package waagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	toolsTestDateLayout = "2006-01-02"
	toolsTestDateFrom   = "2026-03-16"
	toolsTestDateTo     = "2026-03-18"
)

type toolsTestAppointmentsReader struct {
	from         *time.Time
	to           *time.Time
	err          error
	appointments []AppointmentSummary
}

func (r *toolsTestAppointmentsReader) ListAppointmentsByOrganization(_ context.Context, _ uuid.UUID, from, to *time.Time) ([]AppointmentSummary, error) {
	r.from = from
	r.to = to
	if r.err != nil {
		return nil, r.err
	}
	return r.appointments, nil
}

func TestParseAppointmentDateInputAcceptsDutchAndEuropeanFormats(t *testing.T) {
	t.Parallel()

	for _, input := range []string{toolsTestDateFrom, "16-03-2026", "16/03/2026", "16 maart 2026"} {
		parsed, err := parseAppointmentDateInput(input)
		if err != nil {
			t.Fatalf("expected %q to parse, got %v", input, err)
		}
		if parsed.Format(toolsTestDateLayout) != toolsTestDateFrom {
			t.Fatalf("expected normalized date %s for %q, got %s", toolsTestDateFrom, input, parsed.Format(toolsTestDateLayout))
		}
	}
}

func TestHandleGetAppointmentsAcceptsEuropeanDateFormat(t *testing.T) {
	t.Parallel()

	reader := &toolsTestAppointmentsReader{}
	handler := &ToolHandler{appointmentsReader: reader}

	_, err := handler.HandleGetAppointments(newActionToolsTestContext(testSenderPhone), uuid.New(), GetAppointmentsInput{DateFrom: "16-03-2026", DateTo: "18-03-2026"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if reader.from == nil || reader.to == nil {
		t.Fatal("expected parsed date range to reach appointments reader")
	}
	if reader.from.Format(toolsTestDateLayout) != toolsTestDateFrom || reader.to.Format(toolsTestDateLayout) != toolsTestDateTo {
		t.Fatalf("unexpected parsed range from=%v to=%v", reader.from, reader.to)
	}
}

func TestHandleGetAppointmentsReturnsSafeTechnicalFailure(t *testing.T) {
	t.Parallel()

	handler := &ToolHandler{appointmentsReader: &toolsTestAppointmentsReader{err: errors.New("db timeout")}}

	_, err := handler.HandleGetAppointments(newActionToolsTestContext(testSenderPhone), uuid.New(), GetAppointmentsInput{})
	if err == nil {
		t.Fatal("expected error when appointment reader fails")
	}
	if err.Error() != "ik kan de afspraken nu niet ophalen. probeer het later opnieuw" {
		t.Fatalf("unexpected safe error %q", err.Error())
	}
}
