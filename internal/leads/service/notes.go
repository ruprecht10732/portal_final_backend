package service

import (
	"context"
	"strings"

	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"

	"github.com/google/uuid"
)

func (s *Service) AddNote(ctx context.Context, leadID uuid.UUID, authorID uuid.UUID, req transport.CreateLeadNoteRequest) (transport.LeadNoteResponse, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" || len(body) > 2000 {
		return transport.LeadNoteResponse{}, ErrInvalidNote
	}

	if _, err := s.repo.GetByID(ctx, leadID); err != nil {
		if err == repository.ErrNotFound {
			return transport.LeadNoteResponse{}, ErrLeadNotFound
		}
		return transport.LeadNoteResponse{}, err
	}

	note, err := s.repo.CreateLeadNote(ctx, repository.CreateLeadNoteParams{
		LeadID:   leadID,
		AuthorID: authorID,
		Body:     body,
	})
	if err != nil {
		return transport.LeadNoteResponse{}, err
	}

	return toLeadNoteResponse(note), nil
}

func (s *Service) ListNotes(ctx context.Context, leadID uuid.UUID) (transport.LeadNotesResponse, error) {
	if _, err := s.repo.GetByID(ctx, leadID); err != nil {
		if err == repository.ErrNotFound {
			return transport.LeadNotesResponse{}, ErrLeadNotFound
		}
		return transport.LeadNotesResponse{}, err
	}

	notes, err := s.repo.ListLeadNotes(ctx, leadID)
	if err != nil {
		return transport.LeadNotesResponse{}, err
	}

	items := make([]transport.LeadNoteResponse, len(notes))
	for i, note := range notes {
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
		Body:        note.Body,
		CreatedAt:   note.CreatedAt,
		UpdatedAt:   note.UpdatedAt,
	}
}
