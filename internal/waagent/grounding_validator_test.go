package waagent

import "testing"

const (
	testQuoteSpecificReply    = "*Offerte:* OFF-1\n*Bedrag:* € 125,00"
	testQuoteNumberOnlyReply  = "*Offerte:* OFF-2026-0019"
	testUnexpectedFallbackMsg = "unexpected fallback reply %q"
	testNoGroundingIssueMsg   = "expected no grounding issue, got %q"
	testUnexpectedReplyMsg    = "unexpected validated reply %q"
	testAppointmentReply      = "*Datum:* 18 maart 2026\n*Tijd:* 16:00\n*Locatie:* Alkmaar"
	testLeadSpecificReply     = "*Adres:* Kerkstraat 12\n*Status:* Intake\n*Telefoon:* +31612345678"
	testLeadAddressReply      = "*Adres:* Kerkstraat 12, 1811 AB Alkmaar"
	testLeadStatusReply       = "*Status:* Intake\n*Telefoon:* +31612345678"
	testLeadFactFallbackReply = "Ik kan dat klantdetail zo niet bevestigen. Noem de klantnaam of het dossier, dan controleer ik het meteen."
)

func newGroundingEvidenceWithResponse(toolName string, payload string) *replyGroundingEvidence {
	evidence := newReplyGroundingEvidence()
	evidence.toolResponseNames[toolName] = 1
	evidence.toolResponses = append(evidence.toolResponses, toolResponseObservation{Name: toolName, Payload: payload})
	return evidence
}

