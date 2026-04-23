// Package client provides the HTTP client for EP-Online energy label API.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"portal_final_backend/internal/energylabel/transport"
	"portal_final_backend/platform/logger"
)

const (
	baseURL    = "https://public.ep-online.nl"
	apiVersion = "v5"
)

// Reusable time layouts to avoid allocation on every unmarshal call.
var timeLayouts = []string{
	"2006-01-02T15:04:05.9999999",
	"2006-01-02T15:04:05.999999",
	"2006-01-02T15:04:05.99999",
	"2006-01-02T15:04:05.9999",
	"2006-01-02T15:04:05.999",
	"2006-01-02T15:04:05.99",
	"2006-01-02T15:04:05.9",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// flexTime handles EP-Online timestamps that may lack timezone info.
type flexTime struct {
	time.Time
}

func (ft *flexTime) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "" || s == "null" {
		return nil
	}

	if t, err := time.Parse(time.RFC3339, s); err == nil {
		ft.Time = t
		return nil
	}

	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			ft.Time = t
			return nil
		}
	}
	return fmt.Errorf("cannot parse %q as time", s)
}

func (ft *flexTime) toPtr() *time.Time {
	if ft == nil || ft.IsZero() {
		return nil
	}
	t := ft.Time
	return &t
}

// Client provides access to the EP-Online API.
type Client struct {
	httpClient *http.Client
	log        *logger.Logger
	apiKey     string
}

func New(apiKey string, log *logger.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		apiKey:     apiKey,
		log:        log,
	}
}

func (c *Client) GetByAddress(ctx context.Context, postcode, huisnummer, huisletter, toevoeging, detail string) ([]transport.EnergyLabel, error) {
	v := url.Values{}
	v.Set("postcode", postcode)
	v.Set("huisnummer", huisnummer)
	if huisletter != "" {
		v.Set("huisletter", huisletter)
	}
	if toevoeging != "" {
		v.Set("huisnummertoevoeging", toevoeging)
	}
	if detail != "" {
		v.Set("detailaanduiding", detail)
	}

	return c.do(ctx, fmt.Sprintf("%s/api/%s/PandEnergielabel/Adres?%s", baseURL, apiVersion, v.Encode()))
}

func (c *Client) GetByBAGObjectID(ctx context.Context, objectID string) ([]transport.EnergyLabel, error) {
	return c.do(ctx, fmt.Sprintf("%s/api/%s/PandEnergielabel/AdresseerbaarObject/%s", baseURL, apiVersion, url.PathEscape(objectID)))
}

func (c *Client) Ping(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/%s/Ping", baseURL, apiVersion), nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed: %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) do(ctx context.Context, reqURL string) ([]transport.EnergyLabel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ep-online http: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var raw []apiEnergyLabel
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			return nil, err
		}
		// O(N) allocation: Pre-allocate capacity to minimize GC pressure.
		res := make([]transport.EnergyLabel, len(raw))
		for i := range raw {
			res[i] = raw[i].toTransport()
		}
		return res, nil
	case http.StatusNotFound:
		return nil, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("ep-online: unauthorized")
	default:
		return nil, fmt.Errorf("ep-online: upstream error %d", resp.StatusCode)
	}
}

