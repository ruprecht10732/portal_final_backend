// Package transport provides DTOs for the energy label domain.
// Data structures are optimized for 64-bit memory alignment (O(1) space optimization).
package transport

import "time"

// EnergyLabel represents the energy label data from EP-Online.
// Reordered by field size: Slices (24B) -> Strings (16B) -> Pointers/Ints (8B) -> Bools (1B).
type EnergyLabel struct {
	// 24-byte headers
	BAGPandIDs []string `json:"bagPandIds,omitempty"`

	// 16-byte headers
	Energieklasse        string `json:"energieklasse"`
	Gebouwklasse         string `json:"gebouwklasse,omitempty"`
	Gebouwtype           string `json:"gebouwtype,omitempty"`
	Gebouwsubtype        string `json:"gebouwsubtype,omitempty"`
	Postcode             string `json:"postcode,omitempty"`
	Huisletter           string `json:"huisletter,omitempty"`
	Huisnummertoevoeging string `json:"huisnummertoevoeging,omitempty"`
	Detailaanduiding     string `json:"detailaanduiding,omitempty"`
	BAGVerblijfsobjectID string `json:"bagVerblijfsobjectId,omitempty"`
	BAGLigplaatsID       string `json:"bagLigplaatsId,omitempty"`
	BAGStandplaatsID     string `json:"bagStandplaatsId,omitempty"`
	Certificaathouder    string `json:"certificaathouder,omitempty"`
	SoortOpname          string `json:"soortOpname,omitempty"`
	Status               string `json:"status,omitempty"`
	Berekeningstype      string `json:"berekeningstype,omitempty"`

	// 8-byte pointers, ints, and floats
	EnergieIndex                      *float64   `json:"energieIndex,omitempty"`
	Registratiedatum                  *time.Time `json:"registratiedatum,omitempty"`
	Opnamedatum                       *time.Time `json:"opnamedatum,omitempty"`
	GeldigTot                         *time.Time `json:"geldigTot,omitempty"`
	Energiebehoefte                   *float64   `json:"energiebehoefte,omitempty"`
	PrimaireFossieleEnergie           *float64   `json:"primaireFossieleEnergie,omitempty"`
	AandeelHernieuwbareEnergie        *float64   `json:"aandeelHernieuwbareEnergie,omitempty"`
	Temperatuuroverschrijding         *float64   `json:"temperatuuroverschrijding,omitempty"`
	GebruiksoppervlakteThermischeZone *float64   `json:"gebruiksoppervlakteThermischeZone,omitempty"`
	Compactheid                       *float64   `json:"compactheid,omitempty"`
	Warmtebehoefte                    *float64   `json:"warmtebehoefte,omitempty"`
	BerekendeCO2Emissie               *float64   `json:"berekendeCO2Emissie,omitempty"`
	BerekendeEnergieverbruik          *float64   `json:"berekendeEnergieverbruik,omitempty"`
	IsVereenvoudigdLabel              *bool      `json:"isVereenvoudigdLabel,omitempty"`
	Bouwjaar                          int        `json:"bouwjaar,omitempty"`
	Huisnummer                        int        `json:"huisnummer,omitempty"`

	// 1-byte primitives (placed at end to minimize trailing padding)
	OpBasisVanReferentiegebouw bool `json:"opBasisVanReferentiegebouw,omitempty"`
}

// GetByAddressRequest handles address-based energy label lookups.
type GetByAddressRequest struct {
	Postcode             string `form:"postcode" validate:"required,len=6"`
	Huisnummer           string `form:"huisnummer" validate:"required,min=1,max=10"`
	Huisletter           string `form:"huisletter,omitempty" validate:"omitempty,max=2"`
	Huisnummertoevoeging string `form:"huisnummertoevoeging,omitempty" validate:"omitempty,max=10"`
	Detailaanduiding     string `form:"detailaanduiding,omitempty" validate:"omitempty,max=50"`
}

// GetByBAGObjectIDRequest handles BAG ID-based energy label lookups.
type GetByBAGObjectIDRequest struct {
	AdresseerbaarObjectID string `form:"adresseerbaarObjectId" validate:"required,len=16"`
}
