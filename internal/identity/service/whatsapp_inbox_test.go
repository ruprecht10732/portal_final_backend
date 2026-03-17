package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"portal_final_backend/internal/identity/repository"

	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const testWhatsAppMediaPhone = "+31686261598"
const testAudioOggContentType = "audio/ogg"
const testWhatsAppChatLID = "212450775417035@lid"

func TestClearStaleWhatsAppConversationLeadRemovesLeadIDAndPersistsCleanup(t *testing.T) {
	t.Parallel()

	conversationID := uuid.New()
	organizationID := uuid.New()
	leadID := uuid.New()
	conversation := &repository.WhatsAppConversation{ID: conversationID, LeadID: &leadID}

	called := false
	clearStaleWhatsAppConversationLead(context.Background(), organizationID, conversation, func(ctx context.Context, gotOrganizationID, gotConversationID uuid.UUID, gotLeadID *uuid.UUID) (repository.WhatsAppConversation, error) {
		called = true
		if gotOrganizationID != organizationID {
			t.Fatalf("expected organization %s, got %s", organizationID, gotOrganizationID)
		}
		if gotConversationID != conversationID {
			t.Fatalf("expected conversation %s, got %s", conversationID, gotConversationID)
		}
		if gotLeadID != nil {
			t.Fatalf("expected cleanup to clear lead id, got %v", *gotLeadID)
		}
		return repository.WhatsAppConversation{ID: conversationID, OrganizationID: organizationID, LeadID: nil}, nil
	})
	if !called {
		t.Fatal("expected cleanup function to be called")
	}
	if conversation.LeadID != nil {
		t.Fatalf("expected in-memory lead id to be cleared, got %v", *conversation.LeadID)
	}
}

func TestClearStaleWhatsAppConversationLeadClearsCurrentResponseEvenWhenCleanupFails(t *testing.T) {
	t.Parallel()

	leadID := uuid.New()
	conversation := &repository.WhatsAppConversation{ID: uuid.New(), LeadID: &leadID}

	clearStaleWhatsAppConversationLead(context.Background(), uuid.New(), conversation, func(ctx context.Context, organizationID, conversationID uuid.UUID, gotLeadID *uuid.UUID) (repository.WhatsAppConversation, error) {
		return repository.WhatsAppConversation{}, errors.New("database unavailable")
	})
	if conversation.LeadID != nil {
		t.Fatalf("expected in-memory lead id to stay cleared on cleanup failure, got %v", *conversation.LeadID)
	}
}

func TestClearStaleWhatsAppConversationLeadNoopsWithoutLeadID(t *testing.T) {
	t.Parallel()

	conversation := &repository.WhatsAppConversation{ID: uuid.New()}
	called := false
	clearStaleWhatsAppConversationLead(context.Background(), uuid.New(), conversation, func(ctx context.Context, organizationID, conversationID uuid.UUID, gotLeadID *uuid.UUID) (repository.WhatsAppConversation, error) {
		called = true
		return repository.WhatsAppConversation{}, nil
	})
	if called {
		t.Fatal("expected no cleanup call when no lead id is present")
	}
}

func TestAppErrIsRecognizesNotFound(t *testing.T) {
	t.Parallel()

	if !apperr.Is(apperr.NotFound("lead not found"), apperr.KindNotFound) {
		t.Fatal("expected apperr.Is to recognize not found errors")
	}
}