func TestValidateGroundedReplyRejectsQuoteSpecificsWithoutQuoteTool(t *testing.T) {
	t.Parallel()

	reply, issue := validateGroundedReply(testQuoteSpecificReply, newReplyGroundingEvidence())
	if issue.Code != "quote_details_without_quote_tool" {
		t.Fatalf("expected quote grounding issue, got %q", issue.Code)
	}
	if reply != "Noem de klantnaam of het offertenummer, dan pak ik de juiste offerte erbij." {
		t.Fatalf(testUnexpectedFallbackMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsQuoteCapabilityReplyWithoutQuoteTool(t *testing.T) {
	t.Parallel()

	reply, issue := validateGroundedReply("Ik kan open offertes laten zien en kort toelichten waar een offerte over gaat.", newReplyGroundingEvidence())
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != "Ik kan open offertes laten zien en kort toelichten waar een offerte over gaat." {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsQuoteSpecificsWithQuoteTool(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetQuotes", `{"quotes":[{"total_cents":12500,"quote_number":"OFF-1"}]}`)
	reply, issue := validateGroundedReply(testQuoteSpecificReply, evidence)
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != testQuoteSpecificReply {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyRejectsQuoteAmountMissingFromToolResult(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetQuotes", `{"quotes":[{"total_cents":9900,"quote_number":"OFF-1"}]}`)
	reply, issue := validateGroundedReply(testQuoteSpecificReply, evidence)
	if issue.Code != "quote_fact_not_in_tool_result" {
		t.Fatalf("expected quote fact mismatch, got %q", issue.Code)
	}
	if len(issue.UnsupportedFacts) != 1 || issue.UnsupportedFacts[0] != "€ 125,00" {
		t.Fatalf("unexpected unsupported quote facts %#v", issue.UnsupportedFacts)
	}
	if reply != "Ik kan dat offertedetail zo niet bevestigen. Noem de klantnaam of het offertenummer, dan controleer ik het meteen." {
		t.Fatalf(testUnexpectedFallbackMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsQuoteNumberWhenToolPayloadMatches(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetQuotes", `{"quotes":[{"quote_number":"OFF-2026-0019"}]}`)
	reply, issue := validateGroundedReply(testQuoteNumberOnlyReply, evidence)
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != testQuoteNumberOnlyReply {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyRejectsQuoteNumberMissingFromToolResult(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetQuotes", `{"quotes":[{"quote_number":"OFF-2026-0018"}]}`)
	reply, issue := validateGroundedReply(testQuoteNumberOnlyReply, evidence)
	if issue.Code != "quote_fact_not_in_tool_result" {
		t.Fatalf("expected quote-number fact mismatch, got %q", issue.Code)
	}
	if len(issue.UnsupportedFacts) != 1 || issue.UnsupportedFacts[0] != "OFF-2026-0019" {
		t.Fatalf("unexpected unsupported quote facts %#v", issue.UnsupportedFacts)
	}
	if reply != "Ik kan dat offertedetail zo niet bevestigen. Noem de klantnaam of het offertenummer, dan controleer ik het meteen." {
		t.Fatalf(testUnexpectedFallbackMsg, reply)
	}
}

func TestValidateGroundedReplyRejectsAppointmentSpecificsWithoutAppointmentTool(t *testing.T) {
	t.Parallel()

	reply, issue := validateGroundedReply(testAppointmentReply, newReplyGroundingEvidence())
	if issue.Code != "appointment_details_without_appointment_tool" {
		t.Fatalf("expected appointment grounding issue, got %q", issue.Code)
	}
	if reply != "Noem de datum, periode of klant, dan pak ik de juiste afspraak erbij." {
		t.Fatalf(testUnexpectedFallbackMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsAppointmentCapabilityReplyWithoutAppointmentTool(t *testing.T) {
	t.Parallel()

	reply, issue := validateGroundedReply("Ik kan afspraken voor een periode ophalen en laten zien wat er volgende week gepland staat.", newReplyGroundingEvidence())
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != "Ik kan afspraken voor een periode ophalen en laten zien wat er volgende week gepland staat." {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsAppointmentSpecificsWhenDateAndTimeMatchToolResult(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetAppointments", `{"appointments":[{"start_time":"2026-03-18T16:00:00Z","location":"Alkmaar"}]}`)
	reply, issue := validateGroundedReply(testAppointmentReply, evidence)
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != testAppointmentReply {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyRejectsAppointmentTimeMissingFromToolResult(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetAppointments", `{"appointments":[{"start_time":"2026-03-18T15:00:00Z"}]}`)
	reply, issue := validateGroundedReply(testAppointmentReply, evidence)
	if issue.Code != "appointment_fact_not_in_tool_result" {
		t.Fatalf("expected appointment fact mismatch, got %q", issue.Code)
	}
	if len(issue.UnsupportedFacts) == 0 {
		t.Fatal("expected unsupported appointment facts to be reported")
	}
	if reply != "Ik kan dat afspraakdetail zo niet bevestigen. Noem de datum, periode of klant, dan controleer ik het meteen." {
		t.Fatalf(testUnexpectedFallbackMsg, reply)
	}
}

func TestValidateGroundedReplyRejectsLeadSpecificsWithoutLeadTool(t *testing.T) {
	t.Parallel()

	reply, issue := validateGroundedReply(testLeadSpecificReply, newReplyGroundingEvidence())
	if issue.Code != "lead_details_without_lead_tool" {
		t.Fatalf("expected lead grounding issue, got %q", issue.Code)
	}
	if reply != "Noem de klantnaam of het dossier waar het over gaat, dan controleer ik de gegevens." {
		t.Fatalf(testUnexpectedFallbackMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsLeadStatusWhenToolPayloadMatches(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetLeadDetails", `{"lead":{"status":"Intake","phone":"+31612345678"}}`)
	reply, issue := validateGroundedReply(testLeadStatusReply, evidence)
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != testLeadStatusReply {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsLeadPhoneFromSearchLeads(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("SearchLeads", `{"leads":[{"customer_name":"A. Boogaard","phone":"+31640567063","city":"Alkmaar"}],"count":1}`)
	reply, issue := validateGroundedReply("Het telefoonnummer van A. Boogaard is +31640567063.", evidence)
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != "Het telefoonnummer van A. Boogaard is +31640567063." {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsLeadAddressFromNavigationLink(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetNavigationLink", `{"success":true,"message":"Navigatielink gevonden","link":{"lead_id":"lead-1","destination_address":"Kerkstraat 12, 1811 AB Alkmaar","url":"https://maps.example/route"}}`)
	reply, issue := validateGroundedReply(testLeadAddressReply, evidence)
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != testLeadAddressReply {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsAppointmentDetailsFromPartnerJobDetails(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetPartnerJobDetails", `{"success":true,"message":"Opdracht gevonden","job":{"appointment_start":"2026-03-18T16:00:00Z","appointment_end":"2026-03-18T17:00:00Z","destination_address":"Kerkstraat 12, 1811 AB Alkmaar"}}`)
	reply, issue := validateGroundedReply(testAppointmentReply, evidence)
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != testAppointmentReply {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyRejectsLeadAddressFromSearchLeadsOnly(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("SearchLeads", `{"leads":[{"customer_name":"A. Boogaard","phone":"+31640567063","city":"Alkmaar"}],"count":1}`)
	reply, issue := validateGroundedReply(testLeadAddressReply, evidence)
	if issue.Code != "lead_fact_not_in_tool_result" {
		t.Fatalf("expected lead fact mismatch, got %q", issue.Code)
	}
	if reply != testLeadFactFallbackReply {
		t.Fatalf(testUnexpectedFallbackMsg, reply)
	}
}

func TestValidateGroundedReplyRejectsLeadStatusMissingFromToolResult(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetLeadDetails", `{"lead":{"status":"Gepland","phone":"+31612345678"}}`)
	reply, issue := validateGroundedReply(testLeadStatusReply, evidence)
	if issue.Code != "lead_fact_not_in_tool_result" {
		t.Fatalf("expected lead fact mismatch, got %q", issue.Code)
	}
	if len(issue.UnsupportedFacts) != 1 || issue.UnsupportedFacts[0] != "Intake" {
		t.Fatalf("unexpected unsupported lead facts %#v", issue.UnsupportedFacts)
	}
	if reply != testLeadFactFallbackReply {
		t.Fatalf(testUnexpectedFallbackMsg, reply)
	}
}

func TestValidateGroundedReplyAllowsLeadAddressWhenToolPayloadMatches(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetLeadDetails", `{"lead":{"street":"Kerkstraat","house_number":"12","zip_code":"1811 AB","city":"Alkmaar"}}`)
	reply, issue := validateGroundedReply(testLeadAddressReply, evidence)
	if issue.Code != "" {
		t.Fatalf(testNoGroundingIssueMsg, issue.Code)
	}
	if reply != testLeadAddressReply {
		t.Fatalf(testUnexpectedReplyMsg, reply)
	}
}

func TestValidateGroundedReplyRejectsLeadAddressMissingFromToolResult(t *testing.T) {
	t.Parallel()

	evidence := newGroundingEvidenceWithResponse("GetLeadDetails", `{"lead":{"street":"Kerkstraat","house_number":"13","zip_code":"1811 AB","city":"Alkmaar"}}`)
	reply, issue := validateGroundedReply(testLeadAddressReply, evidence)
	if issue.Code != "lead_fact_not_in_tool_result" {
		t.Fatalf("expected address fact mismatch, got %q", issue.Code)
	}
	if len(issue.UnsupportedFacts) != 1 || issue.UnsupportedFacts[0] != "Kerkstraat 12, 1811 AB Alkmaar" {
		t.Fatalf("unexpected unsupported lead facts %#v", issue.UnsupportedFacts)
	}
	if reply != testLeadFactFallbackReply {
		t.Fatalf(testUnexpectedFallbackMsg, reply)
	}
}
