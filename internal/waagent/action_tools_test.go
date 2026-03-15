package waagent

import (
	"context"
	"errors"
	"testing"

	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/internal/whatsapp"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

const (
	actionToolsTestQuoteID     = "quote-123"
	actionToolsTestQuoteNumber = "OFF-2026-001"
	actionToolsTestFileName    = "offerte.pdf"
)

type actionToolsTestQuoteWorkflowWriter struct {
	pdfResult   QuotePDFResult
	pdfErr      error
	generateErr error
}

func (w actionToolsTestQuoteWorkflowWriter) DraftQuote(context.Context, uuid.UUID, DraftQuoteInput) (DraftQuoteOutput, error) {
	return DraftQuoteOutput{}, nil
}

func (w actionToolsTestQuoteWorkflowWriter) GenerateQuote(context.Context, uuid.UUID, GenerateQuoteInput) (GenerateQuoteOutput, error) {
	if w.generateErr != nil {
		return GenerateQuoteOutput{}, w.generateErr
	}
	return GenerateQuoteOutput{}, nil
}

func (w actionToolsTestQuoteWorkflowWriter) GetQuotePDF(context.Context, uuid.UUID, SendQuotePDFInput) (QuotePDFResult, error) {
	if w.pdfErr != nil {
		return QuotePDFResult{}, w.pdfErr
	}
	return w.pdfResult, nil
}

type actionToolsTestTransport struct {
	sendFileResult whatsapp.SendResult
	sendFileErr    error
	lastSendFile   whatsapp.SendFileInput
	lastDeviceID   string
}

type actionToolsTestLeadDetailsReader struct {
	err error
}

func (r actionToolsTestLeadDetailsReader) GetLeadDetails(context.Context, uuid.UUID, string) (*LeadDetailsResult, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &LeadDetailsResult{LeadID: testHintLeadID, CustomerName: "Robin"}, nil
}

func (t *actionToolsTestTransport) SendMessage(context.Context, string, string, string) (whatsapp.SendResult, error) {
	return whatsapp.SendResult{}, nil
}

func (t *actionToolsTestTransport) SendChatPresence(context.Context, string, string, string) error {
	return nil
}

func (t *actionToolsTestTransport) SendFile(_ context.Context, deviceID string, input whatsapp.SendFileInput) (whatsapp.SendResult, error) {
	t.lastDeviceID = deviceID
	t.lastSendFile = input
	return t.sendFileResult, t.sendFileErr
}

func (t *actionToolsTestTransport) DownloadMediaFile(context.Context, string, string, string) (whatsapp.DownloadMediaFileResult, error) {
	return whatsapp.DownloadMediaFileResult{}, nil
}

func newActionToolsTestSender(transport WhatsAppTransport) *Sender {
	return &Sender{
		client:  transport,
		queries: senderTestConfigReader{config: waagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}},
		log:     logger.New("development"),
	}
}

type actionToolsTestContext struct {
	context.Context
}

func (c actionToolsTestContext) UserContent() *genai.Content {
	return nil
}

func (c actionToolsTestContext) InvocationID() string {
	return ""
}

func (c actionToolsTestContext) AgentName() string {
	return ""
}

func (c actionToolsTestContext) ReadonlyState() session.ReadonlyState {
	return nil
}

func (c actionToolsTestContext) UserID() string {
	return ""
}

func (c actionToolsTestContext) AppName() string {
	return ""
}

func (c actionToolsTestContext) SessionID() string {
	return ""
}

func (c actionToolsTestContext) Branch() string {
	return ""
}

func (c actionToolsTestContext) Artifacts() agent.Artifacts {
	return nil
}

func (c actionToolsTestContext) State() session.State {
	return nil
}

func (c actionToolsTestContext) FunctionCallID() string {
	return ""
}

func (c actionToolsTestContext) Actions() *session.EventActions {
	return nil
}

func (c actionToolsTestContext) SearchMemory(context.Context, string) (*memory.SearchResponse, error) {
	return nil, nil
}

func (c actionToolsTestContext) ToolConfirmation() *toolconfirmation.ToolConfirmation {
	return nil
}

func (c actionToolsTestContext) RequestConfirmation(string, any) error {
	return nil
}

func newActionToolsTestContext(phoneKey string) tool.Context {
	ctx := context.Background()
	return actionToolsTestContext{Context: context.WithValue(ctx, phoneKeyContextKey{}, phoneKey)}
}