func TestMergeWhatsAppMediaCacheMetadataRoundTripsCacheFields(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"portal":{"messageType":"audio","attachment":{"mediaType":"audio","filename":"note.ogg","remoteUrl":"https://provider/media"}}}`)
	merged, err := mergeWhatsAppMediaCacheMetadata(raw, whatsAppMediaCacheMetadata{
		MediaType:   "audio",
		Filename:    "note.ogg",
		StorageKey:  "org/whatsapp-media/conversation/message/note_1234.ogg",
		ContentType: testAudioOggContentType,
		SizeBytes:   12345,
	})
	if err != nil {
		t.Fatalf("mergeWhatsAppMediaCacheMetadata error: %v", err)
	}

	cache, ok := cachedWhatsAppMediaFromMetadata(merged)
	if !ok {
		t.Fatal("expected cached metadata to be present")
	}
	if cache.StorageKey != "org/whatsapp-media/conversation/message/note_1234.ogg" {
		t.Fatalf("expected storage key to round-trip, got %q", cache.StorageKey)
	}
	if cache.ContentType != testAudioOggContentType {
		t.Fatalf("expected content type %s, got %q", testAudioOggContentType, cache.ContentType)
	}
	if cache.SizeBytes != 12345 {
		t.Fatalf("expected sizeBytes 12345, got %d", cache.SizeBytes)
	}
	if cache.Filename != "note.ogg" {
		t.Fatalf("expected filename note.ogg, got %q", cache.Filename)
	}
	parsed := parseWhatsAppPortalMetadata(merged)
	if parsed.Attachment == nil || strings.TrimSpace(parsed.Attachment.RemoteURL) != "https://provider/media" {
		t.Fatal("expected existing attachment metadata to be preserved")
	}
}

func TestMergeWhatsAppMediaResponseMetadataOverridesRenderableURL(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"portal":{"messageType":"image","attachment":{"mediaType":"image","filename":"photo.jpg","remoteUrl":"https://provider/media.jpg","storageKey":"org/file.jpg"}}}`)
	merged, err := mergeWhatsAppMediaResponseMetadata(raw, WhatsAppMediaDownloadResult{
		MessageID:   "MSG-1",
		MediaType:   "image/jpeg",
		Filename:    "photo.jpg",
		FilePath:    "org/file.jpg",
		FileSize:    4567,
		DownloadURL: "https://storage/presigned-photo.jpg",
	})
	if err != nil {
		t.Fatalf("mergeWhatsAppMediaResponseMetadata error: %v", err)
	}

	parsed := parseWhatsAppPortalMetadata(merged)
	if parsed.Attachment == nil {
		t.Fatal("expected attachment metadata to be present")
	}
	if strings.TrimSpace(parsed.Attachment.RemoteURL) != "https://storage/presigned-photo.jpg" {
		t.Fatalf("expected remoteUrl to be replaced with presigned url, got %q", parsed.Attachment.RemoteURL)
	}
	if strings.TrimSpace(parsed.Attachment.Path) != "org/file.jpg" {
		t.Fatalf("expected path to be updated to storage key, got %q", parsed.Attachment.Path)
	}
	if strings.TrimSpace(parsed.Attachment.Filename) != "photo.jpg" {
		t.Fatalf("expected filename to stay photo.jpg, got %q", parsed.Attachment.Filename)
	}
}

func TestWhatsAppMediaDownloadTargetPrefersChatIdentifiersFromMetadata(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"payload":{"chat_id":"` + testWhatsAppChatLID + `","from_lid":"` + testWhatsAppChatLID + `","from":"31686261598@s.whatsapp.net"}}`)

	got := whatsAppMediaDownloadTarget(raw, testWhatsAppMediaPhone)
	if got != testWhatsAppChatLID {
		t.Fatalf("expected LID chat_id to be used as definitive chat identifier, got %q", got)
	}
}

func TestWhatsAppMediaDownloadTargetFallsBackToSenderAndPhone(t *testing.T) {
	t.Parallel()

	withSender := json.RawMessage(`{"payload":{"from":"31686261598@s.whatsapp.net"}}`)
	if got := whatsAppMediaDownloadTarget(withSender, testWhatsAppMediaPhone); got != "31686261598@s.whatsapp.net" {
		t.Fatalf("expected sender jid fallback, got %q", got)
	}

	withoutPayload := json.RawMessage(`{"payload":{}}`)
	if got := whatsAppMediaDownloadTarget(withoutPayload, testWhatsAppMediaPhone); got != testWhatsAppMediaPhone {
		t.Fatalf("expected phone fallback, got %q", got)
	}
}

func TestWhatsAppMediaDownloadTargetDetailsReportsSource(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"payload":{"from_lid":"` + testWhatsAppChatLID + `"}}`)

	target, source := whatsAppMediaDownloadTargetDetails(raw, testWhatsAppMediaPhone)
	if target != testWhatsAppChatLID {
		t.Fatalf("expected from_lid to be used when no phone-based target available, got %q", target)
	}
	if source != "payload.from_lid" {
		t.Fatalf("expected payload.from_lid source, got %q", source)
	}
}

