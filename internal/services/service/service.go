package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"portal_final_backend/internal/services/repository"
	"portal_final_backend/internal/services/transport"
	"portal_final_backend/platform/logger"

	"github.com/jackc/pgx/v5/pgconn"
)

// Service provides business logic for service types.
type Service struct {
	repo repository.Repository
	log  *logger.Logger
}

// New creates a new service types service.
func New(repo repository.Repository, log *logger.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// GetByID retrieves a service type by ID.
func (s *Service) GetByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (transport.ServiceTypeResponse, error) {
	st, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}
	return toResponse(st), nil
}

// GetBySlug retrieves a service type by slug.
func (s *Service) GetBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (transport.ServiceTypeResponse, error) {
	st, err := s.repo.GetBySlug(ctx, tenantID, slug)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}
	return toResponse(st), nil
}

// List retrieves all service types (admin default list).
func (s *Service) List(ctx context.Context, tenantID uuid.UUID) (transport.ServiceTypeListResponse, error) {
	items, err := s.repo.List(ctx, tenantID)
	if err != nil {
		return transport.ServiceTypeListResponse{}, err
	}
	return toListResponseWithPagination(items, len(items), 1, len(items)), nil
}

// ListWithFilters retrieves service types with search, filters, and pagination (admin).
func (s *Service) ListWithFilters(ctx context.Context, tenantID uuid.UUID, req transport.ListServiceTypesRequest) (transport.ServiceTypeListResponse, error) {
	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	isActive := req.IsActive
	if isActive == nil {
		defaultActive := true
		isActive = &defaultActive
	}

	params := repository.ListParams{
		OrganizationID: tenantID,
		Search:         req.Search,
		IsActive:       isActive,
		Offset:         (page - 1) * pageSize,
		Limit:          pageSize,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
	}

	items, total, err := s.repo.ListWithFilters(ctx, params)
	if err != nil {
		return transport.ServiceTypeListResponse{}, err
	}

	return toListResponseWithPagination(items, total, page, pageSize), nil
}

// ListActive retrieves only active service types.
func (s *Service) ListActive(ctx context.Context, tenantID uuid.UUID) (transport.ServiceTypeListResponse, error) {
	items, err := s.repo.ListActive(ctx, tenantID)
	if err != nil {
		return transport.ServiceTypeListResponse{}, err
	}
	return toListResponseWithPagination(items, len(items), 1, len(items)), nil
}

// Create creates a new service type.
func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, req transport.CreateServiceTypeRequest) (transport.ServiceTypeResponse, error) {
	params := repository.CreateParams{
		OrganizationID:   tenantID,
		Name:             req.Name,
		Slug:             generateSlug(req.Name),
		Description:      req.Description,
		IntakeGuidelines: req.IntakeGuidelines,
		Icon:             req.Icon,
		Color:            req.Color,
	}

	st, err := s.repo.Create(ctx, params)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	s.log.Info("service type created", "id", st.ID, "name", st.Name, "slug", st.Slug)
	return toResponse(st), nil
}

// Update updates an existing service type.
func (s *Service) Update(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, req transport.UpdateServiceTypeRequest) (transport.ServiceTypeResponse, error) {
	var slug *string
	if req.Name != nil {
		newSlug := generateSlug(*req.Name)
		slug = &newSlug
	}

	params := repository.UpdateParams{
		ID:               id,
		OrganizationID:   tenantID,
		Name:             req.Name,
		Slug:             slug,
		Description:      req.Description,
		IntakeGuidelines: req.IntakeGuidelines,
		Icon:             req.Icon,
		Color:            req.Color,
	}

	st, err := s.repo.Update(ctx, params)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	s.log.Info("service type updated", "id", st.ID, "name", st.Name)
	return toResponse(st), nil
}

// Delete removes or deactivates a service type based on usage.
func (s *Service) Delete(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (transport.DeleteServiceTypeResponse, error) {
	used, err := s.repo.HasLeadServices(ctx, tenantID, id)
	if err != nil {
		return transport.DeleteServiceTypeResponse{}, err
	}

	if used {
		if err := s.repo.SetActive(ctx, tenantID, id, false); err != nil {
			return transport.DeleteServiceTypeResponse{}, err
		}
		s.log.Info("service type deactivated", "id", id)
		return transport.DeleteServiceTypeResponse{Status: "deactivated"}, nil
	}

	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return transport.DeleteServiceTypeResponse{}, err
	}

	s.log.Info("service type deleted", "id", id)
	return transport.DeleteServiceTypeResponse{Status: "deleted"}, nil
}

