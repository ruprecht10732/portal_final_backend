package adapters

import (
	"context"
	"errors"
	"strings"

	identityservice "portal_final_backend/internal/identity/service"
	imapservice "portal_final_backend/internal/imap/service"
	leadmgmt "portal_final_backend/internal/leads/management"
	leadrepo "portal_final_backend/internal/leads/repository"
	leadstransport "portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

type InboxLeadActionsAdapter struct {
	management *leadmgmt.Service
	repo       leadrepo.LeadsRepository
}

const (
	errLeadRepositoryNotConfigured = "lead repository is not configured"
	errLeadServiceNotFound         = "lead service not found"
)

func NewInboxLeadActionsAdapter(management *leadmgmt.Service, repo leadrepo.LeadsRepository) *InboxLeadActionsAdapter {
	return &InboxLeadActionsAdapter{management: management, repo: repo}
}

func (a *InboxLeadActionsAdapter) Create(ctx context.Context, req leadstransport.CreateLeadRequest, tenantID uuid.UUID) (leadstransport.LeadResponse, error) {
	if a == nil || a.management == nil {
		return leadstransport.LeadResponse{}, apperr.Internal("lead management is not configured")
	}
	return a.management.Create(ctx, req, tenantID)
}

func (a *InboxLeadActionsAdapter) ResolveServiceID(ctx context.Context, leadID, organizationID uuid.UUID, requestedServiceID *uuid.UUID) (uuid.UUID, error) {
	if a == nil || a.repo == nil {
		return uuid.UUID{}, apperr.Internal(errLeadRepositoryNotConfigured)
	}
	if requestedServiceID != nil {
		service, err := a.repo.GetLeadServiceByID(ctx, *requestedServiceID, organizationID)
		if err != nil {
			if errors.Is(err, leadrepo.ErrServiceNotFound) {
				return uuid.UUID{}, apperr.NotFound(errLeadServiceNotFound)
			}
			return uuid.UUID{}, err
		}
		if service.LeadID != leadID {
			return uuid.UUID{}, apperr.Validation("lead service does not belong to lead")
		}
		return service.ID, nil
	}

	service, err := a.repo.GetCurrentLeadService(ctx, leadID, organizationID)
	if err != nil {
		if errors.Is(err, leadrepo.ErrServiceNotFound) {
			return uuid.UUID{}, apperr.NotFound(errLeadServiceNotFound)
		}
		return uuid.UUID{}, err
	}
	return service.ID, nil
}

func (a *InboxLeadActionsAdapter) CreateAttachment(ctx context.Context, params identityservice.CreateLeadAttachmentParams) (identityservice.CreateLeadAttachmentResult, error) {
	if a == nil || a.repo == nil {
		return identityservice.CreateLeadAttachmentResult{}, apperr.Internal(errLeadRepositoryNotConfigured)
	}
	service, err := a.repo.GetLeadServiceByID(ctx, params.ServiceID, params.OrganizationID)
	if err != nil {
		if errors.Is(err, leadrepo.ErrServiceNotFound) {
			return identityservice.CreateLeadAttachmentResult{}, apperr.NotFound(errLeadServiceNotFound)
		}
		return identityservice.CreateLeadAttachmentResult{}, err
	}
	if service.LeadID != params.LeadID {
		return identityservice.CreateLeadAttachmentResult{}, apperr.Validation("lead service does not belong to lead")
	}

	attachment, err := a.repo.CreateAttachment(ctx, leadrepo.CreateAttachmentParams{
		LeadServiceID:  params.ServiceID,
		OrganizationID: params.OrganizationID,
		FileKey:        params.FileKey,
		FileName:       params.FileName,
		ContentType:    params.ContentType,
		SizeBytes:      params.SizeBytes,
		UploadedBy:     &params.AuthorID,
	})
	if err != nil {
		return identityservice.CreateLeadAttachmentResult{}, err
	}

	return identityservice.CreateLeadAttachmentResult{AttachmentID: attachment.ID}, nil
}

func (a *InboxLeadActionsAdapter) CreateImportantNote(ctx context.Context, params identityservice.CreateImportantLeadNoteParams) (identityservice.CreateImportantLeadNoteResult, error) {
	if a == nil || a.repo == nil {
		return identityservice.CreateImportantLeadNoteResult{}, apperr.Internal(errLeadRepositoryNotConfigured)
	}

	serviceID := params.ServiceID
	if serviceID == nil {
		resolvedID, err := a.ResolveServiceID(ctx, params.LeadID, params.OrganizationID, nil)
		if err != nil {
			return identityservice.CreateImportantLeadNoteResult{}, err
		}
		serviceID = &resolvedID
	}

	note, err := a.repo.CreateLeadNote(ctx, leadrepo.CreateLeadNoteParams{
		LeadID:         params.LeadID,
		OrganizationID: params.OrganizationID,
		AuthorID:       params.AuthorID,
		Type:           "note",
		Body:           strings.TrimSpace(params.Body),
		ServiceID:      serviceID,
	})
	if err != nil {
		return identityservice.CreateImportantLeadNoteResult{}, err
	}

	return identityservice.CreateImportantLeadNoteResult{NoteID: note.ID, ServiceID: note.ServiceID}, nil
}

func (a *InboxLeadActionsAdapter) CreateTimelineEvent(ctx context.Context, params leadrepo.CreateTimelineEventParams) (leadrepo.TimelineEvent, error) {
	if a == nil || a.repo == nil {
		return leadrepo.TimelineEvent{}, apperr.Internal(errLeadRepositoryNotConfigured)
	}
	return a.repo.CreateTimelineEvent(ctx, params)
}

var _ identityservice.WhatsAppLeadActions = (*InboxLeadActionsAdapter)(nil)
var _ imapservice.InboxLeadActions = (*InboxLeadActionsAdapter)(nil)
