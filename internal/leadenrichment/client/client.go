// Package client provides HTTP clients for PDOK lead enrichment lookups.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"portal_final_backend/platform/logger"
)

const (
	pdokPC4Endpoint       = "https://api.pdok.nl/cbs/postcode4/ogc/v1/collections/postcode4/items"
	pdokPC6Endpoint       = "https://api.pdok.nl/cbs/postcode6/ogc/v1/collections/postcode6/items"
	pdokLocatieEndpoint   = "https://api.pdok.nl/bzk/locatieserver/search/v3_1/free"
	pdokBuurtenEndpoint   = "https://api.pdok.nl/cbs/wijken-en-buurten-2024/ogc/v1/collections/buurten/items"
	cbsODataEndpoint      = "https://opendata.cbs.nl/ODataApi/odata/85618NED/TypedDataSet" // Kerncijfers wijken en buurten 2024
	defaultHTTPTimeout    = 10 * time.Second
	blockedValueThreshold = -99990 // CBS uses -99995, -99997, etc. for privacy-suppressed data
	pc6LatestYear         = 2024
)

// FlexNumber handles JSON values that can be either string or number.
type FlexNumber float64

func (f *FlexNumber) UnmarshalJSON(data []byte) error {
	// Try as number first
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		*f = FlexNumber(num)
		return nil
	}
	// Try as string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "" {
			*f = 0
			return nil
		}
		parsed, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return err
		}
		*f = FlexNumber(parsed)
		return nil
	}
	return fmt.Errorf("cannot unmarshal %s into FlexNumber", string(data))
}

// Client handles PDOK requests.
type Client struct {
	httpClient *http.Client
	log        *logger.Logger
}

// New creates a new PDOK client.
func New(log *logger.Logger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
		log:        log,
	}
}

// PC6Properties holds PC6 properties from PDOK CBS Postcode6 API.
// Field names match the actual API response. Some fields use FlexNumber
// because the API inconsistently returns them as strings or numbers.
type PC6Properties struct {
	// Fields from PDOK CBS Postcode6 API
	GemiddeldGasverbruikWoning      *FlexNumber `json:"gemiddeld_gasverbruik_woning"`
	GemiddeldElektriciteitsverbruik *FlexNumber `json:"gemiddeld_elektriciteitsverbruik_woning"`
	GemiddeldHuishoudensgrootte     *FlexNumber `json:"gemiddelde_huishoudensgrootte"`
	GemiddeldWOZWaarde              *FlexNumber `json:"gemiddelde_woz_waarde_woning"`
	AantalWoningen                  *FlexNumber `json:"aantal_woningen"`
	AantalInwoners                  *FlexNumber `json:"aantal_inwoners"`
	AantalHuishoudens               *FlexNumber `json:"aantal_part_huishoudens"`

	// Percentage fields
	KoopwoningenPct           *FlexNumber `json:"percentage_koopwoningen"`
	HuurwoningenPct           *FlexNumber `json:"percentage_huurwoningen"`
	HuishoudensMetKinderenPct *FlexNumber `json:"percentage_huishoudens_met_kinderen"` // Derived from household types

	// Income/wealth fields (these can be strings in the API response!)
	MediaanInkomenHuishouden *FlexNumber `json:"mediaan_inkomen_huishouden"`

	// Age breakdown percentages
	Inwoners0Tot15Pct  *FlexNumber `json:"percentage_personen_0_tot_15_jaar"`
	Inwoners15Tot25Pct *FlexNumber `json:"percentage_personen_15_tot_25_jaar"`
	Inwoners25Tot45Pct *FlexNumber `json:"percentage_personen_25_tot_45_jaar"`
	Inwoners45Tot65Pct *FlexNumber `json:"percentage_personen_45_tot_65_jaar"`
	Inwoners65PlusPct  *FlexNumber `json:"percentage_personen_65_jaar_en_ouder"`

	// Building age counts (for calculating percentage built after 2000)
	WoningenBouwjaar05Tot15   *FlexNumber `json:"aantal_woningen_bouwjaar_05_tot_15"`
	WoningenBouwjaar15EnLater *FlexNumber `json:"aantal_woningen_bouwjaar_15_en_later"`
}

