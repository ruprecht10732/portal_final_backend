package whatsappagent

import (
	"context"
	"errors"
	"testing"

	"portal_final_backend/internal/whatsapp"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

const testSenderPhone = "+31698765432"
const testSenderDeviceID = "device-123"
const testSenderMessageID = "MSG-123"

type senderTestTransport struct {
	sendMessageResult    whatsapp.SendResult
	sendMessageErr       error
	sendFileResult       whatsapp.SendResult
	sendFileErr          error
	lastSendMessagePhone string
	lastSendMessageText  string
	lastPresencePhone    string
	lastPresenceAction   string
}

func (t *senderTestTransport) SendMessage(context.Context, string, string, string) (whatsapp.SendResult, error) {
	t.lastSendMessagePhone = testSenderPhone
	t.lastSendMessageText = "captured"
	return t.sendMessageResult, t.sendMessageErr
}

func (t *senderTestTransport) SendChatPresence(_ context.Context, _ string, phoneNumber string, action string) error {
	t.lastPresencePhone = phoneNumber
	t.lastPresenceAction = action
	return nil
}

func (t *senderTestTransport) SendFile(context.Context, string, whatsapp.SendFileInput) (whatsapp.SendResult, error) {
	return t.sendFileResult, t.sendFileErr
}

func (t *senderTestTransport) DownloadMediaFile(context.Context, string, string, string, ...string) (whatsapp.DownloadMediaFileResult, error) {
	return whatsapp.DownloadMediaFileResult{}, nil
}

type senderTestConfigReader struct {
	config whatsappagentdb.RacWhatsappAgentConfig
	err    error
}

func (r senderTestConfigReader) GetAgentConfig(context.Context) (whatsappagentdb.RacWhatsappAgentConfig, error) {
	if r.err != nil {
		return whatsappagentdb.RacWhatsappAgentConfig{}, r.err
	}
	return r.config, nil
}

type senderTestInboxWriter struct {
	lastOrgID     uuid.UUID
	lastPhone     string
	lastBody      string
	lastMessageID *string
	err           error
	calls         int
}

func (w *senderTestInboxWriter) PersistOutgoingWhatsAppMessage(_ context.Context, organizationID uuid.UUID, _ *uuid.UUID, phoneNumber string, body string, externalMessageID *string) error {
	w.lastOrgID = organizationID
	w.lastPhone = phoneNumber
	w.lastBody = body
	w.lastMessageID = externalMessageID
	w.calls++
	return w.err
}

func newTestSender(transport WhatsAppTransport, configReader AgentConfigReader, inboxWriter InboxWriter) *Sender {
	return &Sender{client: transport, queries: configReader, inboxWriter: inboxWriter, log: logger.New("development")}
}

func TestSenderSendReplyReturnsConfigError(t *testing.T) {
	t.Parallel()

	sender := newTestSender(&senderTestTransport{}, senderTestConfigReader{err: errors.New("missing config")}, nil)
	err := sender.SendReply(context.Background(), uuid.New(), testSenderPhone, "Hallo")
	if err == nil || err.Error() != "missing config" {
		t.Fatalf("expected config error, got %v", err)
	}
}

func TestSenderSendReplyReturnsTransportError(t *testing.T) {
	t.Parallel()

	transport := &senderTestTransport{sendMessageErr: errors.New("provider down")}
	sender := newTestSender(transport, senderTestConfigReader{config: whatsappagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}}, nil)
	err := sender.SendReply(context.Background(), uuid.New(), testSenderPhone, "Hallo")
	if err == nil || err.Error() != "provider down" {
		t.Fatalf("expected send error, got %v", err)
	}
}

func TestSenderSendReplyPersistsOutgoingMessage(t *testing.T) {
	t.Parallel()

	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	inboxWriter := &senderTestInboxWriter{}
	orgID := uuid.New()
	sender := newTestSender(transport, senderTestConfigReader{config: whatsappagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}}, inboxWriter)

	err := sender.SendReply(context.Background(), orgID, testSenderPhone, "Hallo")
	if err != nil {
		t.Fatalf("expected send to succeed, got %v", err)
	}
	if inboxWriter.calls != 1 {
		t.Fatalf("expected one inbox persist call, got %d", inboxWriter.calls)
	}
	if inboxWriter.lastOrgID != orgID || inboxWriter.lastPhone != testSenderPhone || inboxWriter.lastBody != "Hallo" {
		t.Fatalf("unexpected inbox payload: %#v", inboxWriter)
	}
	if inboxWriter.lastMessageID == nil || *inboxWriter.lastMessageID != testSenderMessageID {
		t.Fatalf("expected message id to be persisted, got %#v", inboxWriter.lastMessageID)
	}
}

func TestSenderSendReplyIgnoresInboxPersistenceFailure(t *testing.T) {
	t.Parallel()

	transport := &senderTestTransport{sendMessageResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	inboxWriter := &senderTestInboxWriter{err: errors.New("db write failed")}
	sender := newTestSender(transport, senderTestConfigReader{config: whatsappagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}}, inboxWriter)

	err := sender.SendReply(context.Background(), uuid.New(), testSenderPhone, "Hallo")
	if err != nil {
		t.Fatalf("expected send to succeed despite inbox persist failure, got %v", err)
	}
	if inboxWriter.calls != 1 {
		t.Fatalf("expected one inbox persist attempt, got %d", inboxWriter.calls)
	}
}

func TestSenderSendFileReplyUsesFilenameWhenCaptionEmpty(t *testing.T) {
	t.Parallel()

	transport := &senderTestTransport{sendFileResult: whatsapp.SendResult{MessageID: testSenderMessageID}}
	inboxWriter := &senderTestInboxWriter{}
	sender := newTestSender(transport, senderTestConfigReader{config: whatsappagentdb.RacWhatsappAgentConfig{DeviceID: testSenderDeviceID}}, inboxWriter)

	err := sender.SendFileReply(context.Background(), uuid.New(), testSenderPhone, "", "offerte.pdf", []byte("pdf"))
	if err != nil {
		t.Fatalf("expected file send to succeed, got %v", err)
	}
	if inboxWriter.lastBody != "offerte.pdf" {
		t.Fatalf("expected empty caption to persist filename body, got %q", inboxWriter.lastBody)
	}
}
