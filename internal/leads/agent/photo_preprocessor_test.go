package agent

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const testServiceTypeKozijn = "Kozijn vervangen"

var defaultPhotoPreprocessingSettings = PhotoPreprocessingSettings{
	Enabled:                         true,
	OCRAssistEnabled:                false,
	LensCorrectionEnabled:           false,
	PerspectiveNormalizationEnabled: false,
}

func TestBasicImagePreprocessorPrepareAddsStructuralVariant(t *testing.T) {
	preprocessor := NewBasicImagePreprocessor()

	prepared, err := preprocessor.Prepare(context.Background(), defaultPhotoPreprocessingSettings, testServiceTypeKozijn, []ImageData{{
		MIMEType: "image/png",
		Filename: "sample.png",
		Data:     mustEncodePNG(t),
	}})
	if err != nil {
		t.Fatalf("expected preprocessing to succeed: %v", err)
	}
	if len(prepared) != 1 {
		t.Fatalf("expected one prepared image, got %d", len(prepared))
	}
	if prepared[0].Metadata.Width != 2 || prepared[0].Metadata.Height != 2 {
		t.Fatalf("expected extracted dimensions, got %dx%d", prepared[0].Metadata.Width, prepared[0].Metadata.Height)
	}
	if len(prepared[0].Variants) != 1 {
		t.Fatalf("expected one structural variant, got %d", len(prepared[0].Variants))
	}
	if !strings.Contains(strings.Join(prepared[0].Metadata.AppliedTransforms, ", "), "grayscale structural variant") {
		t.Fatalf("expected structural transform metadata, got %v", prepared[0].Metadata.AppliedTransforms)
	}
}

func TestBuildPhotoAnalysisPromptIncludesOCRAssistCandidates(t *testing.T) {
	prompt := buildPhotoAnalysisPrompt(
		mustUUID(t),
		mustUUID(t),
		1,
		"",
		testServiceTypeKozijn,
		"Breedte opening vereist",
		[]PreparedImage{{
			Metadata: PreprocessingMetadata{
				Filename: "door.jpg",
				Width:    1200,
				Height:   900,
			},
			OCRCandidates: []OCRCandidate{{Text: "VELUX GGL MK04", Source: "structural_png"}},
		}},
	)

	checks := []string{
		"OCR assist candidate: VELUX GGL MK04 [source=structural_png]",
		"Gebruik eventuele OCR assist candidates uit preprocessing als machine-read startpunt",
	}
	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf("expected OCR-aware prompt to contain %q", token)
		}
	}
}

func TestBasicImagePreprocessorServiceTypeGatesTransforms(t *testing.T) {
	settings := PhotoPreprocessingSettings{
		LensCorrectionEnabled:                true,
		LensCorrectionServiceTypes:           []string{"Dakinspectie"},
		PerspectiveNormalizationEnabled:      true,
		PerspectiveNormalizationServiceTypes: []string{"Traprenovatie"},
	}

	if settings.LensCorrectionEnabled && serviceTypeAllowed(testServiceTypeKozijn, settings.LensCorrectionServiceTypes) {
		t.Fatalf("expected lens correction to be disabled for non-allowed service type")
	}
	if settings.PerspectiveNormalizationEnabled && serviceTypeAllowed(testServiceTypeKozijn, settings.PerspectiveNormalizationServiceTypes) {
		t.Fatalf("expected perspective normalization to be disabled for non-allowed service type")
	}
	if !settings.LensCorrectionEnabled || !serviceTypeAllowed("Dakinspectie", settings.LensCorrectionServiceTypes) {
		t.Fatalf("expected lens correction to be enabled for allowed service type")
	}
	if !settings.PerspectiveNormalizationEnabled || !serviceTypeAllowed("Traprenovatie", settings.PerspectiveNormalizationServiceTypes) {
		t.Fatalf("expected perspective normalization to be enabled for allowed service type")
	}
}

func TestBuildPhotoAnalysisPromptIncludesPreprocessingContext(t *testing.T) {
	prompt := buildPhotoAnalysisPrompt(
		mustUUID(t),
		mustUUID(t),
		1,
		"",
		testServiceTypeKozijn,
		"Breedte opening vereist",
		[]PreparedImage{{
			Metadata: PreprocessingMetadata{
				Filename:          "door.jpg",
				Width:             1200,
				Height:            900,
				AppliedTransforms: []string{"grayscale structural variant"},
				SkippedTransforms: []string{"exif metadata unavailable"},
			},
		}},
	)

	checks := []string{
		"## PREPROCESSING CONTEXT",
		"Foto 1 (door.jpg): 1200x900",
		"transforms=grayscale structural variant",
		"skipped=exif metadata unavailable",
	}
	for _, token := range checks {
		if !strings.Contains(prompt, token) {
			t.Fatalf("expected preprocessing-aware prompt to contain %q", token)
		}
	}
}

func mustEncodePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	img.Set(1, 0, color.RGBA{R: 120, G: 130, B: 140, A: 255})
	img.Set(0, 1, color.RGBA{R: 220, G: 180, B: 80, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("expected png encoding to succeed: %v", err)
	}
	return buffer.Bytes()
}

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	return uuid.New()
}
