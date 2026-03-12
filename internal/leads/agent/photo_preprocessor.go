package agent

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

const imagePNGMimeType = "image/png"

var focalLengthRegexp = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)`)

type OCRCandidate struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence,omitempty"`
	Source     string  `json:"source,omitempty"`
}

type ImageVariant struct {
	Kind              string   `json:"kind"`
	MIMEType          string   `json:"mimeType"`
	Data              []byte   `json:"-"`
	Width             int      `json:"width"`
	Height            int      `json:"height"`
	AppliedTransforms []string `json:"appliedTransforms,omitempty"`
}

type PreprocessingMetadata struct {
	Filename          string    `json:"filename,omitempty"`
	MIMEType          string    `json:"mimeType,omitempty"`
	Format            string    `json:"format,omitempty"`
	Width             int       `json:"width,omitempty"`
	Height            int       `json:"height,omitempty"`
	CameraMake        string    `json:"cameraMake,omitempty"`
	CameraModel       string    `json:"cameraModel,omitempty"`
	FocalLengthMM     string    `json:"focalLengthMm,omitempty"`
	Orientation       string    `json:"orientation,omitempty"`
	CapturedAt        string    `json:"capturedAt,omitempty"`
	AppliedTransforms []string  `json:"appliedTransforms,omitempty"`
	SkippedTransforms []string  `json:"skippedTransforms,omitempty"`
	PreparedAt        time.Time `json:"preparedAt"`
}

type PreparedImage struct {
	Original      ImageData             `json:"-"`
	Variants      []ImageVariant        `json:"variants,omitempty"`
	Metadata      PreprocessingMetadata `json:"metadata"`
	OCRCandidates []OCRCandidate        `json:"ocrCandidates,omitempty"`
}

type PhotoPreprocessingSettings struct {
	Enabled                              bool     `json:"enabled"`
	OCRAssistEnabled                     bool     `json:"ocrAssistEnabled"`
	OCRAssistServiceTypes                []string `json:"ocrAssistServiceTypes,omitempty"`
	LensCorrectionEnabled                bool     `json:"lensCorrectionEnabled"`
	LensCorrectionServiceTypes           []string `json:"lensCorrectionServiceTypes,omitempty"`
	PerspectiveNormalizationEnabled      bool     `json:"perspectiveNormalizationEnabled"`
	PerspectiveNormalizationServiceTypes []string `json:"perspectiveNormalizationServiceTypes,omitempty"`
}

type ImagePreprocessor interface {
	Prepare(ctx context.Context, settings PhotoPreprocessingSettings, serviceType string, images []ImageData) ([]PreparedImage, error)
}

type BasicImagePreprocessor struct {
	tesseractPath string
}

func NewBasicImagePreprocessor() *BasicImagePreprocessor {
	tesseractPath, _ := exec.LookPath("tesseract")
	return &BasicImagePreprocessor{tesseractPath: tesseractPath}
}

func normalizeServiceTypeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizePhotoPreprocessingSettings(settings PhotoPreprocessingSettings) PhotoPreprocessingSettings {
	settings.OCRAssistServiceTypes = normalizeServiceTypeList(settings.OCRAssistServiceTypes)
	settings.LensCorrectionServiceTypes = normalizeServiceTypeList(settings.LensCorrectionServiceTypes)
	settings.PerspectiveNormalizationServiceTypes = normalizeServiceTypeList(settings.PerspectiveNormalizationServiceTypes)
	return settings
}