func TestHandleSendQuotePDFSuccess(t *testing.T) {
	t.Parallel()

	transport := &actionToolsTestTransport{sendFileResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	handler := &ToolHandler{
		quoteWorkflowWriter: actionToolsTestQuoteWorkflowWriter{pdfResult: QuotePDFResult{
			QuoteID:     actionToolsTestQuoteID,
			QuoteNumber: actionToolsTestQuoteNumber,
			FileName:    actionToolsTestFileName,
			Data:        []byte("pdf"),
		}},
		sender: newActionToolsTestSender(transport),
	}

	output, err := handler.HandleSendQuotePDF(newActionToolsTestContext(testSenderPhone), uuid.New(), SendQuotePDFInput{QuoteID: actionToolsTestQuoteID})
	if err != nil {
		t.Fatalf("expected send to succeed, got %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success output, got %#v", output)
	}
	if output.Message != "Offerte-pdf verzonden" {
		t.Fatalf("expected success message, got %q", output.Message)
	}
	if transport.lastDeviceID != testSenderDeviceID {
		t.Fatalf("expected device %q, got %q", testSenderDeviceID, transport.lastDeviceID)
	}
	if transport.lastSendFile.PhoneNumber != testSenderPhone {
		t.Fatalf("expected phone %q, got %q", testSenderPhone, transport.lastSendFile.PhoneNumber)
	}
	if transport.lastSendFile.Caption != "Offerte OFF-2026-001 als pdf." {
		t.Fatalf("expected default caption, got %q", transport.lastSendFile.Caption)
	}
	if transport.lastSendFile.Attachment == nil {
		t.Fatal("expected attachment to be set")
	}
	if transport.lastSendFile.Attachment.Filename != actionToolsTestFileName {
		t.Fatalf("expected filename offerte.pdf, got %q", transport.lastSendFile.Attachment.Filename)
	}
}

func TestHandleSendQuotePDFReturnsSafeMessageWhenFetchFails(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("pdf backend timeout")
	handler := &ToolHandler{
		quoteWorkflowWriter: actionToolsTestQuoteWorkflowWriter{pdfErr: expectedErr},
		sender:              newActionToolsTestSender(&actionToolsTestTransport{}),
	}

	output, err := handler.HandleSendQuotePDF(newActionToolsTestContext(testSenderPhone), uuid.New(), SendQuotePDFInput{QuoteID: actionToolsTestQuoteID})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error to be returned, got %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure output, got %#v", output)
	}
	if output.Message != "Ik kan de offerte-pdf nu niet ophalen. Probeer het later opnieuw." {
		t.Fatalf("expected safe fetch error message, got %q", output.Message)
	}
	if output.Message == expectedErr.Error() {
		t.Fatalf("expected raw error to stay hidden, got %q", output.Message)
	}
}

func TestHandleSendQuotePDFReturnsSafeMessageWhenSendFails(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("provider refused upload")
	handler := &ToolHandler{
		quoteWorkflowWriter: actionToolsTestQuoteWorkflowWriter{pdfResult: QuotePDFResult{
			QuoteID:     actionToolsTestQuoteID,
			QuoteNumber: actionToolsTestQuoteNumber,
			FileName:    actionToolsTestFileName,
			Data:        []byte("pdf"),
		}},
		sender: newActionToolsTestSender(&actionToolsTestTransport{sendFileErr: expectedErr}),
	}

	output, err := handler.HandleSendQuotePDF(newActionToolsTestContext(testSenderPhone), uuid.New(), SendQuotePDFInput{QuoteID: actionToolsTestQuoteID})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error to be returned, got %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure output, got %#v", output)
	}
	if output.Message != "Ik kan de offerte-pdf nu niet via WhatsApp versturen. Probeer het later opnieuw." {
		t.Fatalf("expected safe send error message, got %q", output.Message)
	}
	if output.Message == expectedErr.Error() {
		t.Fatalf("expected raw error to stay hidden, got %q", output.Message)
	}
	if output.QuoteID != actionToolsTestQuoteID || output.QuoteNumber != actionToolsTestQuoteNumber || output.FileName != actionToolsTestFileName {
		t.Fatalf("expected quote metadata to be preserved, got %#v", output)
	}
}

func TestHandleGetLeadDetailsReturnsSafeMessageOnReaderFailure(t *testing.T) {
	t.Parallel()

	handler := &ToolHandler{leadDetailsReader: actionToolsTestLeadDetailsReader{err: errors.New("backend down")}}

	output, err := handler.HandleGetLeadDetails(newActionToolsTestContext(testSenderPhone), uuid.New(), GetLeadDetailsInput{LeadID: testHintLeadID})
	if err == nil {
		t.Fatal("expected reader error")
	}
	if output.Message != "Ik kan de leadgegevens nu niet ophalen. Probeer het later opnieuw." {
		t.Fatalf("unexpected safe error message %q", output.Message)
	}
}

func TestHandleGenerateQuoteReturnsSafeMessageOnWriterFailure(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("quote backend timeout")
	handler := &ToolHandler{quoteWorkflowWriter: actionToolsTestQuoteWorkflowWriter{generateErr: expectedErr}, leadMutationWriter: testLeadMutationWriter{}}
	leadID := uuid.NewString()

	output, err := handler.HandleGenerateQuote(newActionToolsTestContext(testSenderPhone), uuid.New(), GenerateQuoteInput{LeadID: leadID, Prompt: "Maak een offerte"})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if output.Message != "Ik kan de offerte nu niet genereren. Probeer het later opnieuw." {
		t.Fatalf("unexpected safe error message %q", output.Message)
	}
}