func TestNormalizeWhatsAppCachedContentTypeCanonicalizesApplicationOgg(t *testing.T) {
	t.Parallel()

	got := normalizeWhatsAppCachedContentType("application/ogg", "audio", "voice-note.ogg", "")
	if got != testAudioOggContentType {
		t.Fatalf("expected application/ogg to normalize to %s, got %q", testAudioOggContentType, got)
	}
}

func TestResolveWhatsAppMessageDeviceIDPrefersNonJIDOverride(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"device_id":"org_3578b0f5-727a-46b2-8d1e-d7b9820587de"}`)

	deviceID, source := resolveWhatsAppMessageDeviceID(raw, "org_fallback")
	if deviceID != "org_3578b0f5-727a-46b2-8d1e-d7b9820587de" {
		t.Fatalf("expected metadata device id to be used, got %q", deviceID)
	}
	if source != "message_metadata" {
		t.Fatalf("expected message_metadata source, got %q", source)
	}
}

func TestResolveWhatsAppMessageDeviceIDIgnoresJIDOverride(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"device_id":"31686388589@s.whatsapp.net"}`)

	deviceID, source := resolveWhatsAppMessageDeviceID(raw, "org_fallback")
	if deviceID != "org_fallback" {
		t.Fatalf("expected fallback device id when metadata contains jid, got %q", deviceID)
	}
	if source != "message_metadata_ignored_jid" {
		t.Fatalf("expected message_metadata_ignored_jid source, got %q", source)
	}
}

func TestResolveWhatsAppMediaDownloadTargetUsesMetadataWhenPresent(t *testing.T) {
	t.Parallel()

	s := &Service{}
	message := repository.WhatsAppMessage{
		Metadata: json.RawMessage(`{"payload":{"chat_id":"31686261598@s.whatsapp.net"}}`),
	}
	conversation := repository.WhatsAppConversation{PhoneNumber: testWhatsAppMediaPhone}

	target, source := s.resolveWhatsAppMediaDownloadTarget(context.Background(), uuid.New(), uuid.New(), message, conversation)
	if target != "31686261598@s.whatsapp.net" {
		t.Fatalf("expected metadata chat_id target, got %q", target)
	}
	if source != "payload.chat_id" {
		t.Fatalf("expected payload.chat_id source, got %q", source)
	}
}

func TestResolveWhatsAppMediaDownloadTargetFallsBackToPhoneWithoutRepo(t *testing.T) {
	t.Parallel()

	s := &Service{}
	message := repository.WhatsAppMessage{
		Metadata: nil,
	}
	conversation := repository.WhatsAppConversation{PhoneNumber: testWhatsAppMediaPhone}

	target, source := s.resolveWhatsAppMediaDownloadTarget(context.Background(), uuid.New(), uuid.New(), message, conversation)
	if target != testWhatsAppMediaPhone {
		t.Fatalf("expected conversation phone fallback, got %q", target)
	}
	if source != "conversation_phone" {
		t.Fatalf("expected conversation_phone source, got %q", source)
	}
}

func TestResolveWhatsAppMediaDownloadTargetUsesLIDChatID(t *testing.T) {
	t.Parallel()

	s := &Service{}
	message := repository.WhatsAppMessage{
		Metadata: json.RawMessage(`{"payload":{"chat_id":"` + testWhatsAppChatLID + `","from_lid":"` + testWhatsAppChatLID + `"}}`),
	}
	conversation := repository.WhatsAppConversation{PhoneNumber: testWhatsAppMediaPhone}

	target, source := s.resolveWhatsAppMediaDownloadTarget(context.Background(), uuid.New(), uuid.New(), message, conversation)
	if target != testWhatsAppChatLID {
		t.Fatalf("expected LID chat_id to be used as primary target, got %q", target)
	}
	if source != "payload.chat_id" {
		t.Fatalf("expected payload.chat_id source, got %q", source)
	}
}