func normalizeServiceTypeList(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := normalizeServiceTypeKey(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func (p *BasicImagePreprocessor) Prepare(ctx context.Context, settings PhotoPreprocessingSettings, serviceType string, images []ImageData) ([]PreparedImage, error) {
	settings = normalizePhotoPreprocessingSettings(settings)
	prepared := make([]PreparedImage, 0, len(images))
	for _, img := range images {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !settings.Enabled {
			prepared = append(prepared, PreparedImage{
				Original: img,
				Metadata: PreprocessingMetadata{
					Filename:          img.Filename,
					MIMEType:          img.MIMEType,
					PreparedAt:        time.Now().UTC(),
					SkippedTransforms: []string{"preprocessing disabled by organization setting"},
				},
			})
			continue
		}
		prepared = append(prepared, p.prepareOne(ctx, settings, serviceType, img))
	}
	return prepared, nil
}

func (p *BasicImagePreprocessor) prepareOne(ctx context.Context, settings PhotoPreprocessingSettings, serviceType string, img ImageData) PreparedImage {
	prepared := PreparedImage{
		Original: img,
		Metadata: PreprocessingMetadata{
			Filename:   img.Filename,
			MIMEType:   img.MIMEType,
			PreparedAt: time.Now().UTC(),
		},
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil {
		prepared.Metadata.SkippedTransforms = append(prepared.Metadata.SkippedTransforms, "image decode config unavailable")
	} else {
		prepared.Metadata.Format = format
		prepared.Metadata.Width = config.Width
		prepared.Metadata.Height = config.Height
	}

	p.populateExifMetadata(&prepared)
	p.applyAdvancedPreprocessing(ctx, settings, serviceType, img, &prepared)

	if len(prepared.Metadata.AppliedTransforms) == 0 && len(prepared.Metadata.SkippedTransforms) == 0 {
		prepared.Metadata.SkippedTransforms = append(prepared.Metadata.SkippedTransforms, "no preprocessing transforms applied")
	}

	return prepared
}

func (p *BasicImagePreprocessor) applyAdvancedPreprocessing(ctx context.Context, settings PhotoPreprocessingSettings, serviceType string, img ImageData, prepared *PreparedImage) {
	workingImage, _, decodeErr := image.Decode(bytes.NewReader(img.Data))
	if decodeErr != nil {
		prepared.Metadata.SkippedTransforms = append(prepared.Metadata.SkippedTransforms, "image decode unavailable for advanced preprocessing")
		return
	}

	workingImage = p.applyLensCorrectionIfEligible(settings, serviceType, workingImage, prepared)
	workingImage = p.applyPerspectiveNormalizationIfEligible(settings, serviceType, workingImage, prepared)
	p.addStructuralVariant(workingImage, prepared)
	p.addOCRCandidates(ctx, settings, serviceType, img, prepared)
}

func (p *BasicImagePreprocessor) applyLensCorrectionIfEligible(settings PhotoPreprocessingSettings, serviceType string, source image.Image, prepared *PreparedImage) image.Image {
	variant, corrected, skipReason := p.maybeGenerateLensCorrectionVariant(settings, serviceType, source, prepared.Metadata.FocalLengthMM)
	if variant != nil {
		prepared.Variants = append(prepared.Variants, *variant)
		prepared.Metadata.AppliedTransforms = append(prepared.Metadata.AppliedTransforms, variant.AppliedTransforms...)
		return corrected
	}
	if skipReason != "" {
		prepared.Metadata.SkippedTransforms = append(prepared.Metadata.SkippedTransforms, skipReason)
	}
	return source
}

func (p *BasicImagePreprocessor) applyPerspectiveNormalizationIfEligible(settings PhotoPreprocessingSettings, serviceType string, source image.Image, prepared *PreparedImage) image.Image {
	variant, normalized, skipReason := p.maybeGeneratePerspectiveVariant(settings, serviceType, source)
	if variant != nil {
		prepared.Variants = append(prepared.Variants, *variant)
		prepared.Metadata.AppliedTransforms = append(prepared.Metadata.AppliedTransforms, variant.AppliedTransforms...)
		return normalized
	}
	if skipReason != "" {
		prepared.Metadata.SkippedTransforms = append(prepared.Metadata.SkippedTransforms, skipReason)
	}
	return source
}

func (p *BasicImagePreprocessor) addStructuralVariant(source image.Image, prepared *PreparedImage) {
	if variant, skipReason := p.generateStructuralVariantFromImage(source, prepared.Metadata.Format); variant != nil {
		prepared.Variants = append(prepared.Variants, *variant)
		prepared.Metadata.AppliedTransforms = append(prepared.Metadata.AppliedTransforms, variant.AppliedTransforms...)
	} else if skipReason != "" {
		prepared.Metadata.SkippedTransforms = append(prepared.Metadata.SkippedTransforms, skipReason)
	}
}

func (p *BasicImagePreprocessor) addOCRCandidates(ctx context.Context, settings PhotoPreprocessingSettings, serviceType string, img ImageData, prepared *PreparedImage) {
	if candidates, skipReason := p.maybeExtractOCRCandidates(ctx, settings, serviceType, img, prepared.Variants); len(candidates) > 0 {
		prepared.OCRCandidates = candidates
	} else if skipReason != "" {
		prepared.Metadata.SkippedTransforms = append(prepared.Metadata.SkippedTransforms, skipReason)
	}
}

func (p *BasicImagePreprocessor) populateExifMetadata(prepared *PreparedImage) {
	x, err := exif.Decode(bytes.NewReader(prepared.Original.Data))
	if err != nil {
		prepared.Metadata.SkippedTransforms = append(prepared.Metadata.SkippedTransforms, "exif metadata unavailable")
		return
	}
	if tag, err := x.Get(exif.Make); err == nil {
		if value, valueErr := tag.StringVal(); valueErr == nil {
			prepared.Metadata.CameraMake = strings.TrimSpace(value)
		}
	}
	if tag, err := x.Get(exif.Model); err == nil {
		if value, valueErr := tag.StringVal(); valueErr == nil {
			prepared.Metadata.CameraModel = strings.TrimSpace(value)
		}
	}
	if tag, err := x.Get(exif.FocalLength); err == nil {
		prepared.Metadata.FocalLengthMM = strings.TrimSpace(tag.String())
	}
	if tag, err := x.Get(exif.Orientation); err == nil {
		prepared.Metadata.Orientation = strings.TrimSpace(tag.String())
	}
	if capturedAt, err := x.DateTime(); err == nil {
		prepared.Metadata.CapturedAt = capturedAt.UTC().Format(time.RFC3339)
	}
}

func (p *BasicImagePreprocessor) generateStructuralVariantFromImage(source image.Image, format string) (*ImageVariant, string) {
	bounds := source.Bounds()
	grayImg := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray := color.GrayModel.Convert(source.At(x, y)).(color.Gray)
			gray.Y = applyMildContrast(gray.Y)
			grayImg.SetGray(x, y, gray)
		}
	}

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, grayImg); err != nil {
		return nil, "structural variant skipped: png encode failed"
	}

	return &ImageVariant{
		Kind:     structuralVariantKind(format),
		MIMEType: imagePNGMimeType,
		Data:     buffer.Bytes(),
		Width:    bounds.Dx(),
		Height:   bounds.Dy(),
		AppliedTransforms: []string{
			"grayscale structural variant",
			"mild contrast enhancement",
		},
	}, ""
}