// ToggleActive toggles the is_active flag for a service type.
func (s *Service) ToggleActive(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (transport.ServiceTypeResponse, error) {
	// Get current state
	st, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	// Toggle
	newActive := !st.IsActive
	if err := s.repo.SetActive(ctx, tenantID, id, newActive); err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	// Get updated record
	st, err = s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	s.log.Info("service type active toggled", "id", id, "isActive", newActive)
	return toResponse(st), nil
}

// Exists checks if a service type exists by ID.
func (s *Service) Exists(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (bool, error) {
	return s.repo.Exists(ctx, tenantID, id)
}

// SeedDefaults ensures a tenant has the default service types.
func (s *Service) SeedDefaults(ctx context.Context, tenantID uuid.UUID) error {
	items, err := s.repo.List(ctx, tenantID)
	if err != nil {
		return err
	}

	// If the org has existing types, only ensure "Algemeen" exists (the catch-all).
	// If the org has no types at all, seed everything.
	if len(items) > 0 {
		return s.ensureAlgemeen(ctx, tenantID, items)
	}

	for _, def := range defaultServiceTypes {
		_, err := s.repo.Create(ctx, repository.CreateParams{
			OrganizationID:   tenantID,
			Name:             def.Name,
			Slug:             def.Slug,
			Description:      toPtr(def.Description),
			IntakeGuidelines: toPtr(def.IntakeGuidelines),
			Icon:             toPtr(def.Icon),
			Color:            toPtr(def.Color),
		})
		if err != nil {
			if isDuplicateServiceType(err) {
				continue
			}
			return err
		}
	}

	return nil
}

// ensureAlgemeen makes sure the "Algemeen" service type exists for a tenant.
func (s *Service) ensureAlgemeen(ctx context.Context, tenantID uuid.UUID, existing []repository.ServiceType) error {
	for _, st := range existing {
		if st.Slug == "algemeen" {
			return nil // already exists
		}
	}

	// Find the Algemeen definition
	for _, def := range defaultServiceTypes {
		if def.Slug == "algemeen" {
			_, err := s.repo.Create(ctx, repository.CreateParams{
				OrganizationID:   tenantID,
				Name:             def.Name,
				Slug:             def.Slug,
				Description:      toPtr(def.Description),
				IntakeGuidelines: toPtr(def.IntakeGuidelines),
				Icon:             toPtr(def.Icon),
				Color:            toPtr(def.Color),
			})
			if err != nil && !isDuplicateServiceType(err) {
				return err
			}
			return nil
		}
	}
	return nil
}

// toResponse converts a repository ServiceType to transport response.
func toResponse(st repository.ServiceType) transport.ServiceTypeResponse {
	return transport.ServiceTypeResponse{
		ID:               st.ID,
		Name:             st.Name,
		Slug:             st.Slug,
		Description:      st.Description,
		IntakeGuidelines: st.IntakeGuidelines,
		Icon:             st.Icon,
		Color:            st.Color,
		IsActive:         st.IsActive,
		CreatedAt:        st.CreatedAt,
		UpdatedAt:        st.UpdatedAt,
	}
}

// toListResponseWithPagination converts a slice of repository ServiceTypes to transport response.
func toListResponseWithPagination(items []repository.ServiceType, total int, page int, pageSize int) transport.ServiceTypeListResponse {
	responses := make([]transport.ServiceTypeResponse, len(items))
	for i, item := range items {
		responses[i] = toResponse(item)
	}
	if pageSize < 1 {
		pageSize = len(items)
	}
	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	return transport.ServiceTypeListResponse{
		Items:      responses,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// generateSlug creates a URL-friendly slug from a name.
func generateSlug(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")

	// Remove special characters (keep only alphanumeric and hyphens)
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	slug = reg.ReplaceAllString(slug, "")

	// Remove multiple consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Trim hyphens from start and end
	slug = strings.Trim(slug, "-")

	return slug
}

type defaultServiceType struct {
	Name             string
	Slug             string
	Description      string
	Icon             string
	Color            string
	IntakeGuidelines string
}

var defaultServiceTypes = []defaultServiceType{
	{
		Name:        "Ramen en deuren",
		Slug:        "ramen-deuren",
		Description: "Plaatsing, vervanging en reparatie van ramen en deuren",
		Icon:        "window",
		Color:       "#3B82F6",
		IntakeGuidelines: `## Benodigde informatie

### Metingen
- Aantal ramen en/of deuren
- Afmetingen per kozijn (hoogte x breedte)
- Verdieping en bereikbaarheid

### Huidige situatie
- Type glas (enkel, dubbel, HR++)
- Materiaal kozijn (hout, kunststof, aluminium)
- Staat van bestaande kozijnen

### Wensen en planning
- Gewenst glastype (HR++, triple)
- Kleur en afwerking
- Gewenste uitvoerdatum

### Budget indicatie
- Richtbudget of bandbreedte`,
	},
	{
		Name:        "Isolatie",
		Slug:        "isolatie",
		Description: "Dak-, vloer- en spouwmuurisolatie voor woningen",
		Icon:        "home",
		Color:       "#10B981",
		IntakeGuidelines: `## Benodigde informatie

### Type isolatie
- Dak, vloer, spouwmuur of gevel
- Beschikbaar oppervlak (m2)
- Huidige isolatielaag (ja/nee, dikte)

### Huidige situatie
- Bouwjaar woning
- Toegang tot dak/vloer/spouw
- Eventuele vochtproblemen

### Wensen en planning
- Voorkeur materiaal (glaswol, PIR, EPS)
- Gewenste uitvoerdatum

### Budget indicatie
- Richtbudget of bandbreedte`,
	},
	{
		Name:        "Zonnepanelen",
		Slug:        "zonnepanelen",
		Description: "Installatie en onderhoud van zonnepanelen",
		Icon:        "sun",
		Color:       "#F59E0B",
		IntakeGuidelines: `## Benodigde informatie

### Dak en ligging
- Type dak (plat, schuin)
- Orientatie (noord, oost, zuid, west)
- Dakhelling en schaduw (bomen/gebouw)

### Techniek
- Huidige meterkast (aantal groepen)
- Beschikbare ruimte voor omvormer
- Gewenst aantal panelen

### Wensen en planning
- Doel (besparing, teruglevering)
- Gewenste uitvoerdatum

### Budget indicatie
- Richtbudget of bandbreedte`,
	},
	{
		Name:        "Loodgieterswerk",
		Slug:        "loodgieterswerk",
		Description: "Reparaties en installatie van leidingen, kranen en afvoer",
		Icon:        "droplet",
		Color:       "#0EA5E9",
		IntakeGuidelines: `## Benodigde informatie

### Probleemomschrijving
- Type klus (lekkage, verstopping, installatie)
- Locatie in de woning
- Ernst en urgentie

### Huidige situatie
- Materiaal leidingen (koper, pvc, staal)
- Bereikbaarheid (vloer, kruipruimte)
- Vorige reparaties

### Wensen en planning
- Gewenste oplossing
- Gewenste uitvoerdatum

### Budget indicatie
- Richtbudget of bandbreedte`,
	},
	{
		Name:        "Verwarming en klimaat",
		Slug:        "verwarming-klimaat",
		Description: "CV, warmtepompen, ventilatie en airco systemen",
		Icon:        "flame",
		Color:       "#EF4444",
		IntakeGuidelines: `## Benodigde informatie

### Type systeem
- CV ketel, warmtepomp, ventilatie of airco
- Gewenst vermogen of capaciteit

### Huidige situatie
- Bestaand systeem en leeftijd
- Woningtype en oppervlakte
- Beschikbare ruimte voor installatie

### Wensen en planning
- Doel (comfort, besparen, vervangen)
- Gewenste uitvoerdatum

### Budget indicatie
- Richtbudget of bandbreedte`,
	},
	{
		Name:        "Elektra",
		Slug:        "elektra",
		Description: "Elektrische installaties, uitbreidingen en reparaties",
		Icon:        "zap",
		Color:       "#8B5CF6",
		IntakeGuidelines: `## Benodigde informatie

### Werkzaamheden
- Type klus (groepenkast, stopcontacten, verlichting)
- Aantal punten of groepen

### Huidige situatie
- Huidige groepenkast (aantal groepen)
- Bekabeling en bereikbaarheid
- Eventuele storingen

### Wensen en planning
- Gewenste functionaliteit
- Gewenste uitvoerdatum

### Budget indicatie
- Richtbudget of bandbreedte`,
	},
	{
		Name:        "Timmerwerk",
		Slug:        "timmerwerk",
		Description: "Houtwerk, deuren, vloeren en maatwerk",
		Icon:        "hammer",
		Color:       "#D97706",
		IntakeGuidelines: `## Benodigde informatie

### Werkomschrijving
- Type project (kast, vloer, trap, kozijn)
- Afmetingen of tekeningen

### Huidige situatie
- Materiaal en staat
- Montageplek en bereikbaarheid

### Wensen en planning
- Gewenste afwerking en kleur
- Gewenste uitvoerdatum

### Budget indicatie
- Richtbudget of bandbreedte`,
	},
	{
		Name:        "Klusbedrijf",
		Slug:        "klusbedrijf",
		Description: "Algemene klussen en kleine verbouwingen",
		Icon:        "tool",
		Color:       "#6B7280",
		IntakeGuidelines: `## Benodigde informatie

### Kluslijst
- Overzicht van alle taken
- Aantal en afmetingen per taak

### Huidige situatie
- Locatie per klus
- Toegang en bereikbaarheid

### Wensen en planning
- Gewenste materialen of afwerking
- Gewenste uitvoerdatum

### Budget indicatie
- Richtbudget of bandbreedte`,
	},
	{
		Name:        "Algemeen",
		Slug:        "algemeen",
		Description: "Algemene aanvragen en niet-gecategoriseerde verzoeken",
		Icon:        "inbox",
		Color:       "#9CA3AF",
		IntakeGuidelines: `## Benodigde informatie

### Probleemomschrijving
- Korte omschrijving van de aanvraag
- Locatie in de woning
- Urgentie

### Basisgegevens
- Afmetingen of aantallen indien relevant
- Fotos indien mogelijk

### Wensen en planning
- Gewenste uitvoerdatum
- Indicatief budget`,
	},
}

func toPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func isDuplicateServiceType(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	return pgErr.ConstraintName == "idx_service_types_org_name" || pgErr.ConstraintName == "idx_service_types_org_slug"
}
