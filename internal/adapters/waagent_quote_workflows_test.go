package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	storageadapter "portal_final_backend/internal/adapters/storage"
	identityservice "portal_final_backend/internal/identity/service"
	waagent "portal_final_backend/internal/waagent"
	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/internal/whatsapp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const currentPhotoFilename = "current.jpg"

type fakeWAAgentMediaDownloader struct {
	lastDeviceID  string
	lastMessageID string
	lastPhone     string
	result        whatsapp.DownloadMediaFileResult
	err           error
}

func (f *fakeWAAgentMediaDownloader) DownloadMediaFile(_ context.Context, deviceID string, messageID string, phoneNumber string) (whatsapp.DownloadMediaFileResult, error) {
	f.lastDeviceID = deviceID
	f.lastMessageID = messageID
	f.lastPhone = phoneNumber
	return f.result, f.err
}

type fakeWAAgentStorage struct {
	uploadedFileName    string
	uploadedContentType string
	uploadedData        []byte
	fileKey             string
	validateContentErr  error
	validateSizeErr     error
}

func (f *fakeWAAgentStorage) GenerateUploadURL(context.Context, string, string, string, string, int64) (*storageadapter.PresignedURL, error) {
	return nil, nil
}
func (f *fakeWAAgentStorage) GenerateDownloadURL(context.Context, string, string) (*storageadapter.PresignedURL, error) {
	return nil, nil
}
func (f *fakeWAAgentStorage) DownloadFile(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (f *fakeWAAgentStorage) DeleteObject(context.Context, string, string) error { return nil }
func (f *fakeWAAgentStorage) EnsureBucketExists(context.Context, string) error   { return nil }
func (f *fakeWAAgentStorage) GetMaxFileSize() int64                              { return 10 << 20 }
func (f *fakeWAAgentStorage) ValidateContentType(_ string) error                 { return f.validateContentErr }
func (f *fakeWAAgentStorage) ValidateFileSize(_ int64) error                     { return f.validateSizeErr }
func (f *fakeWAAgentStorage) UploadFile(_ context.Context, _, _, fileName, contentType string, reader io.Reader, _ int64) (string, error) {
	f.uploadedFileName = fileName
	f.uploadedContentType = contentType
	data, _ := io.ReadAll(reader)
	f.uploadedData = data
	if f.fileKey == "" {
		f.fileKey = "bucket/path/" + fileName
	}
	return f.fileKey, nil
}

type fakeWAAgentLeadActions struct {
	params identityservice.CreateLeadAttachmentParams
	result identityservice.CreateLeadAttachmentResult
	err    error
}

func (f *fakeWAAgentLeadActions) CreateAttachment(_ context.Context, params identityservice.CreateLeadAttachmentParams) (identityservice.CreateLeadAttachmentResult, error) {
	f.params = params
	if f.result.AttachmentID == uuid.Nil {
		f.result.AttachmentID = uuid.New()
	}
	return f.result, f.err
}

type fakeWAAgentMessageReader struct {
	recent    []waagentdb.GetRecentInboundAgentMessagesRow
	recentArg waagentdb.GetRecentInboundAgentMessagesParams
}

func (f *fakeWAAgentMessageReader) GetRecentInboundAgentMessages(_ context.Context, arg waagentdb.GetRecentInboundAgentMessagesParams) ([]waagentdb.GetRecentInboundAgentMessagesRow, error) {
	f.recentArg = arg
	return f.recent, nil
}

func TestWAAgentCurrentInboundPhotoAdapterUsesCurrentImageMessage(t *testing.T) {
	t.Parallel()

	metadata := mustWAAgentMetadata(t, "DEVICE-1", "image", currentPhotoFilename)
	downloader := &fakeWAAgentMediaDownloader{result: whatsapp.DownloadMediaFileResult{DownloadMediaResult: whatsapp.DownloadMediaResult{Filename: currentPhotoFilename, MediaType: "image"}, ContentType: "image/jpeg", Data: []byte("image-data")}}
	storage := &fakeWAAgentStorage{}
	leadActions := &fakeWAAgentLeadActions{}
	adapter := NewWAAgentCurrentInboundPhotoAdapter(downloader, storage, "attachments", leadActions, &fakeWAAgentMessageReader{})

	orgID := uuid.New()
	leadID := uuid.New()
	serviceID := uuid.New()
	output, err := adapter.AttachCurrentWhatsAppPhoto(context.Background(), orgID, waagent.AttachCurrentWhatsAppPhotoInput{LeadID: leadID.String(), LeadServiceID: serviceID.String()}, waagent.CurrentInboundMessage{ExternalMessageID: "MSG-CURRENT", PhoneNumber: "31612345678", Metadata: metadata})
	if err != nil {
		t.Fatalf("AttachCurrentWhatsAppPhoto error: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got %#v", output)
	}
	if downloader.lastMessageID != "MSG-CURRENT" {
		t.Fatalf("expected current message id, got %q", downloader.lastMessageID)
	}
	if storage.uploadedFileName != currentPhotoFilename {
		t.Fatalf("expected uploaded filename %s, got %q", currentPhotoFilename, storage.uploadedFileName)
	}
	if leadActions.params.ServiceID != serviceID {
		t.Fatalf("expected service id %s, got %s", serviceID, leadActions.params.ServiceID)
	}
}

func TestWAAgentCurrentInboundPhotoAdapterFallsBackToRecentImage(t *testing.T) {
	t.Parallel()

	historyMetadata := mustWAAgentMetadata(t, "DEVICE-HISTORY", "image", "history.png")
	history := waagentdb.GetRecentInboundAgentMessagesRow{
		OrganizationID:    pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true},
		PhoneNumber:       "31612345678",
		Role:              "user",
		ExternalMessageID: pgtype.Text{String: "MSG-HISTORY", Valid: true},
		Metadata:          historyMetadata,
	}
	downloader := &fakeWAAgentMediaDownloader{result: whatsapp.DownloadMediaFileResult{DownloadMediaResult: whatsapp.DownloadMediaResult{Filename: "downloaded.png", MediaType: "image"}, ContentType: "image/png", Data: []byte("history-image")}}
	storage := &fakeWAAgentStorage{}
	leadActions := &fakeWAAgentLeadActions{}
	historyReader := &fakeWAAgentMessageReader{recent: []waagentdb.GetRecentInboundAgentMessagesRow{history}}
	adapter := NewWAAgentCurrentInboundPhotoAdapter(downloader, storage, "attachments", leadActions, historyReader)

	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	leadID := uuid.New()
	serviceID := uuid.New()
	output, err := adapter.AttachCurrentWhatsAppPhoto(context.Background(), orgID, waagent.AttachCurrentWhatsAppPhotoInput{LeadID: leadID.String(), LeadServiceID: serviceID.String()}, waagent.CurrentInboundMessage{ExternalMessageID: "MSG-TEXT", PhoneNumber: "31612345678", Metadata: mustWAAgentMetadata(t, "DEVICE-TEXT", "text", "")})
	if err != nil {
		t.Fatalf("AttachCurrentWhatsAppPhoto error: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got %#v", output)
	}
	if downloader.lastDeviceID != "DEVICE-HISTORY" {
		t.Fatalf("expected history device id, got %q", downloader.lastDeviceID)
	}
	if downloader.lastMessageID != "MSG-HISTORY" {
		t.Fatalf("expected history message id, got %q", downloader.lastMessageID)
	}
	if historyReader.recentArg.PhoneNumber != "31612345678" || historyReader.recentArg.Limit != 10 {
		t.Fatalf("unexpected history lookup args: %#v", historyReader.recentArg)
	}
	if storage.uploadedFileName != "downloaded.png" {
		t.Fatalf("expected downloaded filename, got %q", storage.uploadedFileName)
	}
}

func mustWAAgentMetadata(t *testing.T, deviceID string, messageType string, filename string) []byte {
	t.Helper()
	payload := map[string]any{
		"device_id": deviceID,
		"portal": map[string]any{
			"messageType": messageType,
		},
	}
	if filename != "" {
		payload["portal"].(map[string]any)["attachment"] = map[string]any{"mediaType": messageType, "filename": filename}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	return data
}