func serviceTypeAllowed(serviceType string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true
	}
	normalizedServiceType := normalizeServiceTypeKey(serviceType)
	for _, allowed := range allowlist {
		if normalizeServiceTypeKey(allowed) == normalizedServiceType {
			return true
		}
	}
	return false
}

func (p *BasicImagePreprocessor) maybeGenerateLensCorrectionVariant(settings PhotoPreprocessingSettings, serviceType string, source image.Image, focalLengthRaw string) (*ImageVariant, image.Image, string) {
	if !settings.LensCorrectionEnabled {
		return nil, nil, ""
	}
	if !serviceTypeAllowed(serviceType, settings.LensCorrectionServiceTypes) {
		return nil, nil, "lens correction skipped: service type not enabled"
	}
	focalLengthMM, ok := parseExifFocalLengthMM(focalLengthRaw)
	if !ok {
		return nil, nil, "lens correction skipped: focal length unavailable"
	}
	corrected := applyBestEffortLensCorrection(source, focalLengthMM)
	variant, skipReason := encodeVariantImage(corrected, "lens_corrected", []string{"best-effort lens correction"})
	if variant == nil {
		return nil, nil, skipReason
	}
	return variant, corrected, ""
}

func (p *BasicImagePreprocessor) maybeGeneratePerspectiveVariant(settings PhotoPreprocessingSettings, serviceType string, source image.Image) (*ImageVariant, image.Image, string) {
	if !settings.PerspectiveNormalizationEnabled {
		return nil, nil, ""
	}
	if !serviceTypeAllowed(serviceType, settings.PerspectiveNormalizationServiceTypes) {
		return nil, nil, "perspective normalization skipped: service type not enabled"
	}
	normalized, ok := applyBestEffortPerspectiveNormalization(source)
	if !ok {
		return nil, nil, "perspective normalization skipped: keystone not detected"
	}
	variant, skipReason := encodeVariantImage(normalized, "perspective_normalized", []string{"best-effort perspective normalization"})
	if variant == nil {
		return nil, nil, skipReason
	}
	return variant, normalized, ""
}

