// Package client provides the HTTP client for EP-Online energy label API.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"portal_final_backend/internal/energylabel/transport"
	"portal_final_backend/platform/logger"
)

const (
	baseURL    = "https://public.ep-online.nl"
	apiVersion = "v5"
)

// Client is the HTTP client for EP-Online API.
type Client struct {
	httpClient *http.Client
	apiKey     string
	log        *logger.Logger
}

// New creates a new EP-Online API client.
func New(apiKey string, log *logger.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		apiKey:     apiKey,
		log:        log,
	}
}

// GetByAddress fetches energy label by address.
func (c *Client) GetByAddress(ctx context.Context, postcode, huisnummer, huisletter, toevoeging, detail string) ([]transport.EnergyLabel, error) {
	params := url.Values{}
	params.Set("postcode", postcode)
	params.Set("huisnummer", huisnummer)
	if huisletter != "" {
		params.Set("huisletter", huisletter)
	}
	if toevoeging != "" {
		params.Set("huisnummertoevoeging", toevoeging)
	}
	if detail != "" {
		params.Set("detailaanduiding", detail)
	}

	reqURL := fmt.Sprintf("%s/api/%s/PandEnergielabel/Adres?%s", baseURL, apiVersion, params.Encode())
	return c.doRequest(ctx, reqURL)
}

// GetByBAGObjectID fetches energy label by BAG adresseerbaar object ID.
func (c *Client) GetByBAGObjectID(ctx context.Context, objectID string) ([]transport.EnergyLabel, error) {
	reqURL := fmt.Sprintf("%s/api/%s/PandEnergielabel/AdresseerbaarObject/%s", baseURL, apiVersion, url.PathEscape(objectID))
	return c.doRequest(ctx, reqURL)
}

// Ping checks if the API is available.
func (c *Client) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/api/%s/Ping", baseURL, apiVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed: status %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) doRequest(ctx context.Context, reqURL string) ([]transport.EnergyLabel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("ep-online request failed", "error", err, "url", reqURL)
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// Success - continue to decode
	case http.StatusUnauthorized:
		c.log.Error("ep-online unauthorized", "status", resp.StatusCode)
		return nil, fmt.Errorf("unauthorized: invalid API key")
	case http.StatusNotFound:
		// No energy label found for this address - not an error
		c.log.Debug("ep-online no label found", "url", reqURL)
		return nil, nil
	case http.StatusBadRequest:
		c.log.Error("ep-online bad request", "status", resp.StatusCode, "url", reqURL)
		return nil, fmt.Errorf("bad request: invalid parameters")
	default:
		c.log.Error("ep-online upstream error", "status", resp.StatusCode, "url", reqURL)
		return nil, fmt.Errorf("upstream error: status %d", resp.StatusCode)
	}

	var apiLabels []apiEnergyLabel
	if err := json.NewDecoder(resp.Body).Decode(&apiLabels); err != nil {
		c.log.Error("ep-online decode failed", "error", err)
		return nil, fmt.Errorf("decode response: %w", err)
	}

	labels := make([]transport.EnergyLabel, 0, len(apiLabels))
	for _, api := range apiLabels {
		labels = append(labels, api.toTransport())
	}

	return labels, nil
}

