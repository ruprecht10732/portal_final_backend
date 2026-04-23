package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appointmentstransport "portal_final_backend/internal/appointments/transport"
	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/management"
	leadsrepo "portal_final_backend/internal/leads/repository"
	leadstransport "portal_final_backend/internal/leads/transport"
	quotestransport "portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestGetDetailContextReturnsAggregatePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testState := newDetailContextTestState()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leads/"+testState.leadID.String()+"/detail-context", nil)
	testState.router.ServeHTTP(recorder, req)

	assertDetailContextResponse(t, recorder, testState)
}

type detailContextTestState struct {
	router         *gin.Engine
	leadID         uuid.UUID
	quoteID        uuid.UUID
	appointmentID  uuid.UUID
	conversationID uuid.UUID
}

func newDetailContextTestState() detailContextTestState {
	tenantID := uuid.New()
	userID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	conversationID := uuid.New()
	accountID := uuid.New()
	quoteID := uuid.New()
	appointmentID := uuid.New()
	workflowID := uuid.New()
	now := time.Date(2026, time.March, 15, 10, 0, 0, 0, time.UTC)

	lead := leadsrepo.Lead{
		ID:                 leadID,
		OrganizationID:     tenantID,
		ConsumerFirstName:  "Robin",
		ConsumerLastName:   "Builder",
		ConsumerPhone:      "+31600000000",
		ConsumerRole:       "Owner",
		AddressStreet:      "Canal Street",
		AddressHouseNumber: "12",
		AddressZipCode:     "1234AB",
		AddressCity:        "Amsterdam",
		Source:             stringPtr("Google Ads"),
		WhatsAppOptedIn:    true,
		CreatedAt:          now.Add(-2 * time.Hour),
		UpdatedAt:          now.Add(-time.Hour),
	}
	services := []leadsrepo.LeadService{{
		ID:             serviceID,
		LeadID:         leadID,
		OrganizationID: tenantID,
		ServiceType:    "Roof Repair",
		Status:         "Pending",
		PipelineStage:  "Triage",
		CreatedAt:      now.Add(-90 * time.Minute),
		UpdatedAt:      now.Add(-70 * time.Minute),
	}}
	repo := &detailContextRepoStub{
		Repository: &leadsrepo.Repository{},
		lead:       lead,
		services:   services,
		notes: []leadsrepo.LeadNote{{
			ID:          uuid.New(),
			LeadID:      leadID,
			AuthorID:    userID,
			AuthorEmail: "agent@example.com",
			Type:        "note",
			Body:        "Customer asked for a morning callback.",
			CreatedAt:   now.Add(-50 * time.Minute),
			UpdatedAt:   now.Add(-50 * time.Minute),
		}},
		whatsAppItems: []leadsrepo.LinkedWhatsAppConversation{{
			ConversationID:        conversationID,
			PhoneNumber:           "+31600000000",
			DisplayName:           "Robin Builder",
			LastMessagePreview:    "Can you confirm the visit?",
			RelationshipUpdatedAt: now.Add(-40 * time.Minute),
		}},
		emailItems: []leadsrepo.LinkedIMAPMessage{{
			AccountID:             accountID,
			MessageUID:            42,
			Subject:               "Question about my quote",
			RelationshipUpdatedAt: now.Add(-35 * time.Minute),
		}},
		analysis: &leadsrepo.AIAnalysis{
			ID:                      uuid.New(),
			LeadID:                  leadID,
			OrganizationID:          tenantID,
			LeadServiceID:           serviceID,
			UrgencyLevel:            "Medium",
			LeadQuality:             "Potential",
			RecommendedAction:       "RequestInfo",
			MissingInformation:      []string{"Roof dimensions"},
			ResolvedInformation:     []string{"Leak location"},
			ExtractedFacts:          map[string]string{"issue": "roof leak"},
			PreferredContactChannel: "WhatsApp",
			SuggestedContactMessage: "Please share the roof dimensions.",
			Summary:                 "Intake is promising but incomplete.",
			CreatedAt:               now.Add(-30 * time.Minute),
		},
		photo: &leadsrepo.PhotoAnalysis{
			ID:              uuid.New(),
			LeadID:          leadID,
			ServiceID:       serviceID,
			Summary:         "Visible flashing damage near the chimney.",
			Observations:    []string{"Cracked flashing"},
			ScopeAssessment: "Local repair",
			ConfidenceLevel: "High",
			PhotoCount:      3,
			CreatedAt:       now.Add(-25 * time.Minute),
		},
	}

	mgmt := management.New(repo, events.NewInMemoryBus(nil), nil)
	mgmt.SetLeadDetailQuotesReader(detailContextQuoteReader{items: []quotestransport.QuoteResponse{{
		ID:            quoteID,
		LeadID:        leadID,
		LeadServiceID: &serviceID,
		QuoteNumber:   "Q-2026-001",
		Status:        quotestransport.QuoteStatusSent,
		TotalCents:    125000,
		CreatedAt:     now.Add(-20 * time.Minute),
		UpdatedAt:     now.Add(-20 * time.Minute),
	}}})
	mgmt.SetLeadDetailAppointmentsReader(detailContextAppointmentReader{items: []appointmentstransport.AppointmentResponse{{
		ID:            appointmentID,
		UserID:        userID,
		LeadID:        &leadID,
		LeadServiceID: &serviceID,
		Type:          appointmentstransport.AppointmentTypeLeadVisit,
		Title:         "Site visit",
		StartTime:     now.Add(24 * time.Hour),
		EndTime:       now.Add(25 * time.Hour),
		Status:        appointmentstransport.AppointmentStatusScheduled,
		CreatedAt:     now.Add(-15 * time.Minute),
		UpdatedAt:     now.Add(-15 * time.Minute),
	}}})
	mgmt.SetLeadDetailWorkflowContextReader(detailContextWorkflowReader{context: &leadstransport.LeadDetailWorkflowContext{
		Override: &leadstransport.LeadDetailWorkflowOverrideContext{
			WorkflowID:   stringPtr(workflowID.String()),
			OverrideMode: "manual",
		},
		Resolved: &leadstransport.LeadDetailWorkflowResolutionContext{
			WorkflowID:       stringPtr(workflowID.String()),
			WorkflowName:     stringPtr("Priority Roofing"),
			ResolutionSource: "manual_override",
			OverrideMode:     stringPtr("manual"),
		},
	}})

	h := New(HandlerDeps{Mgmt: mgmt, EventBus: events.NewInMemoryBus(nil), Repo: repo})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(httpkit.ContextUserIDKey, userID)
		c.Set(httpkit.ContextRolesKey, []string{"admin"})
		c.Set(httpkit.ContextTenantIDKey, tenantID)
		c.Next()
	})
	h.RegisterRoutes(router.Group("/leads"))

	return detailContextTestState{
		router:         router,
		leadID:         leadID,
		quoteID:        quoteID,
		appointmentID:  appointmentID,
		conversationID: conversationID,
	}
}