// apiEnergyLabel reordered to minimize struct padding.
type apiEnergyLabel struct {
	Registratiedatum                        *flexTime `json:"Registratiedatum"`
	Opnamedatum                             *flexTime `json:"Opnamedatum"`
	GeldigTot                               *flexTime `json:"Geldig_tot"`
	Certificaathouder                       *string   `json:"Certificaathouder"`
	SoortOpname                             *string   `json:"Soort_opname"`
	Status                                  *string   `json:"Status"`
	Berekeningstype                         *string   `json:"Berekeningstype"`
	Gebouwklasse                            *string   `json:"Gebouwklasse"`
	Gebouwtype                              *string   `json:"Gebouwtype"`
	Gebouwsubtype                           *string   `json:"Gebouwsubtype"`
	SBIcode                                 *string   `json:"SBIcode"`
	Postcode                                *string   `json:"Postcode"`
	Huisletter                              *string   `json:"Huisletter"`
	Huisnummertoevoeging                    *string   `json:"Huisnummertoevoeging"`
	Detailaanduiding                        *string   `json:"Detailaanduiding"`
	BAGVerblijfsobjectID                    *string   `json:"BAGVerblijfsobjectID"`
	BAGLigplaatsID                          *string   `json:"BAGLigplaatsID"`
	BAGStandplaatsID                        *string   `json:"BAGStandplaatsID"`
	Energieklasse                           *string   `json:"Energieklasse"`
	BAGPandIDs                              []string  `json:"BAGPandIDs"`
	GebruiksoppervlakteThermischeZone       *float64  `json:"Gebruiksoppervlakte_thermische_zone"`
	Compactheid                             *float64  `json:"Compactheid"`
	EnergieIndex                            *float64  `json:"EnergieIndex"`
	EnergieIndexEMGForfaitair               *float64  `json:"EnergieIndex_EMG_forfaitair"`
	Energiebehoefte                         *float64  `json:"Energiebehoefte"`
	PrimaireFossieleEnergie                 *float64  `json:"PrimaireFossieleEnergie"`
	PrimaireFossieleEnergieEMGForfaitair    *float64  `json:"Primaire_fossiele_energie_EMG_forfaitair"`
	AandeelHernieuwbareEnergie              *float64  `json:"Aandeel_hernieuwbare_energie"`
	AandeelHernieuwbareEnergieEMGForfaitair *float64  `json:"Aandeel_hernieuwbare_energie_EMG_forfaitair"`
	Temperatuuroverschrijding               *float64  `json:"Temperatuuroverschrijding"`
	Warmtebehoefte                          *float64  `json:"Warmtebehoefte"`
	EisEnergiebehoefte                      *float64  `json:"Eis_energiebehoefte"`
	EisPrimaireFossieleEnergie              *float64  `json:"Eis_primaire_fossiele_energie"`
	EisAandeelHernieuwbareEnergie           *float64  `json:"Eis_aandeel_hernieuwbare_energie"`
	EisTemperatuuroverschrijding            *float64  `json:"Eis_temperatuuroverschrijding"`
	BerekendeCO2Emissie                     *float64  `json:"BerekendeCO2Emissie"`
	BerekendeEnergieverbruik                *float64  `json:"BerekendeEnergieverbruik"`
	Huisnummer                              int       `json:"Huisnummer"`
	Bouwjaar                                int       `json:"Bouwjaar"`
	IsVereenvoudigdLabel                    *bool     `json:"IsVereenvoudigdLabel"`
	OpBasisVanReferentiegebouw              bool      `json:"Op_basis_van_referentiegebouw"`
}

func (a *apiEnergyLabel) toTransport() transport.EnergyLabel {
	return transport.EnergyLabel{
		Registratiedatum:                  a.Registratiedatum.toPtr(),
		Opnamedatum:                       a.Opnamedatum.toPtr(),
		GeldigTot:                         a.GeldigTot.toPtr(),
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
		Energieklasse:                     val(a.Energieklasse),
		Certificaathouder:                 val(a.Certificaathouder),
		SoortOpname:                       val(a.SoortOpname),
		Status:                            val(a.Status),
		Berekeningstype:                   val(a.Berekeningstype),
		Gebouwklasse:                      val(a.Gebouwklasse),
		Gebouwtype:                        val(a.Gebouwtype),
		Gebouwsubtype:                     val(a.Gebouwsubtype),
		Postcode:                          val(a.Postcode),
		Huisletter:                        val(a.Huisletter),
		Huisnummertoevoeging:              val(a.Huisnummertoevoeging),
		Detailaanduiding:                  val(a.Detailaanduiding),
		BAGVerblijfsobjectID:              val(a.BAGVerblijfsobjectID),
		BAGLigplaatsID:                    val(a.BAGLigplaatsID),
		BAGStandplaatsID:                  val(a.BAGStandplaatsID),
	}
}

func val(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