// apiEnergyLabel is the raw response from EP-Online API (PandEnergielabelV5).
type apiEnergyLabel struct {
	Registratiedatum                        *time.Time `json:"Registratiedatum"`
	Opnamedatum                             *time.Time `json:"Opnamedatum"`
	GeldigTot                               *time.Time `json:"Geldig_tot"`
	Certificaathouder                       *string    `json:"Certificaathouder"`
	SoortOpname                             *string    `json:"Soort_opname"`
	Status                                  *string    `json:"Status"`
	Berekeningstype                         *string    `json:"Berekeningstype"`
	IsVereenvoudigdLabel                    *bool      `json:"IsVereenvoudigdLabel"`
	OpBasisVanReferentiegebouw              bool       `json:"Op_basis_van_referentiegebouw"`
	Gebouwklasse                            *string    `json:"Gebouwklasse"`
	Gebouwtype                              *string    `json:"Gebouwtype"`
	Gebouwsubtype                           *string    `json:"Gebouwsubtype"`
	SBIcode                                 *string    `json:"SBIcode"`
	Postcode                                *string    `json:"Postcode"`
	Huisnummer                              int        `json:"Huisnummer"`
	Huisletter                              *string    `json:"Huisletter"`
	Huisnummertoevoeging                    *string    `json:"Huisnummertoevoeging"`
	Detailaanduiding                        *string    `json:"Detailaanduiding"`
	BAGVerblijfsobjectID                    *string    `json:"BAGVerblijfsobjectID"`
	BAGLigplaatsID                          *string    `json:"BAGLigplaatsID"`
	BAGStandplaatsID                        *string    `json:"BAGStandplaatsID"`
	BAGPandIDs                              []string   `json:"BAGPandIDs"`
	Bouwjaar                                int        `json:"Bouwjaar"`
	GebruiksoppervlakteThermischeZone       *float64   `json:"Gebruiksoppervlakte_thermische_zone"`
	Compactheid                             *float64   `json:"Compactheid"`
	Energieklasse                           *string    `json:"Energieklasse"`
	EnergieIndex                            *float64   `json:"EnergieIndex"`
	EnergieIndexEMGForfaitair               *float64   `json:"EnergieIndex_EMG_forfaitair"`
	Energiebehoefte                         *float64   `json:"Energiebehoefte"`
	PrimaireFossieleEnergie                 *float64   `json:"PrimaireFossieleEnergie"`
	PrimaireFossieleEnergieEMGForfaitair    *float64   `json:"Primaire_fossiele_energie_EMG_forfaitair"`
	AandeelHernieuwbareEnergie              *float64   `json:"Aandeel_hernieuwbare_energie"`
	AandeelHernieuwbareEnergieEMGForfaitair *float64   `json:"Aandeel_hernieuwbare_energie_EMG_forfaitair"`
	Temperatuuroverschrijding               *float64   `json:"Temperatuuroverschrijding"`
	Warmtebehoefte                          *float64   `json:"Warmtebehoefte"`
	EisEnergiebehoefte                      *float64   `json:"Eis_energiebehoefte"`
	EisPrimaireFossieleEnergie              *float64   `json:"Eis_primaire_fossiele_energie"`
	EisAandeelHernieuwbareEnergie           *float64   `json:"Eis_aandeel_hernieuwbare_energie"`
	EisTemperatuuroverschrijding            *float64   `json:"Eis_temperatuuroverschrijding"`
	BerekendeCO2Emissie                     *float64   `json:"BerekendeCO2Emissie"`
	BerekendeEnergieverbruik                *float64   `json:"BerekendeEnergieverbruik"`
}

func (a *apiEnergyLabel) toTransport() transport.EnergyLabel {
	label := transport.EnergyLabel{
		Registratiedatum:                  a.Registratiedatum,
		Opnamedatum:                       a.Opnamedatum,
		GeldigTot:                         a.GeldigTot,
		Huisnummer:                        a.Huisnummer,
		Bouwjaar:                          a.Bouwjaar,
		OpBasisVanReferentiegebouw:        a.OpBasisVanReferentiegebouw,
		BAGPandIDs:                        a.BAGPandIDs,
		IsVereenvoudigdLabel:              a.IsVereenvoudigdLabel,
		EnergieIndex:                      a.EnergieIndex,
		Energiebehoefte:                   a.Energiebehoefte,
		PrimaireFossieleEnergie:           a.PrimaireFossieleEnergie,
		AandeelHernieuwbareEnergie:        a.AandeelHernieuwbareEnergie,
		Temperatuuroverschrijding:         a.Temperatuuroverschrijding,
		GebruiksoppervlakteThermischeZone: a.GebruiksoppervlakteThermischeZone,
		Compactheid:                       a.Compactheid,
		Warmtebehoefte:                    a.Warmtebehoefte,
		BerekendeCO2Emissie:               a.BerekendeCO2Emissie,
		BerekendeEnergieverbruik:          a.BerekendeEnergieverbruik,
	}

	// Copy string pointers
	if a.Energieklasse != nil {
		label.Energieklasse = *a.Energieklasse
	}
	if a.Certificaathouder != nil {
		label.Certificaathouder = *a.Certificaathouder
	}
	if a.SoortOpname != nil {
		label.SoortOpname = *a.SoortOpname
	}
	if a.Status != nil {
		label.Status = *a.Status
	}
	if a.Berekeningstype != nil {
		label.Berekeningstype = *a.Berekeningstype
	}
	if a.Gebouwklasse != nil {
		label.Gebouwklasse = *a.Gebouwklasse
	}
	if a.Gebouwtype != nil {
		label.Gebouwtype = *a.Gebouwtype
	}
	if a.Gebouwsubtype != nil {
		label.Gebouwsubtype = *a.Gebouwsubtype
	}
	if a.Postcode != nil {
		label.Postcode = *a.Postcode
	}
	if a.Huisletter != nil {
		label.Huisletter = *a.Huisletter
	}
	if a.Huisnummertoevoeging != nil {
		label.Huisnummertoevoeging = *a.Huisnummertoevoeging
	}
	if a.Detailaanduiding != nil {
		label.Detailaanduiding = *a.Detailaanduiding
	}
	if a.BAGVerblijfsobjectID != nil {
		label.BAGVerblijfsobjectID = *a.BAGVerblijfsobjectID
	}
	if a.BAGLigplaatsID != nil {
		label.BAGLigplaatsID = *a.BAGLigplaatsID
	}
	if a.BAGStandplaatsID != nil {
		label.BAGStandplaatsID = *a.BAGStandplaatsID
	}

	return label
}
