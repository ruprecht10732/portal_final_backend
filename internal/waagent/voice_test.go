package waagent

import (
	"encoding/json"
	"testing"
)

func TestIsAudioInboundMessageDetectsPortalAudio(t *testing.T) {
	t.Parallel()

	metadata := []byte(`{"portal":{"messageType":"audio","attachment":{"mediaType":"audio","filename":"note.ogg"}}}`)
	if !isAudioInboundMessage(CurrentInboundMessage{Metadata: metadata}) {
		t.Fatal("expected audio metadata to be detected as audio inbound message")
	}
}

func TestMergeVoiceTranscriptionMetadataAddsTranscriptionDetails(t *testing.T) {
	t.Parallel()

	confidence := 0.91
	raw := []byte(`{"portal":{"messageType":"audio"}}`)
	merged := mergeVoiceTranscriptionMetadata(raw, voiceTranscriptionUpdate{
		Status:        "completed",
		Provider:      "openai-compatible",
		StorageBucket: "lead-service-attachments",
		StorageKey:    "org/waagent/file.ogg",
		Language:      "nl",
		Transcript:    "Kunt u mijn afspraak verzetten?",
		Confidence:    &confidence,
	})

	var payload map[string]any
	if err := json.Unmarshal(merged, &payload); err != nil {
		t.Fatalf("unmarshal merged metadata: %v", err)
	}
	portal, _ := payload["portal"].(map[string]any)
	if portal == nil {
		t.Fatal("expected portal metadata to exist")
	}
	transcription, _ := portal["transcription"].(map[string]any)
	if transcription == nil {
		t.Fatal("expected transcription metadata to exist")
	}
	if got := transcription["status"]; got != "completed" {
		t.Fatalf("expected completed status, got %#v", got)
	}
	if got := transcription["provider"]; got != "openai-compatible" {
		t.Fatalf("expected provider to be stored, got %#v", got)
	}
	if got := transcription["text"]; got != "Kunt u mijn afspraak verzetten?" {
		t.Fatalf("expected transcript text to be stored, got %#v", got)
	}
}
