package engine

import (
	"encoding/json"
	"fmt"
	"mime"
	"path"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const voiceMessagePlaceholder = "[Spraakbericht]"
const voiceDefaultContentType = "audio/ogg"

var voicePathSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type portalMetadata struct {
	MessageType string `json:"messageType,omitempty"`
	Caption     string `json:"caption,omitempty"`
	Text        string `json:"text,omitempty"`
	Attachment  *struct {
		MediaType string `json:"mediaType,omitempty"`
		Filename  string `json:"filename,omitempty"`
		Path      string `json:"path,omitempty"`
		RemoteURL string `json:"remoteUrl,omitempty"`
	} `json:"attachment,omitempty"`
}

type voiceTranscriptionUpdate struct {
	Status        string
	Provider      string
	StorageBucket string
	StorageKey    string
	Language      string
	Transcript    string
	ErrorMessage  string
	Confidence    *float64
}

func isAudioInboundMessage(inbound CurrentInboundMessage) bool {
	metadata := parsePortalMetadata(inbound.Metadata)
	if strings.EqualFold(strings.TrimSpace(metadata.MessageType), "audio") {
		return true
	}
	return metadata.Attachment != nil && strings.EqualFold(strings.TrimSpace(metadata.Attachment.MediaType), "audio")
}

func parsePortalMetadata(raw []byte) portalMetadata {
	if len(raw) == 0 {
		return portalMetadata{}
	}
	var envelope struct {
		Portal portalMetadata `json:"portal"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil {
		if strings.TrimSpace(envelope.Portal.MessageType) != "" || envelope.Portal.Attachment != nil {
			return envelope.Portal
		}
	}
	var metadata portalMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return portalMetadata{}
	}
	return metadata
}

func mergeVoiceTranscriptionMetadata(raw []byte, update voiceTranscriptionUpdate) []byte {
	envelope := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &envelope)
	}
	portal, _ := envelope["portal"].(map[string]any)
	if portal == nil {
		portal = map[string]any{}
	}
	if strings.TrimSpace(stringValue(portal["messageType"])) == "" {
		portal["messageType"] = "audio"
	}
	transcription := map[string]any{
		"status": strings.TrimSpace(update.Status),
	}
	if strings.TrimSpace(update.Provider) != "" {
		transcription["provider"] = strings.TrimSpace(update.Provider)
	}
	if strings.TrimSpace(update.StorageBucket) != "" {
		transcription["storageBucket"] = strings.TrimSpace(update.StorageBucket)
	}
	if strings.TrimSpace(update.StorageKey) != "" {
		transcription["storageKey"] = strings.TrimSpace(update.StorageKey)
	}
	if strings.TrimSpace(update.Language) != "" {
		transcription["language"] = strings.TrimSpace(update.Language)
	}
	if strings.TrimSpace(update.Transcript) != "" {
		transcription["text"] = strings.TrimSpace(update.Transcript)
	}
	if strings.TrimSpace(update.ErrorMessage) != "" {
		transcription["error"] = strings.TrimSpace(update.ErrorMessage)
	}
	if update.Confidence != nil {
		transcription["confidence"] = *update.Confidence
	}
	portal["transcription"] = transcription
	envelope["portal"] = portal
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return raw
	}
	return encoded
}

func voiceStorageFolder(orgID uuid.UUID, phoneNumber string, externalMessageID string) string {
	return path.Join(orgID.String(), "whatsappagent", sanitizeVoicePathSegment(phoneNumber), sanitizeVoicePathSegment(externalMessageID))
}

func sanitizeVoicePathSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return voicePathSanitizer.ReplaceAllString(trimmed, "_")
}

func chooseVoiceFileName(downloadFilename string, contentType string) string {
	if trimmed := strings.TrimSpace(downloadFilename); trimmed != "" {
		base := path.Base(trimmed)
		sanitized := voicePathSanitizer.ReplaceAllString(base, "_")
		if sanitized != "" && sanitized != "." && sanitized != ".." {
			return sanitized
		}
	}
	ext := ".ogg"
	if extensions, err := mime.ExtensionsByType(strings.TrimSpace(contentType)); err == nil && len(extensions) > 0 && strings.TrimSpace(extensions[0]) != "" {
		ext = extensions[0]
	}
	return fmt.Sprintf("voice-message%s", ext)
}

func normalizeVoiceImportContentType(contentType string, mediaType string) string {
	trimmed := strings.TrimSpace(strings.Split(contentType, ";")[0])
	isAudio := strings.EqualFold(strings.TrimSpace(mediaType), "audio")
	if trimmed == "" {
		if isAudio {
			return voiceDefaultContentType
		}
		return "application/octet-stream"
	}
	// GoWA returns "application/ogg" for WhatsApp voice notes (OGG Opus).
	// Remap to the correct audio MIME type when the media type confirms audio.
	if isAudio && !strings.HasPrefix(strings.ToLower(trimmed), "audio/") {
		switch strings.ToLower(trimmed) {
		case "application/ogg":
			return voiceDefaultContentType
		case "application/opus":
			return "audio/opus"
		default:
			return voiceDefaultContentType
		}
	}
	return trimmed
}

func formatVoiceTranscriptUserMessage(transcript string) string {
	trimmed := strings.TrimSpace(transcript)
	if trimmed == "" {
		return voiceMessagePlaceholder
	}
	return "Spraakbericht: " + trimmed
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}
