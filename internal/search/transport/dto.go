package transport

import "time"

type SearchRequest struct {
	Query string `form:"q" validate:"required,min=2,max=100"`
	Limit int    `form:"limit" validate:"omitempty,min=1,max=50"`
}

type SearchResultItem struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`         // "lead", "quote", "partner", "appointment"
	Title        string    `json:"title"`        // Primary display text (Name, Quote #)
	Subtitle     string    `json:"subtitle"`     // Secondary context (City, Status)
	Preview      string    `json:"preview"`      // Short snippet of what matched (notes/description/etc)
	Status       string    `json:"status"`       // Status badge text
	Link         string    `json:"link"`         // Frontend route
	Score        float64   `json:"score"`        // Relevance score
	MatchedField string    `json:"matchedField"` // Which field matched (debug/highlighting)
	CreatedAt    time.Time `json:"createdAt"`
}

type SearchResponse struct {
	Items []SearchResultItem `json:"items"`
	Total int                `json:"total"`
}
