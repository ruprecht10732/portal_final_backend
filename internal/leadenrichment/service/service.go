// Package service provides lead enrichment logic with caching.
package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"portal_final_backend/internal/leadenrichment/client"
	"portal_final_backend/platform/logger"
)

const (
	cacheTTL = 365 * 24 * time.Hour
)

// EnrichmentData contains lead enrichment values from PDOK CBS APIs.
// Data is sourced from PC4 (most complete), PC6, and Buurt levels.
type EnrichmentData struct {
	Source    string
	Postcode6 string
	Postcode4 string
	Buurtcode string
	DataYear  *int // Year of PC4/PC6 data (e.g., 2022, 2023, 2024)

	// Energy - from PC4
	GemAardgasverbruik        *float64
	GemElektriciteitsverbruik *float64

	// Housing - from PC4/PC6
	HuishoudenGrootte    *float64
	KoopwoningenPct      *float64
	BouwjaarVanaf2000Pct *float64
	WOZWaarde            *float64 // PC4: gemiddelde_woz_waarde_woning

	// Income - from PC4
	MediaanVermogenX1000     *float64
	GemInkomenHuishouden     *float64 // PC4: gemiddeld_inkomen_huishouden (in 1000s)
	PctHoogInkomen           *float64 // PC4: percentage_hoog_inkomen_huishouden
	PctLaagInkomen           *float64 // PC4: percentage_laag_inkomen_huishouden
	MediaanInkomenHuishouden *float64 // PC6: mediaan_inkomen_huishouden (legacy)

	// Demographics
	HuishoudensMetKinderenPct *float64
	AantalHuishoudens         *int
	Stedelijkheid             *int // PC4: 1=zeer sterk, 5=niet stedelijk

	Confidence *float64
	FetchedAt  time.Time
}

type cacheEntry struct {
	data      *EnrichmentData
	expiresAt time.Time
}

// Service handles lead enrichment lookups with caching and fallback.
type Service struct {
	client   *client.Client
	log      *logger.Logger
	cache    map[string]cacheEntry
	cacheMu  sync.RWMutex
	cacheTTL time.Duration
}

// New creates a new lead enrichment service.
func New(client *client.Client, log *logger.Logger) *Service {
	return &Service{
		client:   client,
		log:      log,
		cache:    make(map[string]cacheEntry),
		cacheTTL: cacheTTL,
	}
}

// GetByPostcode fetches enrichment data for a postcode.
// Fetches PC4 first (richest data), then PC6, then buurt fallback.
// Returns nil if lookup fails or data is not available.
func (s *Service) GetByPostcode(ctx context.Context, postcode string) (*EnrichmentData, error) {
	normalized := normalizePostcode(postcode)
	if normalized == "" {
		return nil, nil
	}

	if cached := s.getFromCache(normalized); cached != nil {
		return cached, nil
	}

	now := time.Now().UTC()
	result := &EnrichmentData{
		Source:    "pdok",
		Postcode6: normalized,
		FetchedAt: now,
	}

	// Extract PC4 from PC6 (first 4 characters)
	if len(normalized) >= 4 {
		pc4 := normalized[:4]
		result.Postcode4 = pc4
		s.enrichFromPC4(ctx, pc4, result)
	}

	// Enrich with PC6 data
	s.enrichFromPC6(ctx, normalized, result)

	// Fall back to buurt for any still-missing fields
	s.enrichFromBuurt(ctx, normalized, result)

	// Calculate confidence based on data sources
	result.Confidence = s.calculateConfidence(result)

	s.setCache(normalized, result)
	return result, nil
}

