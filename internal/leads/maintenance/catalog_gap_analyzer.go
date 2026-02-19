package maintenance

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	catalogrepo "portal_final_backend/internal/catalog/repository"
	leadsrepo "portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/logger"

	"github.com/google/uuid"
)

type CatalogGapAnalyzer struct {
	leads   *leadsrepo.Repository
	catalog catalogrepo.Repository
	log     *logger.Logger
}

type CatalogGapRunResult struct {
	OrganizationID uuid.UUID
	Candidates     int
	CreatedDrafts  int
	SkippedExists  int
}

func NewCatalogGapAnalyzer(leads *leadsrepo.Repository, catalog catalogrepo.Repository, log *logger.Logger) *CatalogGapAnalyzer {
	return &CatalogGapAnalyzer{leads: leads, catalog: catalog, log: log}
}

type gapCandidate struct {
	Text   string
	Count  int
	Source string
}

type groupedCandidate struct {
	Key            string
	Title          string
	TotalCount     int
	Sources        map[string]int
	Representative string
}

var whitespaceRe = regexp.MustCompile(`\s+`)
var nonWordRe = regexp.MustCompile(`[^a-z0-9\s\-]+`)

func normalizeGapText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = whitespaceRe.ReplaceAllString(s, " ")
	s = nonWordRe.ReplaceAllString(s, " ")
	s = whitespaceRe.ReplaceAllString(strings.TrimSpace(s), " ")
	return s
}

func titleFromText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Keep original casing as much as possible; just ensure it starts with a letter/number.
	return strings.ToUpper(s[:1]) + s[1:]
}

func (a *CatalogGapAnalyzer) gatherCandidates(ctx context.Context, organizationID uuid.UUID, lookbackDays, threshold int) ([]*groupedCandidate, error) {
	candidates, err := a.fetchRawCandidates(ctx, organizationID, lookbackDays, threshold)
	if err != nil {
		return nil, err
	}

	groups := a.groupCandidates(candidates)
	return a.filterAndSortGroups(groups, threshold), nil
}

func (a *CatalogGapAnalyzer) fetchRawCandidates(ctx context.Context, organizationID uuid.UUID, lookbackDays, threshold int) ([]gapCandidate, error) {
	misses, err := a.leads.ListFrequentCatalogSearchMisses(ctx, organizationID, lookbackDays, threshold, 50)
	if err != nil {
		return nil, err
	}
	adHoc, err := a.leads.ListFrequentAdHocQuoteItems(ctx, organizationID, lookbackDays, threshold, 50)
	if err != nil {
		return nil, err
	}

	candidates := make([]gapCandidate, 0, len(misses)+len(adHoc))
	for _, m := range misses {
		candidates = append(candidates, gapCandidate{Text: m.Query, Count: m.SearchCount, Source: "search_miss"})
	}
	for _, it := range adHoc {
		candidates = append(candidates, gapCandidate{Text: it.Description, Count: it.UseCount, Source: "ad_hoc_quote"})
	}
	return candidates, nil
}

func (a *CatalogGapAnalyzer) groupCandidates(candidates []gapCandidate) map[string]*groupedCandidate {
	groups := make(map[string]*groupedCandidate)
	for _, c := range candidates {
		key := normalizeGapText(c.Text)
		if key == "" {
			continue
		}
		g, ok := groups[key]
		if !ok {
			g = &groupedCandidate{Key: key, Sources: make(map[string]int)}
			g.Representative = strings.TrimSpace(c.Text)
			groups[key] = g
		}
		g.TotalCount += c.Count
		g.Sources[c.Source] += c.Count
		// Prefer the longest representative (usually more descriptive) when counts tie.
		if len(strings.TrimSpace(c.Text)) > len(g.Representative) {
			g.Representative = strings.TrimSpace(c.Text)
		}
	}
	return groups
}

func (a *CatalogGapAnalyzer) filterAndSortGroups(groups map[string]*groupedCandidate, threshold int) []*groupedCandidate {
	ordered := make([]*groupedCandidate, 0, len(groups))
	for _, g := range groups {
		if g.TotalCount < threshold {
			continue
		}
		g.Title = titleFromText(g.Representative)
		if g.Title == "" {
			continue
		}
		ordered = append(ordered, g)
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].TotalCount == ordered[j].TotalCount {
			return ordered[i].Title < ordered[j].Title
		}
		return ordered[i].TotalCount > ordered[j].TotalCount
	})
	return ordered
}