type pc6Response struct {
	Features []struct {
		Properties PC6Properties `json:"properties"`
	} `json:"features"`
}

// GetPC6 fetches PC6-level statistics from PDOK.
func (c *Client) GetPC6(ctx context.Context, postcode6 string) (*PC6Properties, bool, error) {
	params := url.Values{}
	params.Set("f", "json")
	params.Set("postcode6", postcode6)
	params.Set("jaarcode", fmt.Sprintf("%d", pc6LatestYear))
	params.Set("limit", "1")

	payload, err := c.fetchPC6(ctx, params)
	if err != nil {
		return nil, false, err
	}
	if len(payload.Features) == 0 {
		params.Del("jaarcode")
		payload, err = c.fetchPC6(ctx, params)
		if err != nil {
			return nil, false, err
		}
		if len(payload.Features) == 0 {
			return nil, false, nil
		}
	}

	props := payload.Features[0].Properties
	if isPC6Blocked(props) {
		return &props, true, nil
	}
	return &props, false, nil
}

func (c *Client) fetchPC6(ctx context.Context, params url.Values) (pc6Response, error) {
	reqURL := fmt.Sprintf("%s?%s", pdokPC6Endpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return pc6Response{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("pdok pc6 request failed", "error", err)
		return pc6Response{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.log.Error("pdok pc6 request error", "status", resp.StatusCode)
		return pc6Response{}, fmt.Errorf("pdok pc6 status %d", resp.StatusCode)
	}

	var payload pc6Response
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.log.Error("pdok pc6 decode failed", "error", err)
		return pc6Response{}, err
	}

	return payload, nil
}

func isPC6Blocked(props PC6Properties) bool {
	// Check if key fields have valid data (not privacy-suppressed)
	checks := []struct {
		hasValue  bool
		isBlocked bool
	}{
		checkBlockedFlexNumber(props.AantalInwoners),
		checkBlockedFlexNumber(props.AantalHuishoudens),
		checkBlockedFlexNumber(props.KoopwoningenPct),
	}

	for _, c := range checks {
		if !c.hasValue || c.isBlocked {
			return true
		}
	}
	return false
}

func checkBlockedFlexNumber(value *FlexNumber) struct {
	hasValue  bool
	isBlocked bool
} {
	if value == nil {
		return struct {
			hasValue  bool
			isBlocked bool
		}{false, false}
	}
	return struct {
		hasValue  bool
		isBlocked bool
	}{true, float64(*value) <= blockedValueThreshold}
}

// ToFloat64Ptr converts FlexNumber pointer to float64 pointer, filtering blocked values.
func (f *FlexNumber) ToFloat64Ptr() *float64 {
	if f == nil {
		return nil
	}
	val := float64(*f)
	if val <= blockedValueThreshold {
		return nil
	}
	return &val
}

// ToIntPtr converts FlexNumber pointer to int pointer, filtering blocked values.
func (f *FlexNumber) ToIntPtr() *int {
	if f == nil {
		return nil
	}
	val := int(*f)
	if float64(*f) <= blockedValueThreshold {
		return nil
	}
	return &val
}

// BuurtProperties holds properties from PDOK CBS Wijken en Buurten 2024 API.
// These provide richer statistics at the neighborhood (buurt) level.
type BuurtProperties struct {
	Buurtcode    string `json:"buurtcode"`
	Buurtnaam    string `json:"buurtnaam"`
	Gemeentenaam string `json:"gemeentenaam"`

	// Housing & ownership
	AantalWoningen     *FlexNumber `json:"aantal_woningen"`
	KoopwoningenPct    *FlexNumber `json:"percentage_koopwoningen"`
	HuurwoningenPct    *FlexNumber `json:"percentage_huurwoningen"`
	GemiddeldWOZWaarde *FlexNumber `json:"gemiddelde_woningwaarde"`

	// Building age
	BouwjaarVanaf2000Pct *FlexNumber `json:"percentage_bouwjaarklasse_vanaf_2000"`

	// Demographics
	AantalInwoners        *FlexNumber `json:"aantal_inwoners"`
	AantalHuishoudens     *FlexNumber `json:"aantal_particuliere_huishoudens"`
	GemHuishoudensgrootte *FlexNumber `json:"gemiddelde_huishoudensgrootte"`

	// Household types
	HuishoudensMetKinderenPct *FlexNumber `json:"percentage_huishoudens_met_kinderen"`
	EenpersoonsHuishoudensPct *FlexNumber `json:"percentage_eenpersoonshuishoudens"`

	// Income & wealth
	MediaanInkomen  *FlexNumber `json:"mediaan_besteedbaar_inkomen_per_inwoner"`
	MediaanVermogen *FlexNumber `json:"mediaan_vermogen_van_particuliere_huish"`

	// Energy usage
	GemiddeldGasverbruik     *FlexNumber `json:"gemiddeld_gasverbruik_totaal"`
	GemiddeldElektraverbruik *FlexNumber `json:"gemiddeld_elektriciteitsverbruik_totaal"`

	// Age breakdown
	Inwoners0Tot15Pct  *FlexNumber `json:"percentage_personen_0_tot_15_jaar"`
	Inwoners15Tot25Pct *FlexNumber `json:"percentage_personen_15_tot_25_jaar"`
	Inwoners25Tot45Pct *FlexNumber `json:"percentage_personen_25_tot_45_jaar"`
	Inwoners45Tot65Pct *FlexNumber `json:"percentage_personen_45_tot_65_jaar"`
	Inwoners65PlusPct  *FlexNumber `json:"percentage_personen_65_jaar_en_ouder"`
}

type buurtResponse struct {
	Features []struct {
		Properties BuurtProperties `json:"properties"`
	} `json:"features"`
}

type locatieResponse struct {
	Response struct {
		Docs []struct {
			Buurtcode string `json:"buurtcode"`
			Buurtnaam string `json:"buurtnaam"`
		} `json:"docs"`
	} `json:"response"`
}

// GetBuurtcode fetches the buurtcode for a postcode from PDOK locatieserver.
func (c *Client) GetBuurtcode(ctx context.Context, postcode6 string) (string, error) {
	params := url.Values{}
	params.Set("q", postcode6)
	params.Set("fq", "type:adres") // Use adres type - postcode type doesn't include buurtcode
	params.Set("rows", "1")
	params.Set("fl", "buurtcode,buurtnaam")

	reqURL := fmt.Sprintf("%s?%s", pdokLocatieEndpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("pdok locatie request failed", "error", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pdok locatie status %d", resp.StatusCode)
	}

	var payload locatieResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.log.Error("pdok locatie decode failed", "error", err)
		return "", err
	}

	if len(payload.Response.Docs) == 0 {
		return "", nil
	}

	return payload.Response.Docs[0].Buurtcode, nil
}

// GetBuurt fetches buurt-level statistics from PDOK CBS Wijken en Buurten 2024.
func (c *Client) GetBuurt(ctx context.Context, buurtcode string) (*BuurtProperties, bool, error) {
	if buurtcode == "" {
		return nil, false, nil
	}

	params := url.Values{}
	params.Set("f", "json")
	params.Set("buurtcode", buurtcode)
	params.Set("limit", "1")

	reqURL := fmt.Sprintf("%s?%s", pdokBuurtenEndpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("pdok buurt request failed", "error", err)
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.log.Error("pdok buurt request error", "status", resp.StatusCode)
		return nil, false, fmt.Errorf("pdok buurt status %d", resp.StatusCode)
	}

	var payload buurtResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.log.Error("pdok buurt decode failed", "error", err)
		return nil, false, err
	}

	if len(payload.Features) == 0 {
		return nil, false, nil
	}

	props := payload.Features[0].Properties
	if isBuurtBlocked(props) {
		return &props, true, nil
	}
	return &props, false, nil
}

