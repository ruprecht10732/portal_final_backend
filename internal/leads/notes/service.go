// Package notes handles lead note operations.
// This is a vertically sliced feature package containing service logic
// for creating and listing notes on leads.
package notes

import (
	"context"
	"errors"
	"strings"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

// ValidNoteTypes defines the allowed note types.
var ValidNoteTypes = map[string]bool{
	"note":   true,
	"call":   true,
	"text":   true,
	"email":  true,
	"system": true,
}

// Repository defines the data access interface needed by the notes service.
// This is a consumer-driven interface - only what notes needs.
type Repository interface {
	// LeadExistenceChecker
	GetByID(ctx context.Context, id uuid.UUID) (repository.Lead, error)
	// NoteStore
	repository.NoteStore
}

// Service handles lead note operations.
type Service struct {
	repo Repository
}

// New creates a new notes service.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// Add adds a new note to a lead.
func (s *Service) Add(ctx context.Context, leadID uuid.UUID, authorID uuid.UUID, req transport.CreateLeadNoteRequest) (transport.LeadNoteResponse, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" || len(body) > 2000 {
		return transport.LeadNoteResponse{}, apperr.Validation("note body must be between 1 and 2000 characters")
	}

	noteType := strings.TrimSpace(req.Type)
	if noteType == "" {
		noteType = "note"
	}
	if !ValidNoteTypes[noteType] {
		return transport.LeadNoteResponse{}, apperr.Validation("invalid note type")
	}

	// Verify lead exists
	if _, err := s.repo.GetByID(ctx, leadID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadNoteResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadNoteResponse{}, err
	}

	note, err := s.repo.CreateLeadNote(ctx, repository.CreateLeadNoteParams{
		LeadID:   leadID,
		AuthorID: authorID,
		Type:     noteType,
		Body:     body,
	})
	if err != nil {
		return transport.LeadNoteResponse{}, err
	}

	return toLeadNoteResponse(note), nil
}

// List retrieves all notes for a lead.
func (s *Service) List(ctx context.Context, leadID uuid.UUID) (transport.LeadNotesResponse, error) {
	// Verify lead exists
	if _, err := s.repo.GetByID(ctx, leadID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadNotesResponse{}, apperr.NotFound("lead not found")
		}
		return transport.LeadNotesResponse{}, err
	}

	notesList, err := s.repo.ListLeadNotes(ctx, leadID)
	if err != nil {
		return transport.LeadNotesResponse{}, err
	}

	items := make([]transport.LeadNoteResponse, len(notesList))
	for i, note := range notesList {
		items[i] = toLeadNoteResponse(note)
	}

	return transport.LeadNotesResponse{Items: items}, nil
}

func toLeadNoteResponse(note repository.LeadNote) transport.LeadNoteResponse {
	return transport.LeadNoteResponse{
		ID:          note.ID,
		LeadID:      note.LeadID,
		AuthorID:    note.AuthorID,
		AuthorEmail: note.AuthorEmail,
		Type:        note.Type,
		Body:        note.Body,
		CreatedAt:   note.CreatedAt,
		UpdatedAt:   note.UpdatedAt,
	}
}
