package waagent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"portal_final_backend/internal/scheduler"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type testQuotesReader struct{}

func (testQuotesReader) ListQuotesByOrganization(context.Context, uuid.UUID, *string) ([]QuoteSummary, error) {
	return nil, nil
}

type testAppointmentsReader struct{}

func (testAppointmentsReader) ListAppointmentsByOrganization(context.Context, uuid.UUID, *time.Time, *time.Time) ([]AppointmentSummary, error) {
	return nil, nil
}

type testLeadSearchReader struct{}

func (testLeadSearchReader) SearchLeads(context.Context, uuid.UUID, string, int) ([]LeadSearchResult, error) {
	return nil, nil
}

type testNavigationLinkReader struct{}

func (testNavigationLinkReader) GetNavigationLink(context.Context, uuid.UUID, string) (*NavigationLinkResult, error) {
	return nil, nil
}

type testLeadDetailsReader struct{}

func (testLeadDetailsReader) GetLeadDetails(context.Context, uuid.UUID, string) (*LeadDetailsResult, error) {
	return nil, nil
}

type testCatalogSearchReader struct{}

func (testCatalogSearchReader) SearchProductMaterials(context.Context, uuid.UUID, SearchProductMaterialsInput) (SearchProductMaterialsOutput, error) {
	return SearchProductMaterialsOutput{}, nil
}

type testLeadMutationWriter struct{}

func (testLeadMutationWriter) CreateLead(context.Context, uuid.UUID, CreateLeadInput) (CreateLeadOutput, error) {
	return CreateLeadOutput{}, nil
}

func (testLeadMutationWriter) ResolveServiceID(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (uuid.UUID, error) {
	return uuid.UUID{}, nil
}

func (testLeadMutationWriter) UpdateLeadDetails(context.Context, uuid.UUID, UpdateLeadDetailsInput) ([]string, error) {
	return nil, nil
}

func (testLeadMutationWriter) AskCustomerClarification(context.Context, uuid.UUID, AskCustomerClarificationInput) error {
	return nil
}

func (testLeadMutationWriter) SaveNote(context.Context, uuid.UUID, SaveNoteInput) error {
	return nil
}

func (testLeadMutationWriter) UpdateLeadStatus(context.Context, uuid.UUID, UpdateStatusInput) (string, error) {
	return "", nil
}

type testQuoteWorkflowWriter struct{}

func (testQuoteWorkflowWriter) DraftQuote(context.Context, uuid.UUID, DraftQuoteInput) (DraftQuoteOutput, error) {
	return DraftQuoteOutput{}, nil
}

func (testQuoteWorkflowWriter) GenerateQuote(context.Context, uuid.UUID, GenerateQuoteInput) (GenerateQuoteOutput, error) {
	return GenerateQuoteOutput{}, nil
}

func (testQuoteWorkflowWriter) GetQuotePDF(context.Context, uuid.UUID, SendQuotePDFInput) (QuotePDFResult, error) {
	return QuotePDFResult{}, nil
}

type testPhotoAttacher struct{}

func (testPhotoAttacher) AttachCurrentWhatsAppPhoto(context.Context, uuid.UUID, AttachCurrentWhatsAppPhotoInput, CurrentInboundMessage) (AttachCurrentWhatsAppPhotoOutput, error) {
	return AttachCurrentWhatsAppPhotoOutput{}, nil
}

type testInboxMessageSync struct{}

func (testInboxMessageSync) PersistIncomingWhatsAppMessage(context.Context, uuid.UUID, string, string, string, *string, []byte) error {
	return nil
}

func (testInboxMessageSync) UpdateIncomingWhatsAppMessage(context.Context, uuid.UUID, string, string, []byte) error {
	return nil
}

type testVisitSlotReader struct{}

func (testVisitSlotReader) GetAvailableVisitSlots(context.Context, uuid.UUID, string, string, int) ([]VisitSlotSummary, error) {
	return nil, nil
}

type testVisitMutationWriter struct{}

func (testVisitMutationWriter) ScheduleVisit(context.Context, uuid.UUID, ScheduleVisitInput) (*AppointmentSummary, error) {
	return nil, nil
}

func (testVisitMutationWriter) RescheduleVisit(context.Context, uuid.UUID, RescheduleVisitInput) (*AppointmentSummary, error) {
	return nil, nil
}

func (testVisitMutationWriter) CancelVisit(context.Context, uuid.UUID, CancelVisitInput) error {
	return nil
}

type testInboxWriter struct{}

func (testInboxWriter) PersistOutgoingWhatsAppMessage(context.Context, uuid.UUID, *uuid.UUID, string, string, *string) error {
	return nil
}

type testStorage struct{}

func (testStorage) DownloadFile(context.Context, string, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (testStorage) UploadFile(context.Context, string, string, string, string, io.Reader, int64) (string, error) {
	return "", nil
}

func (testStorage) ValidateContentType(string) error {
	return nil
}

func (testStorage) ValidateFileSize(int64) error {
	return nil
}

type testAudioScheduler struct{}

func (testAudioScheduler) EnqueueWAAgentVoiceTranscription(context.Context, scheduler.WAAgentVoiceTranscriptionPayload) error {
	return nil
}

type testAudioTranscriber struct{}

func (testAudioTranscriber) Name() string {
	return "test"
}

func (testAudioTranscriber) Transcribe(context.Context, AudioTranscriptionInput) (AudioTranscriptionResult, error) {
	return AudioTranscriptionResult{}, nil
}

func validModuleDependencies() ModuleDependencies {
	return ModuleDependencies{
		WhatsAppClient:              &whatsapp.Client{},
		QuotesReader:                testQuotesReader{},
		AppointmentsReader:          testAppointmentsReader{},
		LeadSearchReader:            testLeadSearchReader{},
		LeadDetailsReader:           testLeadDetailsReader{},
		NavigationLinkReader:        testNavigationLinkReader{},
		CatalogSearchReader:         testCatalogSearchReader{},
		LeadMutationWriter:          testLeadMutationWriter{},
		QuoteWorkflowWriter:         testQuoteWorkflowWriter{},
		CurrentInboundPhotoAttacher: testPhotoAttacher{},
		InboxMessageSync:            testInboxMessageSync{},
		VisitSlotReader:             testVisitSlotReader{},
		VisitMutationWriter:         testVisitMutationWriter{},
		RedisClient:                 redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}),
		InboxWriter:                 testInboxWriter{},
		Logger:                      logger.New("development"),
	}
}

