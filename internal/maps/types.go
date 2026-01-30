package maps

// LookupRequest represents the query parameters from the frontend.
type LookupRequest struct {
	Query string `form:"q" binding:"required,min=3"`
}

// AddressSuggestion is the normalized data returned to the frontend form.
type AddressSuggestion struct {
	Label       string `json:"label"`
	Street      string `json:"street"`
	HouseNumber string `json:"houseNumber"`
	ZipCode     string `json:"zipCode"`
	City        string `json:"city"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
}

type nominatimAddress struct {
	Road         string `json:"road"`
	HouseNumber  string `json:"house_number"`
	Postcode     string `json:"postcode"`
	City         string `json:"city"`
	Town         string `json:"town"`
	Village      string `json:"village"`
	Municipality string `json:"municipality"`
	Hamlet       string `json:"hamlet"`
}

// nominatimResponse mirrors the relevant parts of the OSM search payload.
type nominatimResponse struct {
	DisplayName string           `json:"display_name"`
	Lat         string           `json:"lat"`
	Lon         string           `json:"lon"`
	Address     nominatimAddress `json:"address"`
}