func isBuurtBlocked(props BuurtProperties) bool {
	// Only consider buurt blocked if primary demographic fields are ALL blocked
	// (AantalInwoners OR AantalHuishoudens must be valid for any useful data)
	inwonersCheck := checkBlockedFlexNumber(props.AantalInwoners)
	huishoudensCheck := checkBlockedFlexNumber(props.AantalHuishoudens)

	// If neither population indicator is available/valid, buurt data is useless
	if (!inwonersCheck.hasValue || inwonersCheck.isBlocked) &&
		(!huishoudensCheck.hasValue || huishoudensCheck.isBlocked) {
		return true
	}
	return false
}

// CBSBuurtData holds data from CBS OData API for a buurt.
type CBSBuurtData struct {
	MediaanVermogen *float64 // Mediaan vermogen van particuliere huishoudens (× 1000 EUR)
}

type cbsODataResponse struct {
	Value []struct {
		MediaanVermogen *float64 `json:"MediaanVermogenVanParticuliereHuish_91"`
	} `json:"value"`
}

// GetCBSBuurtData fetches additional buurt data from CBS OData API (mediaan vermogen).
func (c *Client) GetCBSBuurtData(ctx context.Context, buurtcode string) (*CBSBuurtData, error) {
	if buurtcode == "" {
		return nil, nil
	}

	// CBS OData uses padded buurtcodes (e.g., "BU03580003  " with trailing spaces to 10 chars)
	paddedBuurtcode := fmt.Sprintf("%-10s", buurtcode)

	params := url.Values{}
	params.Set("$filter", fmt.Sprintf("WijkenEnBuurten eq '%s'", paddedBuurtcode))
	params.Set("$select", "MediaanVermogenVanParticuliereHuish_91")

	reqURL := fmt.Sprintf("%s?%s", cbsODataEndpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("cbs odata request failed", "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.log.Error("cbs odata request error", "status", resp.StatusCode)
		return nil, fmt.Errorf("cbs odata status %d", resp.StatusCode)
	}

	var payload cbsODataResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.log.Error("cbs odata decode failed", "error", err)
		return nil, err
	}

	if len(payload.Value) == 0 {
		return nil, nil
	}

	return &CBSBuurtData{
		MediaanVermogen: payload.Value[0].MediaanVermogen,
	}, nil
}