func (p *BasicImagePreprocessor) maybeExtractOCRCandidates(ctx context.Context, settings PhotoPreprocessingSettings, serviceType string, original ImageData, variants []ImageVariant) ([]OCRCandidate, string) {
	if !settings.OCRAssistEnabled {
		return nil, ""
	}
	if !serviceTypeAllowed(serviceType, settings.OCRAssistServiceTypes) {
		return nil, "ocr assist skipped: service type not enabled"
	}
	if strings.TrimSpace(p.tesseractPath) == "" {
		return nil, "ocr assist skipped: tesseract binary not available"
	}

	assets := buildOCRAssets(original, variants)
	for _, asset := range assets {
		lines, err := p.runOCRCommand(ctx, asset.kind, asset.mimeType, asset.data)
		if err != nil {
			continue
		}
		candidates := buildOCRCandidates(lines, asset.kind)
		if len(candidates) > 0 {
			return candidates, ""
		}
	}
	return nil, "ocr assist skipped: no text candidates extracted"
}

type ocrAsset struct {
	kind     string
	mimeType string
	data     []byte
}

func buildOCRAssets(original ImageData, variants []ImageVariant) []ocrAsset {
	assets := make([]ocrAsset, 0, len(variants)+1)
	for _, variant := range variants {
		if strings.Contains(variant.Kind, "structural") {
			assets = append(assets, ocrAsset{kind: variant.Kind, mimeType: variant.MIMEType, data: variant.Data})
		}
	}
	for _, variant := range variants {
		if !strings.Contains(variant.Kind, "structural") {
			assets = append(assets, ocrAsset{kind: variant.Kind, mimeType: variant.MIMEType, data: variant.Data})
		}
	}
	assets = append(assets, ocrAsset{kind: "original", mimeType: original.MIMEType, data: original.Data})
	return assets
}

func (p *BasicImagePreprocessor) runOCRCommand(ctx context.Context, sourceKind string, mimeType string, data []byte) ([]string, error) {
	ext := extensionForMIMEType(mimeType)
	if ext == "" {
		ext = ".img"
	}
	tempFile, err := os.CreateTemp("", "photo-ocr-*"+ext)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.Remove(tempFile.Name())
	}()
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return nil, err
	}
	if err := tempFile.Close(); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, p.tesseractPath, tempFile.Name(), "stdout", "--psm", "11")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ocr command failed for %s: %w", sourceKind, err)
	}
	return strings.Split(string(output), "\n"), nil
}

func buildOCRCandidates(lines []string, source string) []OCRCandidate {
	seen := make(map[string]struct{})
	candidates := make([]OCRCandidate, 0, 8)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 2 {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		candidates = append(candidates, OCRCandidate{Text: trimmed, Source: source})
		if len(candidates) == 8 {
			break
		}
	}
	return candidates
}

func extensionForMIMEType(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case imagePNGMimeType:
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return filepath.Ext(mimeType)
	}
}

func parseExifFocalLengthMM(raw string) (float64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	if strings.Contains(trimmed, "/") {
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) == 2 {
			numerator, errNum := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			denominator, errDen := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if errNum == nil && errDen == nil && denominator != 0 {
				return numerator / denominator, true
			}
		}
	}
	match := focalLengthRegexp.FindStringSubmatch(trimmed)
	if len(match) < 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func applyBestEffortLensCorrection(source image.Image, focalLengthMM float64) image.Image {
	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	output := image.NewRGBA(bounds)
	cx := float64(bounds.Min.X) + float64(width-1)/2
	cy := float64(bounds.Min.Y) + float64(height-1)/2
	maxRadius := math.Hypot(float64(width)/2, float64(height)/2)
	coefficient := lensCorrectionCoefficient(focalLengthMM)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			radius := math.Hypot(dx, dy)
			normalizedRadius := radius / maxRadius
			sourceRadius := normalizedRadius * (1 + coefficient*normalizedRadius*normalizedRadius)
			scale := 1.0
			if normalizedRadius > 0 {
				scale = sourceRadius / normalizedRadius
			}
			sx := cx + dx*scale
			sy := cy + dy*scale
			output.Set(x, y, sampleNearest(source, sx, sy))
		}
	}
	return output
}

func lensCorrectionCoefficient(focalLengthMM float64) float64 {
	switch {
	case focalLengthMM <= 0:
		return -0.08
	case focalLengthMM < 3.0:
		return -0.22
	case focalLengthMM < 4.5:
		return -0.16
	case focalLengthMM < 6.0:
		return -0.10
	default:
		return -0.06
	}
}

