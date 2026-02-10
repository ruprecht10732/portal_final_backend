package transport

import (
	"time"

	"github.com/google/uuid"
)

type CreateLeadNoteRequest struct {
	Body      string  `json:"body" validate:"required,min=1,max=2000"`
	Type      string  `json:"type" validate:"omitempty,oneof=note call text email system"`
	ServiceID *string `json:"serviceId" validate:"omitempty,uuid"`
}

type LeadNoteResponse struct {
	ID          uuid.UUID `json:"id"`
	LeadID      uuid.UUID `json:"leadId"`
	AuthorID    uuid.UUID `json:"authorId"`
	AuthorEmail string    `json:"authorEmail"`
	Type        string    `json:"type"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type LeadNotesResponse struct {
	Items []LeadNoteResponse `json:"items"`
}