func assertDetailContextResponse(t *testing.T, recorder *httptest.ResponseRecorder, state detailContextTestState) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var response leadstransport.LeadDetailContextResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	assertDetailContextLeadAndNotes(t, response, state)
	assertDetailContextActivitySlices(t, response, state)
	assertDetailContextWorkflowAndAnalysis(t, response)
}

func assertDetailContextLeadAndNotes(t *testing.T, response leadstransport.LeadDetailContextResponse, state detailContextTestState) {
	t.Helper()

	if response.Lead.ID != state.leadID {
		t.Fatalf("expected lead id %s, got %s", state.leadID, response.Lead.ID)
	}
	if len(response.Notes) != 1 || response.Notes[0].Body == "" {
		t.Fatalf("expected notes payload, got %+v", response.Notes)
	}
}

func assertDetailContextActivitySlices(t *testing.T, response leadstransport.LeadDetailContextResponse, state detailContextTestState) {
	t.Helper()

	if len(response.Quotes) != 1 || response.Quotes[0].ID != state.quoteID {
		t.Fatalf("expected quote payload, got %+v", response.Quotes)
	}
	if len(response.Appointments) != 1 || response.Appointments[0].ID != state.appointmentID {
		t.Fatalf("expected appointment payload, got %+v", response.Appointments)
	}
	if len(response.Communications.WhatsAppConversations) != 1 || response.Communications.WhatsAppConversations[0].ConversationID != state.conversationID {
		t.Fatalf("expected WhatsApp communications payload, got %+v", response.Communications.WhatsAppConversations)
	}
}