func applyBestEffortPerspectiveNormalization(source image.Image) (image.Image, bool) {
	gray := image.NewGray(source.Bounds())
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			gray.Set(x, y, color.GrayModel.Convert(source.At(x, y)))
		}
	}

	topLeft, topRight, topOK := estimateBandBounds(gray, 0.12, 0.35)
	bottomLeft, bottomRight, bottomOK := estimateBandBounds(gray, 0.65, 0.9)
	if !topOK || !bottomOK {
		return nil, false
	}
	topWidth := topRight - topLeft
	bottomWidth := bottomRight - bottomLeft
	if topWidth <= 0 || bottomWidth <= 0 {
		return nil, false
	}
	if math.Abs(float64(topWidth-bottomWidth)) < float64(gray.Bounds().Dx())*0.08 {
		return nil, false
	}

	bounds := source.Bounds()
	output := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		progress := float64(y-bounds.Min.Y) / math.Max(1, float64(bounds.Dy()-1))
		left := interpolate(topLeft, bottomLeft, progress)
		right := interpolate(topRight, bottomRight, progress)
		if right-left < 8 {
			continue
		}
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			sx := left + (right-left)*float64(x-bounds.Min.X)/math.Max(1, float64(bounds.Dx()-1))
			output.Set(x, y, sampleNearest(source, sx, float64(y)))
		}
	}
	return output, true
}

func estimateBandBounds(gray *image.Gray, startRatio float64, endRatio float64) (float64, float64, bool) {
	bounds := gray.Bounds()
	startY := bounds.Min.Y + int(float64(bounds.Dy())*startRatio)
	endY := bounds.Min.Y + int(float64(bounds.Dy())*endRatio)
	if endY <= startY {
		return 0, 0, false
	}
	leftTotal := 0.0
	rightTotal := 0.0
	count := 0.0
	for y := startY; y < endY; y++ {
		left, right, ok := detectActiveRowBounds(gray, y)
		if !ok {
			continue
		}
		leftTotal += left
		rightTotal += right
		count++
	}
	if count == 0 {
		return 0, 0, false
	}
	return leftTotal / count, rightTotal / count, true
}

func detectActiveRowBounds(gray *image.Gray, y int) (float64, float64, bool) {
	bounds := gray.Bounds()
	left := -1
	right := -1
	for x := bounds.Min.X + 1; x < bounds.Max.X; x++ {
		prev := gray.GrayAt(x-1, y).Y
		curr := gray.GrayAt(x, y).Y
		if absInt(int(curr)-int(prev)) >= 20 {
			left = x
			break
		}
	}
	for x := bounds.Max.X - 1; x > bounds.Min.X; x-- {
		next := gray.GrayAt(x, y).Y
		prev := gray.GrayAt(x-1, y).Y
		if absInt(int(next)-int(prev)) >= 20 {
			right = x
			break
		}
	}
	if left < 0 || right <= left {
		return 0, 0, false
	}
	return float64(left), float64(right), true
}

func encodeVariantImage(img image.Image, kind string, transforms []string) (*ImageVariant, string) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return nil, fmt.Sprintf("%s skipped: png encode failed", kind)
	}
	bounds := img.Bounds()
	return &ImageVariant{
		Kind:              kind,
		MIMEType:          imagePNGMimeType,
		Data:              buffer.Bytes(),
		Width:             bounds.Dx(),
		Height:            bounds.Dy(),
		AppliedTransforms: transforms,
	}, ""
}

func sampleNearest(source image.Image, x float64, y float64) color.Color {
	bounds := source.Bounds()
	sx := int(math.Round(x))
	sy := int(math.Round(y))
	if sx < bounds.Min.X {
		sx = bounds.Min.X
	}
	if sx >= bounds.Max.X {
		sx = bounds.Max.X - 1
	}
	if sy < bounds.Min.Y {
		sy = bounds.Min.Y
	}
	if sy >= bounds.Max.Y {
		sy = bounds.Max.Y - 1
	}
	return source.At(sx, sy)
}

func interpolate(start float64, end float64, progress float64) float64 {
	return start + (end-start)*progress
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func structuralVariantKind(format string) string {
	if strings.TrimSpace(format) == "" {
		return "structural"
	}
	return fmt.Sprintf("structural_%s", format)
}

func applyMildContrast(value uint8) uint8 {
	adjusted := (float64(value)-128.0)*1.18 + 128.0
	if adjusted < 0 {
		return 0
	}
	if adjusted > 255 {
		return 255
	}
	return uint8(adjusted)
}

func init() {
	image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("jpg", "jpg", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "gif", gif.Decode, gif.DecodeConfig)
}