// PC4Properties holds PC4 properties from PDOK CBS Postcode4 API.
// PC4 has richer data than PC6, including gas, electricity, income, WOZ.
type PC4Properties struct {
	Postcode4 int `json:"postcode"`
	Jaarcode  int `json:"jaarcode"`
	DataYear  int // Populated by the fetching code

	// Energy usage
	GemiddeldGasverbruikWoning      *FlexNumber `json:"gemiddeld_gasverbruik_woning"`
	GemiddeldElektriciteitsverbruik *FlexNumber `json:"gemiddeld_elektriciteitsverbruik_woning"`

	// Housing
	AantalWoningen              *FlexNumber `json:"aantal_woningen"`
	GemiddeldHuishoudensgrootte *FlexNumber `json:"gemiddelde_huishoudensgrootte"`
	GemiddeldWOZWaarde          *FlexNumber `json:"gemiddelde_woz_waarde_woning"`
	KoopwoningenPct             *FlexNumber `json:"percentage_koopwoningen"`
	HuurwoningenPct             *FlexNumber `json:"percentage_huurwoningen"`

	// Income
	GemiddeldInkomen *FlexNumber `json:"gemiddeld_inkomen_huishouden"`
	PctHoogInkomen   *FlexNumber `json:"percentage_hoog_inkomen_huishouden"`
	PctLaagInkomen   *FlexNumber `json:"percentage_laag_inkomen_huishouden"`

	// Demographics
	AantalInwoners    *FlexNumber `json:"aantal_inwoners"`
	AantalHuishoudens *FlexNumber `json:"aantal_part_huishoudens"`
	Stedelijkheid     *FlexNumber `json:"stedelijkheid"`

	// Building age counts (for calculating percentage built after 2000)
	WoningenBouwjaarVoor1945  *FlexNumber `json:"aantal_woningen_bouwjaar_voor_1945"`
	WoningenBouwjaar45Tot65   *FlexNumber `json:"aantal_woningen_bouwjaar_45_tot_65"`
	WoningenBouwjaar65Tot75   *FlexNumber `json:"aantal_woningen_bouwjaar_65_tot_75"`
	WoningenBouwjaar75Tot85   *FlexNumber `json:"aantal_woningen_bouwjaar_75_tot_85"`
	WoningenBouwjaar85Tot95   *FlexNumber `json:"aantal_woningen_bouwjaar_85_tot_95"`
	WoningenBouwjaar95Tot05   *FlexNumber `json:"aantal_woningen_bouwjaar_95_tot_05"`
	WoningenBouwjaar05Tot15   *FlexNumber `json:"aantal_woningen_bouwjaar_05_tot_15"`
	WoningenBouwjaar15EnLater *FlexNumber `json:"aantal_woningen_bouwjaar_15_en_later"`

	// Household composition
	AantalEenouderhuishoudens  *FlexNumber `json:"aantal_eenouderhuishoudens"`
	AantalTweeouderhuishoudens *FlexNumber `json:"aantal_tweeouderhuishoudens"`
}

