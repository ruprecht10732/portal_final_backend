package engine

import (
	"context"
	"testing"

	"portal_final_backend/internal/whatsapp"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
	iter "iter"
)

const (
	actionToolsTestPhone       = "+31612345678"
	actionToolsTestDeviceID    = "agent-device"
	actionToolsTestQuoteID     = "quote-123"
	actionToolsTestQuoteNumber = "OFF-2026-001"
)

type actionToolsQuoteWorkflowWriterStub struct {
	lastPDFInput SendQuotePDFInput
	pdfResult    QuotePDFResult
	pdfErr       error
}

func (s *actionToolsQuoteWorkflowWriterStub) DraftQuote(context.Context, uuid.UUID, DraftQuoteInput) (DraftQuoteOutput, error) {
	return DraftQuoteOutput{}, nil
}

func (s *actionToolsQuoteWorkflowWriterStub) GenerateQuote(context.Context, uuid.UUID, GenerateQuoteInput) (GenerateQuoteOutput, error) {
	return GenerateQuoteOutput{}, nil
}

func (s *actionToolsQuoteWorkflowWriterStub) GetQuotePDF(_ context.Context, _ uuid.UUID, input SendQuotePDFInput) (QuotePDFResult, error) {
	s.lastPDFInput = input
	return s.pdfResult, s.pdfErr
}

type actionToolsSenderTransportStub struct {
	lastFileInput whatsapp.SendFileInput
}

func (s *actionToolsSenderTransportStub) SendMessage(context.Context, string, string, string) (whatsapp.SendResult, error) {
	return whatsapp.SendResult{}, nil
}

func (s *actionToolsSenderTransportStub) SendChatPresence(context.Context, string, string, string) error {
	return nil
}

func (s *actionToolsSenderTransportStub) SendFile(_ context.Context, _ string, input whatsapp.SendFileInput) (whatsapp.SendResult, error) {
	s.lastFileInput = input
	return whatsapp.SendResult{MessageID: "msg-1"}, nil
}

func (s *actionToolsSenderTransportStub) DownloadMediaFile(context.Context, string, string, string, ...string) (whatsapp.DownloadMediaFileResult, error) {
	return whatsapp.DownloadMediaFileResult{}, nil
}

type actionToolsAgentConfigReaderStub struct{}

func (actionToolsAgentConfigReaderStub) GetAgentConfig(context.Context) (whatsappagentdb.RacWhatsappAgentConfig, error) {
	return whatsappagentdb.RacWhatsappAgentConfig{DeviceID: actionToolsTestDeviceID}, nil
}

type actionToolsInboxWriterStub struct{}

func (actionToolsInboxWriterStub) PersistOutgoingWhatsAppMessage(context.Context, uuid.UUID, *uuid.UUID, string, string, *string) error {
	return nil
}

type actionToolsStateStub struct{}

func (actionToolsStateStub) Get(string) (any, error) { return nil, nil }
func (actionToolsStateStub) Set(string, any) error   { return nil }
func (actionToolsStateStub) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for key, value := range map[string]any{} {
			if !yield(key, value) {
				return
			}
		}
	}
}

type actionToolsReadonlyStateStub struct{}

func (actionToolsReadonlyStateStub) Get(string) (any, error) { return nil, nil }
func (actionToolsReadonlyStateStub) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for key, value := range map[string]any{} {
			if !yield(key, value) {
				return
			}
		}
	}
}

type actionToolsToolContextStub struct {
	context.Context
}

func newActionToolsTestContext(phone string) *actionToolsToolContextStub {
	ctx := context.WithValue(context.Background(), phoneKeyContextKey{}, phone)
	ctx = context.WithValue(ctx, orgIDContextKey{}, uuid.New())
	return &actionToolsToolContextStub{Context: ctx}
}

