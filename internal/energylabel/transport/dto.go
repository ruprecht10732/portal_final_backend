// Package transport provides DTOs for the energy label domain.
package transport

import "time"

// EnergyLabel represents the energy label data from EP-Online.
type EnergyLabel struct {
	// Core label information
	Energieklasse string   `json:"energieklasse"` // A+++, A++, A+, A, B, C, D, E, F, G
	EnergieIndex  *float64 `json:"energieIndex,omitempty"`

	// Registration info
	Registratiedatum *time.Time `json:"registratiedatum,omitempty"`
	Opnamedatum      *time.Time `json:"opnamedatum,omitempty"`
	GeldigTot        *time.Time `json:"geldigTot,omitempty"`

	// Building info
	Gebouwklasse  string `json:"gebouwklasse,omitempty"`  // Woning or Utiliteitsgebouw
	Gebouwtype    string `json:"gebouwtype,omitempty"`    // Type of dwelling
	Gebouwsubtype string `json:"gebouwsubtype,omitempty"` // Apartment position in building
	Bouwjaar      int    `json:"bouwjaar,omitempty"`

	// Address info (from BAG)
	Postcode             string `json:"postcode,omitempty"`
	Huisnummer           int    `json:"huisnummer,omitempty"`
	Huisletter           string `json:"huisletter,omitempty"`
	Huisnummertoevoeging string `json:"huisnummertoevoeging,omitempty"`
	Detailaanduiding     string `json:"detailaanduiding,omitempty"`

	// BAG identifiers
	BAGVerblijfsobjectID string   `json:"bagVerblijfsobjectId,omitempty"`
	BAGLigplaatsID       string   `json:"bagLigplaatsId,omitempty"`
	BAGStandplaatsID     string   `json:"bagStandplaatsId,omitempty"`
	BAGPandIDs           []string `json:"bagPandIds,omitempty"`

	// Energy performance metrics (NTA 8800)
	Energiebehoefte                   *float64 `json:"energiebehoefte,omitempty"`                   // kWh/m2·jaar
	PrimaireFossieleEnergie           *float64 `json:"primaireFossieleEnergie,omitempty"`           // kWh/m2·jaar
	AandeelHernieuwbareEnergie        *float64 `json:"aandeelHernieuwbareEnergie,omitempty"`        // %
	Temperatuuroverschrijding         *float64 `json:"temperatuuroverschrijding,omitempty"`         // TOjuli or GTO
	GebruiksoppervlakteThermischeZone *float64 `json:"gebruiksoppervlakteThermischeZone,omitempty"` // m2
	Compactheid                       *float64 `json:"compactheid,omitempty"`                       // ratio
	Warmtebehoefte                    *float64 `json:"warmtebehoefte,omitempty"`                    // kWh/m2·jaar (EPV)
	BerekendeCO2Emissie               *float64 `json:"berekendeCO2Emissie,omitempty"`               // kg/m2·jaar
	BerekendeEnergieverbruik          *float64 `json:"berekendeEnergieverbruik,omitempty"`          // kWh/m2·jaar

	// Label metadata
	Certificaathouder          string `json:"certificaathouder,omitempty"`
	SoortOpname                string `json:"soortOpname,omitempty"` // Basis or Detail
	Status                     string `json:"status,omitempty"`
	Berekeningstype            string `json:"berekeningstype,omitempty"`
	IsVereenvoudigdLabel       *bool  `json:"isVereenvoudigdLabel,omitempty"` // VEL indicator
	OpBasisVanReferentiegebouw bool   `json:"opBasisVanReferentiegebouw,omitempty"`
}

// GetByAddressRequest contains parameters for looking up energy label by address.
type GetByAddressRequest struct {
	Postcode             string `json:"postcode" validate:"required,len=6"`
	Huisnummer           string `json:"huisnummer" validate:"required,min=1,max=5"`
	Huisletter           string `json:"huisletter,omitempty" validate:"omitempty,len=1"`
	Huisnummertoevoeging string `json:"huisnummertoevoeging,omitempty" validate:"omitempty,max=4"`
	Detailaanduiding     string `json:"detailaanduiding,omitempty"`
}

// GetByBAGObjectIDRequest contains parameters for looking up energy label by BAG ID.
type GetByBAGObjectIDRequest struct {
	AdresseerbaarObjectID string `json:"adresseerbaarObjectId" validate:"required,len=16"`
}