type pc4Response struct {
	Features []struct {
		Properties PC4Properties `json:"properties"`
	} `json:"features"`
}

// PC4YearlyData contains PC4 data merged from multiple years.
type PC4YearlyData struct {
	Properties PC4Properties
	Year       int  // Primary year (newest year with any data)
	Blocked    bool // True if no useful data found across all years
}

// GetPC4 fetches PC4-level statistics from PDOK, merging data from years 2024 -> 2023 -> 2022.
// For each field, uses the newest available non-blocked value.
func (c *Client) GetPC4(ctx context.Context, postcode4 string) (*PC4YearlyData, error) {
	years := []int{2024, 2023, 2022}

	// Collect data from all years
	var allData []*PC4Properties
	var primaryYear int
	for _, year := range years {
		data, _, err := c.fetchPC4Year(ctx, postcode4, year)
		if err != nil {
			c.log.Debug("pc4 fetch failed", "postcode4", postcode4, "year", year, "error", err)
			continue
		}
		if data == nil {
			continue
		}
		data.DataYear = year
		allData = append(allData, data)
		if primaryYear == 0 {
			primaryYear = year
		}
	}

	if len(allData) == 0 {
		return nil, nil
	}

	// Merge data: for each field, use newest non-blocked value
	merged := mergePC4Data(allData)
	merged.DataYear = primaryYear

	return &PC4YearlyData{
		Properties: merged,
		Year:       primaryYear,
		Blocked:    false,
	}, nil
}

// mergePC4Data merges PC4 data from multiple years, preferring newer years.
// allData should be sorted newest-first.
func mergePC4Data(allData []*PC4Properties) PC4Properties {
	if len(allData) == 0 {
		return PC4Properties{}
	}

	// Start with the newest data as base
	result := *allData[0]

	// For each older year, fill in any blocked/missing fields
	for i := 1; i < len(allData); i++ {
		older := allData[i]
		fillIfBlocked(&result.GemiddeldGasverbruikWoning, older.GemiddeldGasverbruikWoning)
		fillIfBlocked(&result.GemiddeldElektriciteitsverbruik, older.GemiddeldElektriciteitsverbruik)
		fillIfBlocked(&result.GemiddeldWOZWaarde, older.GemiddeldWOZWaarde)
		fillIfBlocked(&result.GemiddeldInkomen, older.GemiddeldInkomen)
		fillIfBlocked(&result.PctHoogInkomen, older.PctHoogInkomen)
		fillIfBlocked(&result.PctLaagInkomen, older.PctLaagInkomen)
		fillIfBlocked(&result.Stedelijkheid, older.Stedelijkheid)
		fillIfBlocked(&result.KoopwoningenPct, older.KoopwoningenPct)
		fillIfBlocked(&result.HuurwoningenPct, older.HuurwoningenPct)
		fillIfBlocked(&result.AantalWoningen, older.AantalWoningen)
		fillIfBlocked(&result.AantalInwoners, older.AantalInwoners)
		fillIfBlocked(&result.AantalHuishoudens, older.AantalHuishoudens)
		fillIfBlocked(&result.GemiddeldHuishoudensgrootte, older.GemiddeldHuishoudensgrootte)
		fillIfBlocked(&result.AantalEenouderhuishoudens, older.AantalEenouderhuishoudens)
		fillIfBlocked(&result.AantalTweeouderhuishoudens, older.AantalTweeouderhuishoudens)
		fillIfBlocked(&result.WoningenBouwjaar05Tot15, older.WoningenBouwjaar05Tot15)
		fillIfBlocked(&result.WoningenBouwjaar15EnLater, older.WoningenBouwjaar15EnLater)
	}

	return result
}