func TestValidateModuleDependenciesRequiresCoreProductionDeps(t *testing.T) {
	t.Parallel()

	err := validateModuleDependencies(nil, ModuleConfig{}, ModuleDependencies{})
	if err == nil {
		t.Fatal("expected validation error")
	}

	message := err.Error()
	for _, expected := range []string{"database pool", "logger", "moonshot api key", "llm model", "redis client"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("expected %q in validation error, got %q", expected, message)
		}
	}
}

func TestValidateModuleDependenciesRejectsPartialAudioConfig(t *testing.T) {
	t.Parallel()

	deps := validModuleDependencies()
	deps.Storage = testStorage{}

	err := validateModuleDependencies(&pgxpool.Pool{}, ModuleConfig{MoonshotAPIKey: "key", LLMModel: "model"}, deps)
	if err == nil {
		t.Fatal("expected audio validation error")
	}
	if !strings.Contains(err.Error(), "invalid audio transcription configuration") {
		t.Fatalf("expected audio validation error, got %q", err)
	}
	for _, expected := range []string{"attachment bucket", "transcription scheduler", "audio transcriber"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected %q in validation error, got %q", expected, err)
		}
	}
}

func TestValidateModuleDependenciesAcceptsCompleteAudioConfig(t *testing.T) {
	t.Parallel()

	deps := validModuleDependencies()
	deps.Storage = testStorage{}
	deps.AttachmentBucket = "attachments"
	deps.TranscriptionScheduler = testAudioScheduler{}
	deps.AudioTranscriber = testAudioTranscriber{}

	err := validateModuleDependencies(&pgxpool.Pool{}, ModuleConfig{MoonshotAPIKey: "key", LLMModel: "model"}, deps)
	if err != nil {
		t.Fatalf("expected valid configuration, got %v", err)
	}
}