func (*actionToolsToolContextStub) UserContent() *genai.Content { return nil }
func (*actionToolsToolContextStub) InvocationID() string        { return "invocation-1" }
func (*actionToolsToolContextStub) AgentName() string           { return "WhatsAppAgent" }
func (*actionToolsToolContextStub) ReadonlyState() session.ReadonlyState {
	return actionToolsReadonlyStateStub{}
}
func (*actionToolsToolContextStub) UserID() string                 { return "user-1" }
func (*actionToolsToolContextStub) AppName() string                { return "app-1" }
func (*actionToolsToolContextStub) SessionID() string              { return "session-1" }
func (*actionToolsToolContextStub) Branch() string                 { return "" }
func (*actionToolsToolContextStub) Artifacts() agent.Artifacts     { return nil }
func (*actionToolsToolContextStub) State() session.State           { return actionToolsStateStub{} }
func (*actionToolsToolContextStub) FunctionCallID() string         { return "call-1" }
func (*actionToolsToolContextStub) Actions() *session.EventActions { return &session.EventActions{} }
func (*actionToolsToolContextStub) SearchMemory(context.Context, string) (*memory.SearchResponse, error) {
	return &memory.SearchResponse{}, nil
}
func (*actionToolsToolContextStub) ToolConfirmation() *toolconfirmation.ToolConfirmation {
	return nil
}
func (*actionToolsToolContextStub) RequestConfirmation(string, any) error { return nil }

func newActionToolsTestSender(transport WhatsAppTransport) *Sender {
	return &Sender{
		client:      transport,
		queries:     actionToolsAgentConfigReaderStub{},
		inboxWriter: actionToolsInboxWriterStub{},
		log:         logger.New("development"),
	}
}

func TestHandleSendQuotePDFReusesSingleRecentQuoteHint(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	workflow := &actionToolsQuoteWorkflowWriterStub{pdfResult: QuotePDFResult{QuoteID: actionToolsTestQuoteID, QuoteNumber: actionToolsTestQuoteNumber, FileName: "offerte.pdf", Data: []byte("pdf")}}
	transport := &actionToolsSenderTransportStub{}
	store := NewConversationLeadHintStore()
	store.Set(context.Background(), orgID.String(), actionToolsTestPhone, ConversationLeadHint{RecentQuotes: []RecentQuoteHint{{QuoteID: actionToolsTestQuoteID, QuoteNumber: actionToolsTestQuoteNumber}}})
	handler := &ToolHandler{quoteWorkflowWriter: workflow, sender: newActionToolsTestSender(transport), leadHintStore: store}

	output, err := handler.HandleSendQuotePDF(newActionToolsTestContext(actionToolsTestPhone), orgID, SendQuotePDFInput{})
	if err != nil {
		t.Fatalf("expected quote pdf send to succeed, got %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success output, got %#v", output)
	}
	if workflow.lastPDFInput.QuoteID != actionToolsTestQuoteID {
		t.Fatalf("expected quote id to be reused from hint, got %q", workflow.lastPDFInput.QuoteID)
	}
	if transport.lastFileInput.Caption != "Offerte OFF-2026-001 als pdf." {
		t.Fatalf("expected default caption from quote number, got %q", transport.lastFileInput.Caption)
	}
}

func TestHandleSendQuotePDFRequestsClarificationForAmbiguousRecentQuotes(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	store := NewConversationLeadHintStore()
	store.Set(context.Background(), orgID.String(), actionToolsTestPhone, ConversationLeadHint{RecentQuotes: []RecentQuoteHint{{QuoteID: "quote-1", QuoteNumber: "OFF-1"}, {QuoteID: "quote-2", QuoteNumber: "OFF-2"}}})
	handler := &ToolHandler{quoteWorkflowWriter: &actionToolsQuoteWorkflowWriterStub{}, sender: newActionToolsTestSender(&actionToolsSenderTransportStub{}), leadHintStore: store}

	output, err := handler.HandleSendQuotePDF(newActionToolsTestContext(actionToolsTestPhone), orgID, SendQuotePDFInput{})
	if err != nil {
		t.Fatalf("expected graceful clarification output, got %v", err)
	}
	if output.Success {
		t.Fatalf("expected clarification output, got %#v", output)
	}
	if output.Message != "Noem het offertenummer van de offerte die ik moet sturen." {
		t.Fatalf("unexpected clarification message: %q", output.Message)
	}
}