// enrichFromPC4 fetches PC4-level data (most complete: gas, electricity, income, WOZ).
func (s *Service) enrichFromPC4(ctx context.Context, pc4 string, result *EnrichmentData) {
	pc4Data, err := s.client.GetPC4(ctx, pc4)
	if err != nil {
		s.log.Debug("pc4 lookup failed", "pc4", pc4, "error", err)
		return
	}
	if pc4Data == nil {
		return
	}

	props := pc4Data.Properties
	result.Source = "pdok_pc4"
	result.DataYear = &pc4Data.Year

	// Energy - PC4 has the best data
	result.GemAardgasverbruik = props.GemiddeldGasverbruikWoning.ToFloat64Ptr()
	result.GemElektriciteitsverbruik = props.GemiddeldElektriciteitsverbruik.ToFloat64Ptr()

	// Housing
	result.HuishoudenGrootte = props.GemiddeldHuishoudensgrootte.ToFloat64Ptr()
	result.KoopwoningenPct = props.KoopwoningenPct.ToFloat64Ptr()
	result.WOZWaarde = props.GemiddeldWOZWaarde.ToFloat64Ptr()
	result.BouwjaarVanaf2000Pct = props.BouwjaarVanaf2000Pct()

	// Income - PC4 has gemiddeld (average), convert to float
	result.GemInkomenHuishouden = props.GemiddeldInkomen.ToFloat64Ptr()
	result.PctHoogInkomen = props.PctHoogInkomen.ToFloat64Ptr()
	result.PctLaagInkomen = props.PctLaagInkomen.ToFloat64Ptr()

	// Demographics
	result.HuishoudensMetKinderenPct = props.HuishoudensMetKinderenPct()
	result.AantalHuishoudens = props.AantalHuishoudens.ToIntPtr()
	if s := props.Stedelijkheid.ToIntPtr(); s != nil {
		result.Stedelijkheid = s
	}
}

// enrichFromPC6 fills in any missing fields from PC6-level data.
func (s *Service) enrichFromPC6(ctx context.Context, postcode string, result *EnrichmentData) {
	pc6Data, _, err := s.client.GetPC6(ctx, postcode)
	if err != nil {
		s.log.Debug("pc6 lookup failed", "postcode", postcode, "error", err)
		return
	}
	if pc6Data == nil {
		return
	}

	fillMissingPC6Fields(result, pc6Data)
	fillPC6BouwjaarPct(result, pc6Data)
	result.Source = appendSource(result.Source, "pc6", "pdok_pc6")
}

func fillMissingPC6Fields(result *EnrichmentData, pc6Data *client.PC6Properties) {
	if result.HuishoudenGrootte == nil {
		result.HuishoudenGrootte = pc6Data.GemiddeldHuishoudensgrootte.ToFloat64Ptr()
	}
	if result.KoopwoningenPct == nil {
		result.KoopwoningenPct = pc6Data.KoopwoningenPct.ToFloat64Ptr()
	}
	if result.HuishoudensMetKinderenPct == nil {
		result.HuishoudensMetKinderenPct = pc6Data.HuishoudensMetKinderenPct.ToFloat64Ptr()
	}
	if result.MediaanInkomenHuishouden == nil {
		result.MediaanInkomenHuishouden = pc6Data.MediaanInkomenHuishouden.ToFloat64Ptr()
	}
	if result.AantalHuishoudens == nil {
		result.AantalHuishoudens = pc6Data.AantalHuishoudens.ToIntPtr()
	}
}

func fillPC6BouwjaarPct(result *EnrichmentData, pc6Data *client.PC6Properties) {
	if result.BouwjaarVanaf2000Pct != nil {
		return
	}
	totalPtr := pc6Data.AantalWoningen.ToFloat64Ptr()
	bouwjaar05Tot15Ptr := pc6Data.WoningenBouwjaar05Tot15.ToFloat64Ptr()
	bouwjaar15EnLaterPtr := pc6Data.WoningenBouwjaar15EnLater.ToFloat64Ptr()
	if totalPtr == nil || bouwjaar05Tot15Ptr == nil || bouwjaar15EnLaterPtr == nil {
		return
	}
	total := *totalPtr
	if total <= 0 {
		return
	}
	recent := *bouwjaar05Tot15Ptr + *bouwjaar15EnLaterPtr
	pct := (recent / total) * 100
	result.BouwjaarVanaf2000Pct = &pct
}

// enrichFromBuurt fills in any missing fields from buurt-level statistics.
func (s *Service) enrichFromBuurt(ctx context.Context, postcode string, result *EnrichmentData) {
	buurtcode := s.getBuurtcode(ctx, postcode)
	if buurtcode == "" {
		return
	}

	result.Buurtcode = buurtcode

	s.applyCBSBuurtData(ctx, buurtcode, result)

	buurtData := s.getBuurtData(ctx, buurtcode)
	if buurtData == nil {
		return
	}

	fillMissingBuurtFields(result, buurtData)
	result.Source = appendSource(result.Source, "buurt", "pdok_buurt")
}

