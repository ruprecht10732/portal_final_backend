package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/logger"
)

const nominatimURL = "https://nominatim.openstreetmap.org/search"

// Module wires the maps address lookup HTTP routes.
type Module struct {
	svc *Service
}

// NewModule creates a new maps Module.
func NewModule(log *logger.Logger) *Module {
	return &Module{svc: NewService(log)}
}

// Name returns the module name.
func (m *Module) Name() string {
	return "maps"
}

// RegisterRoutes registers the maps API endpoints.
func (m *Module) RegisterRoutes(ctx *apphttp.RouterContext) {
	group := ctx.Protected.Group("/maps")
	group.GET("/address-lookup", m.lookupAddress)
}

// lookupAddress handles GET /api/v1/maps/address-lookup?q=...
func (m *Module) lookupAddress(c *gin.Context) {
	var req struct {
		Query string `form:"q" binding:"required,min=3"`
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "query 'q' is required (min 3 chars)", nil)
		return
	}

	results, err := m.svc.SearchAddress(c.Request.Context(), req.Query)
	if err != nil {
		httpkit.Error(c, http.StatusBadGateway, "address lookup service unavailable", nil)
		return
	}

	httpkit.OK(c, results)
}

// Service handles address lookup via Nominatim.
type Service struct {
	client *http.Client
	log    *logger.Logger
}

// NewService creates a new Service for maps.
func NewService(log *logger.Logger) *Service {
	return &Service{
		client: &http.Client{Timeout: 5 * time.Second},
		log:    log,
	}
}

// AddressSuggestion is the normalized data returned to the caller.
type AddressSuggestion struct {
	Label       string `json:"label"`
	Street      string `json:"street"`
	HouseNumber string `json:"houseNumber"`
	ZipCode     string `json:"zipCode"`
	City        string `json:"city"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
}

// SearchAddress queries Nominatim for address suggestions.
func (s *Service) SearchAddress(ctx context.Context, query string) ([]AddressSuggestion, error) {
	params := url.Values{
		"q":              {query},
		"format":         {"json"},
		"addressdetails": {"1"},
		"limit":          {"5"},
		"countrycodes":   {"nl"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nominatimURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// Security: Prevent blocklisting by providing an explicit User-Agent
	req.Header.Set("User-Agent", "PortalApp/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Error("nominatim request failed", "error", err)
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.log.Error("nominatim upstream error", "status", resp.StatusCode)
		return nil, fmt.Errorf("upstream api error: %d", resp.StatusCode)
	}

	var rawResults []struct {
		Lat     string `json:"lat"`
		Lon     string `json:"lon"`
		Address struct {
			Road         string `json:"road"`
			HouseNumber  string `json:"house_number"`
			Postcode     string `json:"postcode"`
			City         string `json:"city"`
			Town         string `json:"town"`
			Village      string `json:"village"`
			Municipality string `json:"municipality"`
			Hamlet       string `json:"hamlet"`
		} `json:"address"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResults); err != nil {
		s.log.Error("failed to decode nominatim payload", "error", err)
		return nil, fmt.Errorf("decode error: %w", err)
	}

	suggestions := make([]AddressSuggestion, 0, len(rawResults))
	for _, raw := range rawResults {
		if raw.Address.Road == "" {
			continue
		}

		city := raw.Address.City
		if city == "" {
			city = raw.Address.Town
		}
		if city == "" {
			city = raw.Address.Village
		}
		if city == "" {
			city = raw.Address.Municipality
		}
		if city == "" {
			city = raw.Address.Hamlet
		}
		if city == "" {
			continue
		}

		sug := AddressSuggestion{
			Street:      raw.Address.Road,
			HouseNumber: raw.Address.HouseNumber,
			ZipCode:     raw.Address.Postcode,
			City:        city,
			Lat:         raw.Lat,
			Lon:         raw.Lon,
		}

		// Performance: Preallocate string builder capacity for O(N) concatenation
		var b strings.Builder
		b.Grow(len(sug.Street) + len(sug.HouseNumber) + len(sug.ZipCode) + len(sug.City) + 5)
		b.WriteString(sug.Street)
		if sug.HouseNumber != "" {
			b.WriteByte(' ')
			b.WriteString(sug.HouseNumber)
		}
		b.WriteString(", ")
		if sug.ZipCode != "" {
			b.WriteString(sug.ZipCode)
			b.WriteByte(' ')
		}
		b.WriteString(sug.City)
		sug.Label = b.String()

		suggestions = append(suggestions, sug)
	}

	return suggestions, nil
}
