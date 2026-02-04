package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"portal_final_backend/platform/logger"
)

const nominatimURL = "https://nominatim.openstreetmap.org/search"

type Service struct {
	client *http.Client
	log    *logger.Logger
}

func NewService(log *logger.Logger) *Service {
	return &Service{
		client: &http.Client{Timeout: 5 * time.Second},
		log:    log,
	}
}

func (s *Service) SearchAddress(ctx context.Context, query string) ([]AddressSuggestion, error) {
	params := url.Values{}
	params.Add("q", query)
	params.Add("format", "json")
	params.Add("addressdetails", "1")
	params.Add("limit", "5")
	params.Add("countrycodes", "nl")

	reqURL := fmt.Sprintf("%s?%s", nominatimURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "PortalApp/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Error("nominatim request failed", "error", err)
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		s.log.Error("nominatim upstream error", "status", resp.StatusCode)
		return nil, fmt.Errorf("upstream api error: %d", resp.StatusCode)
	}

	var rawResults []nominatimResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawResults); err != nil {
		s.log.Error("failed to decode nominatim payload", "error", err)
		return nil, err
	}

	suggestions := make([]AddressSuggestion, 0, len(rawResults))
	for _, raw := range rawResults {
		suggestion, ok := buildSuggestion(raw)
		if !ok {
			continue
		}

		suggestions = append(suggestions, suggestion)
	}

	return suggestions, nil
}

func buildSuggestion(raw nominatimResponse) (AddressSuggestion, bool) {
	if raw.Address.Road == "" {
		return AddressSuggestion{}, false
	}

	city := pickCity(raw.Address)
	if city == "" {
		return AddressSuggestion{}, false
	}

	suggestion := AddressSuggestion{
		Street:      raw.Address.Road,
		HouseNumber: raw.Address.HouseNumber,
		ZipCode:     raw.Address.Postcode,
		City:        city,
		Lat:         raw.Lat,
		Lon:         raw.Lon,
	}

	suggestion.Label = buildLabel(suggestion)

	return suggestion, true
}

func pickCity(address nominatimAddress) string {
	if address.City != "" {
		return address.City
	}
	if address.Town != "" {
		return address.Town
	}
	if address.Village != "" {
		return address.Village
	}
	if address.Municipality != "" {
		return address.Municipality
	}
	return address.Hamlet
}

func buildLabel(suggestion AddressSuggestion) string {
	parts := []string{suggestion.Street}
	if suggestion.HouseNumber != "" {
		parts = append(parts, suggestion.HouseNumber)
	}
	parts = append(parts, ",")
	if suggestion.ZipCode != "" {
		parts = append(parts, suggestion.ZipCode)
	}
	parts = append(parts, suggestion.City)

	label := strings.Join(parts, " ")
	label = strings.ReplaceAll(label, " ,", ",")
	return strings.TrimSpace(label)
}