func assertDetailContextWorkflowAndAnalysis(t *testing.T, response leadstransport.LeadDetailContextResponse) {
	t.Helper()

	if response.Workflow == nil || response.Workflow.Resolved == nil || response.Workflow.Resolved.ResolutionSource != "manual_override" {
		t.Fatalf("expected workflow resolution payload, got %+v", response.Workflow)
	}
	if response.CurrentServiceAnalysis == nil || response.CurrentServiceAnalysis.Analysis == nil {
		t.Fatalf("expected current service analysis payload, got %+v", response.CurrentServiceAnalysis)
	}
	if response.CurrentServicePhotoAnalysis == nil || response.CurrentServicePhotoAnalysis.Summary == "" {
		t.Fatalf("expected current service photo analysis payload, got %+v", response.CurrentServicePhotoAnalysis)
	}
}

type detailContextRepoStub struct {
	*leadsrepo.Repository
	lead          leadsrepo.Lead
	services      []leadsrepo.LeadService
	notes         []leadsrepo.LeadNote
	whatsAppItems []leadsrepo.LinkedWhatsAppConversation
	emailItems    []leadsrepo.LinkedIMAPMessage
	analysis      *leadsrepo.AIAnalysis
	photo         *leadsrepo.PhotoAnalysis
}

func (s *detailContextRepoStub) GetByIDWithServices(_ context.Context, _ uuid.UUID, _ uuid.UUID) (leadsrepo.Lead, []leadsrepo.LeadService, error) {
	return s.lead, s.services, nil
}

func (s *detailContextRepoStub) GetByID(_ context.Context, _ uuid.UUID, _ uuid.UUID) (leadsrepo.Lead, error) {
	return s.lead, nil
}

func (s *detailContextRepoStub) ListLeadNotes(_ context.Context, _ uuid.UUID, _ uuid.UUID) ([]leadsrepo.LeadNote, error) {
	return s.notes, nil
}

func (s *detailContextRepoStub) ListLinkedWhatsAppConversations(_ context.Context, _ uuid.UUID, _ uuid.UUID) ([]leadsrepo.LinkedWhatsAppConversation, error) {
	return s.whatsAppItems, nil
}

func (s *detailContextRepoStub) ListLinkedIMAPMessages(_ context.Context, _ uuid.UUID, _ uuid.UUID) ([]leadsrepo.LinkedIMAPMessage, error) {
	return s.emailItems, nil
}

func (s *detailContextRepoStub) GetLatestAIAnalysis(_ context.Context, _ uuid.UUID, _ uuid.UUID) (leadsrepo.AIAnalysis, error) {
	if s.analysis == nil {
		return leadsrepo.AIAnalysis{}, leadsrepo.ErrNotFound
	}
	return *s.analysis, nil
}

func (s *detailContextRepoStub) GetLatestPhotoAnalysis(_ context.Context, _ uuid.UUID, _ uuid.UUID) (leadsrepo.PhotoAnalysis, error) {
	if s.photo == nil {
		return leadsrepo.PhotoAnalysis{}, leadsrepo.ErrPhotoAnalysisNotFound
	}
	return *s.photo, nil
}

type detailContextQuoteReader struct {
	items []quotestransport.QuoteResponse
}

func (r detailContextQuoteReader) ListLeadQuotes(_ context.Context, _ uuid.UUID, _ uuid.UUID) ([]quotestransport.QuoteResponse, error) {
	return r.items, nil
}

type detailContextAppointmentReader struct {
	items []appointmentstransport.AppointmentResponse
}

func (r detailContextAppointmentReader) ListLeadAppointments(_ context.Context, _ uuid.UUID, _ bool, _ uuid.UUID, _ uuid.UUID) ([]appointmentstransport.AppointmentResponse, error) {
	return r.items, nil
}

type detailContextWorkflowReader struct {
	context *leadstransport.LeadDetailWorkflowContext
}

func (r detailContextWorkflowReader) GetLeadWorkflowContext(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *string, _ *string, _ *string) (*leadstransport.LeadDetailWorkflowContext, error) {
	return r.context, nil
}

func stringPtr(value string) *string {
	return &value
}