func TestIsWhatsAppLID(t *testing.T) {
	t.Parallel()

	if !isWhatsAppLID("212450775417035@lid") {
		t.Fatal("expected @lid suffix to be recognized as LID")
	}
	if isWhatsAppLID("31686261598@s.whatsapp.net") {
		t.Fatal("expected @s.whatsapp.net to not be recognized as LID")
	}
	if isWhatsAppLID("+31686261598") {
		t.Fatal("expected phone number to not be recognized as LID")
	}
	if isWhatsAppLID("") {
		t.Fatal("expected empty string to not be recognized as LID")
	}
}

func TestWhatsAppMetadataLIDExtractsChatID(t *testing.T) {
	t.Parallel()

	got := whatsAppMetadataLID(json.RawMessage(`{"payload":{"chat_id":"212450775417035@lid","from":"31686261598@s.whatsapp.net"}}`))
	if got != testWhatsAppChatLID {
		t.Fatalf("expected LID from chat_id, got %q", got)
	}
}

func TestWhatsAppMetadataLIDExtractsFromLID(t *testing.T) {
	t.Parallel()

	got := whatsAppMetadataLID(json.RawMessage(`{"payload":{"from_lid":"212450775417035@lid","chat_id":"31686261598@s.whatsapp.net"}}`))
	if got != testWhatsAppChatLID {
		t.Fatalf("expected LID from from_lid, got %q", got)
	}
}

func TestWhatsAppMetadataLIDReturnsEmptyWhenNoLID(t *testing.T) {
	t.Parallel()

	got := whatsAppMetadataLID(json.RawMessage(`{"payload":{"from":"31686261598@s.whatsapp.net","chat_id":"31686261598@s.whatsapp.net"}}`))
	if got != "" {
		t.Fatalf("expected empty string when no LID present, got %q", got)
	}
}

func TestWhatsAppMetadataLIDHandlesNilMetadata(t *testing.T) {
	t.Parallel()

	got := whatsAppMetadataLID(nil)
	if got != "" {
		t.Fatalf("expected empty string for nil metadata, got %q", got)
	}
}

func TestWhatsAppMediaDownloadFallbackPhonesIncludesLIDAndConversationPhone(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"payload":{"chat_id":"212450775417035@lid","from":"31686261598@s.whatsapp.net"}}`)
	got := whatsAppMediaDownloadFallbackPhones(raw, "+31686261598", "31686261598@s.whatsapp.net")
	if len(got) != 2 {
		t.Fatalf("expected 2 fallbacks (LID + phone), got %v", got)
	}
	if got[0] != testWhatsAppChatLID {
		t.Fatalf("expected LID as first fallback, got %q", got[0])
	}
	if got[1] != testWhatsAppMediaPhone {
		t.Fatalf("expected conversation phone as second fallback, got %q", got[1])
	}
}

func TestWhatsAppMediaDownloadFallbackPhonesExcludesPrimaryTarget(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"payload":{"chat_id":"212450775417035@lid"}}`)
	got := whatsAppMediaDownloadFallbackPhones(raw, "212450775417035@lid", "212450775417035@lid")
	if len(got) != 0 {
		t.Fatalf("expected no fallbacks when LID and conversation phone match primary target, got %v", got)
	}
}

func TestWhatsAppMediaDownloadFallbackPhonesHandlesNoLID(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"payload":{"from":"31686261598@s.whatsapp.net"}}`)
	got := whatsAppMediaDownloadFallbackPhones(raw, "+31686261598", "31686261598@s.whatsapp.net")
	if len(got) != 1 {
		t.Fatalf("expected 1 fallback (phone only), got %v", got)
	}
	if got[0] != testWhatsAppMediaPhone {
		t.Fatalf("expected conversation phone as fallback, got %q", got[0])
	}
}