func (s *Service) getBuurtcode(ctx context.Context, postcode string) string {
	buurtcode, err := s.client.GetBuurtcode(ctx, postcode)
	if err != nil {
		s.log.Debug("buurtcode lookup failed", "postcode", postcode, "error", err)
		return ""
	}
	return buurtcode
}

func (s *Service) applyCBSBuurtData(ctx context.Context, buurtcode string, result *EnrichmentData) {
	cbsData, err := s.client.GetCBSBuurtData(ctx, buurtcode)
	if err != nil {
		s.log.Debug("cbs odata lookup failed", "buurtcode", buurtcode, "error", err)
		return
	}
	if cbsData == nil || cbsData.MediaanVermogen == nil {
		return
	}
	result.MediaanVermogenX1000 = cbsData.MediaanVermogen
}

func (s *Service) getBuurtData(ctx context.Context, buurtcode string) *client.BuurtProperties {
	buurtData, _, err := s.client.GetBuurt(ctx, buurtcode)
	if err != nil {
		s.log.Debug("buurt lookup failed", "buurtcode", buurtcode, "error", err)
		return nil
	}
	return buurtData
}

func fillMissingBuurtFields(result *EnrichmentData, buurtData *client.BuurtProperties) {
	if result.KoopwoningenPct == nil {
		result.KoopwoningenPct = buurtData.KoopwoningenPct.ToFloat64Ptr()
	}
	if result.HuishoudensMetKinderenPct == nil {
		result.HuishoudensMetKinderenPct = buurtData.HuishoudensMetKinderenPct.ToFloat64Ptr()
	}
	if result.BouwjaarVanaf2000Pct == nil {
		result.BouwjaarVanaf2000Pct = buurtData.BouwjaarVanaf2000Pct.ToFloat64Ptr()
	}
	if result.GemAardgasverbruik == nil {
		result.GemAardgasverbruik = buurtData.GemiddeldGasverbruik.ToFloat64Ptr()
	}
	if result.HuishoudenGrootte == nil {
		result.HuishoudenGrootte = buurtData.GemHuishoudensgrootte.ToFloat64Ptr()
	}
	if result.AantalHuishoudens == nil {
		result.AantalHuishoudens = buurtData.AantalHuishoudens.ToIntPtr()
	}
}

func appendSource(current, suffix, replacement string) string {
	if current == "pdok" {
		return replacement
	}
	return current + "+" + suffix
}

// calculateConfidence returns a confidence score based on data completeness.
func (s *Service) calculateConfidence(result *EnrichmentData) *float64 {
	if result == nil {
		return nil
	}

	// Start with base confidence
	confidence := 1.0

	// Reduce confidence if using older data
	if result.DataYear != nil {
		switch *result.DataYear {
		case 2024:
			confidence *= 1.0
		case 2023:
			confidence *= 0.95
		case 2022:
			confidence *= 0.90
		default:
			confidence *= 0.85
		}
	}

	// Reduce confidence for buurt-level data (less precise)
	if strings.Contains(result.Source, "buurt") {
		confidence *= 0.95
	}

	// Reduce confidence for missing key fields
	missingCount := 0
	if result.KoopwoningenPct == nil {
		missingCount++
	}
	if result.GemInkomenHuishouden == nil && result.MediaanInkomenHuishouden == nil {
		missingCount++
	}
	if result.GemAardgasverbruik == nil {
		missingCount++
	}
	if result.WOZWaarde == nil {
		missingCount++
	}

	// Each missing key field reduces confidence by 5%
	confidence *= (1.0 - float64(missingCount)*0.05)

	return &confidence
}

func (s *Service) getFromCache(key string) *EnrichmentData {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	entry, ok := s.cache[key]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.data
}

func (s *Service) setCache(key string, data *EnrichmentData) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.cache[key] = cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(s.cacheTTL),
	}
}

func normalizePostcode(value string) string {
	cleaned := strings.ToUpper(strings.ReplaceAll(value, " ", ""))
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	return strings.TrimSpace(cleaned)
}

func toPtr(value float64) *float64 {
	return &value
}