func (a *CatalogGapAnalyzer) createDrafts(ctx context.Context, organizationID uuid.UUID, ordered []*groupedCandidate, maxDrafts int, res *CatalogGapRunResult) error {
	vatRateID, err := a.pickDefaultVatRate(ctx, organizationID)
	if err != nil {
		return err
	}

	created := 0
	for _, g := range ordered {
		if created >= maxDrafts {
			break
		}

		exists, err := a.productAlreadyExists(ctx, organizationID, g.Title)
		if err != nil {
			return err
		}
		if exists {
			res.SkippedExists++
			continue
		}

		ref, err := a.catalog.NextProductReference(ctx, organizationID)
		if err != nil {
			return fmt.Errorf("next product reference: %w", err)
		}

		desc := fmt.Sprintf("AUTO-DRAFT (Librarian): created because this item appears frequently (%d) as a missing catalog match. Sources=%v. Review title, unit, VAT and pricing before use.", g.TotalCount, g.Sources)
		product, err := a.catalog.CreateProduct(ctx, catalogrepo.CreateProductParams{
			OrganizationID: organizationID,
			VatRateID:      vatRateID,
			IsDraft:        true,
			Title:          g.Title,
			Reference:      ref,
			Description:    &desc,
			PriceCents:     0,
			UnitPriceCents: 0,
			UnitLabel:      strPtr("per stuk"),
			LaborTimeText:  nil,
			Type:           "material",
			PeriodCount:    nil,
			PeriodUnit:     nil,
		})
		if err != nil {
			return fmt.Errorf("create draft product: %w", err)
		}

		created++
		res.CreatedDrafts++
		if a.log != nil {
			a.log.Info("catalog gap: created draft product", "orgId", organizationID, "productId", product.ID, "title", product.Title, "count", g.TotalCount)
		}
	}
	return nil
}

func (a *CatalogGapAnalyzer) RunForOrganization(ctx context.Context, organizationID uuid.UUID, threshold int, lookbackDays int, maxDrafts int) (CatalogGapRunResult, error) {
	if a == nil || a.leads == nil || a.catalog == nil {
		return CatalogGapRunResult{}, fmt.Errorf("catalog gap analyzer not configured")
	}
	if organizationID == uuid.Nil {
		return CatalogGapRunResult{}, fmt.Errorf("organization_id is required")
	}
	if threshold <= 0 {
		return CatalogGapRunResult{OrganizationID: organizationID}, nil
	}
	if lookbackDays <= 0 {
		lookbackDays = 14
	}
	if maxDrafts <= 0 {
		maxDrafts = 10
	}

	ordered, err := a.gatherCandidates(ctx, organizationID, lookbackDays, threshold)
	if err != nil {
		return CatalogGapRunResult{}, err
	}

	res := CatalogGapRunResult{OrganizationID: organizationID, Candidates: len(ordered)}
	if len(ordered) == 0 {
		return res, nil
	}

	if err := a.createDrafts(ctx, organizationID, ordered, maxDrafts, &res); err != nil {
		return res, err
	}

	return res, nil
}

func (a *CatalogGapAnalyzer) pickDefaultVatRate(ctx context.Context, organizationID uuid.UUID) (uuid.UUID, error) {
	rates, _, err := a.catalog.ListVatRates(ctx, catalogrepo.ListVatRatesParams{
		OrganizationID: organizationID,
		Search:         "",
		Offset:         0,
		Limit:          50,
		SortBy:         "rateBps",
		SortOrder:      "desc",
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("list vat rates: %w", err)
	}
	if len(rates) == 0 {
		return uuid.Nil, fmt.Errorf("no VAT rates configured")
	}
	// Prefer 21% if present.
	for _, r := range rates {
		if r.RateBps == 2100 {
			return r.ID, nil
		}
	}
	return rates[0].ID, nil
}

func (a *CatalogGapAnalyzer) productAlreadyExists(ctx context.Context, organizationID uuid.UUID, title string) (bool, error) {
	items, _, err := a.catalog.ListProducts(ctx, catalogrepo.ListProductsParams{
		OrganizationID: organizationID,
		Search:         strings.TrimSpace(title),
		Limit:          5,
		Offset:         0,
		SortBy:         "title",
		SortOrder:      "asc",
	})
	if err != nil {
		return false, fmt.Errorf("list products: %w", err)
	}
	needle := normalizeGapText(title)
	for _, p := range items {
		if normalizeGapText(p.Title) == needle {
			return true, nil
		}
	}
	return false, nil
}

func strPtr(s string) *string { return &s }