// fillIfBlocked fills dst with src if dst is nil or blocked.
func fillIfBlocked(dst **FlexNumber, src *FlexNumber) {
	if *dst == nil || float64(**dst) <= blockedValueThreshold {
		if src != nil && float64(*src) > blockedValueThreshold {
			*dst = src
		}
	}
}

func (c *Client) fetchPC4Year(ctx context.Context, postcode4 string, year int) (*PC4Properties, bool, error) {
	params := url.Values{}
	params.Set("f", "json")
	params.Set("postcode", postcode4)
	params.Set("jaarcode", fmt.Sprintf("%d", year))
	params.Set("limit", "1")

	reqURL := fmt.Sprintf("%s?%s", pdokPC4Endpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("pdok pc4 request failed", "error", err)
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.log.Error("pdok pc4 request error", "status", resp.StatusCode)
		return nil, false, fmt.Errorf("pdok pc4 status %d", resp.StatusCode)
	}

	var payload pc4Response
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.log.Error("pdok pc4 decode failed", "error", err)
		return nil, false, err
	}

	if len(payload.Features) == 0 {
		return nil, false, nil
	}

	props := payload.Features[0].Properties
	if isPC4Blocked(props) {
		return &props, true, nil
	}
	return &props, false, nil
}

func isPC4Blocked(props PC4Properties) bool {
	// PC4 data is considered blocked if the key VALUE fields we care about are blocked.
	// We want gas, electricity, income, WOZ - if ALL of these are blocked, try older year.
	// Note: AantalInwoners/Huishoudens/KoopwoningenPct are often valid even when value fields are blocked.
	valueChecks := []struct {
		hasValue  bool
		isBlocked bool
	}{
		checkBlockedFlexNumber(props.GemiddeldGasverbruikWoning),
		checkBlockedFlexNumber(props.GemiddeldElektriciteitsverbruik),
		checkBlockedFlexNumber(props.GemiddeldInkomen),
		checkBlockedFlexNumber(props.GemiddeldWOZWaarde),
	}

	// Count how many value fields are blocked
	blockedCount := 0
	for _, c := range valueChecks {
		if !c.hasValue || c.isBlocked {
			blockedCount++
		}
	}

	// If ALL 4 value fields are blocked, consider this data blocked and try older year
	return blockedCount == 4
}

// HuishoudensMetKinderenPct calculates percentage of households with children from PC4 data.
func (p *PC4Properties) HuishoudensMetKinderenPct() *float64 {
	huishoudens := p.AantalHuishoudens.ToIntPtr()
	eenouder := p.AantalEenouderhuishoudens.ToIntPtr()
	tweeouder := p.AantalTweeouderhuishoudens.ToIntPtr()

	if huishoudens == nil || *huishoudens == 0 {
		return nil
	}

	var metKinderen int
	if eenouder != nil {
		metKinderen += *eenouder
	}
	if tweeouder != nil {
		metKinderen += *tweeouder
	}

	pct := float64(metKinderen) / float64(*huishoudens) * 100
	return &pct
}

// BouwjaarVanaf2000Pct calculates percentage of buildings built after 2000 from PC4 data.
func (p *PC4Properties) BouwjaarVanaf2000Pct() *float64 {
	totaal := p.AantalWoningen.ToFloat64Ptr()
	if totaal == nil || *totaal == 0 {
		return nil
	}

	var recent float64
	if v := p.WoningenBouwjaar95Tot05.ToFloat64Ptr(); v != nil {
		recent += *v
	}
	if v := p.WoningenBouwjaar05Tot15.ToFloat64Ptr(); v != nil {
		recent += *v
	}
	if v := p.WoningenBouwjaar15EnLater.ToFloat64Ptr(); v != nil {
		recent += *v
	}

	pct := (recent / *totaal) * 100
	return &pct
}
