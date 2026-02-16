package service

import (
	"context"
	"time"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/internal/quotes/repository"

	"github.com/google/uuid"
)

const (
	msgTotalFormat  = "Total: €%.2f"
	msgLinkExpired  = "this quote link has expired"
	msgAlreadyFinal = "this quote has already been finalized"
	msgReadOnly     = "this preview link is read-only"
	msgInvalidField = "invalid "

	defaultPaymentTermDays   = 7
	defaultQuoteValidityDays = 14
	defaultPublicTokenTTL    = 30 * 24 * time.Hour

	jobStepQueued              = "Queued"
	jobStepQueueFailed         = "Queueing failed"
	jobStepPreparingContext    = "Preparing quote context"
	jobStepGeneratingAIQuote   = "Generating AI quote"
	jobStepGenerationFailed    = "Generation failed"
	jobStepFinalizingAndSaving = "Finalizing and saving"
	jobStepCompleted           = "Completed"
)

// TimelineWriter is the narrow interface a quotes service needs to create lead timeline events.
type TimelineWriter interface {
	CreateTimelineEvent(ctx context.Context, params TimelineEventParams) error
}

// TimelineEventParams captures timeline event data without importing the leads domain.
type TimelineEventParams struct {
	LeadID         uuid.UUID
	ServiceID      *uuid.UUID
	OrganizationID uuid.UUID
	ActorType      string
	ActorName      string
	EventType      string
	Title          string
	Summary        *string
	Metadata       map[string]any
}

// QuoteContactData holds the consumer/organization/agent info needed for quote emails.
type QuoteContactData struct {
	ConsumerEmail    string
	ConsumerName     string
	ConsumerPhone    string
	OrganizationName string
	AgentEmail       string
	AgentName        string
}

// QuoteContactReader fetches contact details for quote workflows.
type QuoteContactReader interface {
	GetQuoteContactData(ctx context.Context, leadID uuid.UUID, organizationID uuid.UUID) (QuoteContactData, error)
}

// QuoteTermsResolver resolves effective quote terms (payment + validity days).
type QuoteTermsResolver interface {
	ResolveQuoteTerms(ctx context.Context, organizationID uuid.UUID, leadID uuid.UUID, leadServiceID *uuid.UUID) (paymentDays int, validDays int, err error)
}

// QuotePromptGenerator generates quotes from prompt input.
type QuotePromptGenerator interface {
	GenerateFromPrompt(ctx context.Context, leadID, serviceID, tenantID uuid.UUID, prompt string, existingQuoteID *uuid.UUID) (*GenerateQuoteResult, error)
}

// GenerateQuoteResult is the result of a prompt-based quote generation.
type GenerateQuoteResult struct {
	QuoteID     uuid.UUID
	QuoteNumber string
	ItemCount   int
}

// Service provides business logic for quotes.
type Service struct {
	repo       *repository.Repository
	timeline   TimelineWriter
	eventBus   events.Bus
	contacts   QuoteContactReader
	quoteTerms QuoteTermsResolver
	promptGen  QuotePromptGenerator
	sse        *sse.Service
	jobQueue   GenerateQuoteJobQueue
}

// GenerateQuoteJobQueue enqueues async quote generation tasks.
type GenerateQuoteJobQueue interface {
	EnqueueGenerateQuoteJobRequest(ctx context.Context, jobID, tenantID, userID, leadID, leadServiceID uuid.UUID, prompt string, quoteID *uuid.UUID) error
}

// GenerateQuoteJobStatus is the status for an async quote generation job.
type GenerateQuoteJobStatus string

const (
	GenerateQuoteJobStatusPending   GenerateQuoteJobStatus = "pending"
	GenerateQuoteJobStatusRunning   GenerateQuoteJobStatus = "running"
	GenerateQuoteJobStatusCompleted GenerateQuoteJobStatus = "completed"
	GenerateQuoteJobStatusFailed    GenerateQuoteJobStatus = "failed"
)

// GenerateQuoteJob stores progress and result data for an async generation run.
type GenerateQuoteJob struct {
	JobID           uuid.UUID
	TenantID        uuid.UUID
	UserID          uuid.UUID
	LeadID          uuid.UUID
	LeadServiceID   uuid.UUID
	Status          GenerateQuoteJobStatus
	Step            string
	ProgressPercent int
	Error           *string
	QuoteID         *uuid.UUID
	QuoteNumber     *string
	ItemCount       *int
	StartedAt       time.Time
	UpdatedAt       time.Time
	FinishedAt      *time.Time
}

// DraftQuoteParams contains the data needed to create or update an AI-drafted quote.
type DraftQuoteParams struct {
	QuoteID        *uuid.UUID
	LeadID         uuid.UUID
	LeadServiceID  uuid.UUID
	OrganizationID uuid.UUID
	CreatedByID    uuid.UUID
	Notes          string
	Items          []DraftQuoteItemParams
	Attachments    []DraftQuoteAttachmentParams
	URLs           []DraftQuoteURLParams
}

type DraftQuoteItemParams struct {
	Description      string
	Quantity         string
	UnitPriceCents   int64
	TaxRateBps       int
	IsOptional       bool
	CatalogProductID *uuid.UUID
}

type DraftQuoteAttachmentParams struct {
	Filename         string
	FileKey          string
	Source           string
	CatalogProductID *uuid.UUID
}

type DraftQuoteURLParams struct {
	Label            string
	Href             string
	CatalogProductID *uuid.UUID
}

type DraftQuoteResult struct {
	QuoteID     uuid.UUID
	QuoteNumber string
	ItemCount   int
}

// PublicQuoteStorageMeta contains storage-related identifiers resolved from a public token.
type PublicQuoteStorageMeta struct {
	QuoteID    uuid.UUID
	OrgID      uuid.UUID
	PDFFileKey string
}

// New creates a new quotes service.
func New(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetTimelineWriter(tw TimelineWriter)                  { s.timeline = tw }
func (s *Service) SetEventBus(bus events.Bus)                           { s.eventBus = bus }
func (s *Service) SetQuoteContactReader(cr QuoteContactReader)          { s.contacts = cr }
func (s *Service) SetQuoteTermsResolver(r QuoteTermsResolver)           { s.quoteTerms = r }
func (s *Service) SetQuotePromptGenerator(g QuotePromptGenerator)       { s.promptGen = g }
func (s *Service) SetSSEService(sseSvc *sse.Service)                    { s.sse = sseSvc }
func (s *Service) SetGenerateQuoteJobQueue(queue GenerateQuoteJobQueue) { s.jobQueue = queue }
