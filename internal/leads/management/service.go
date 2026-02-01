// Package management handles lead CRUD operations.
// This is a vertically sliced feature package containing service logic
// for creating, reading, updating, and deleting leads.
package management

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"portal_final_backend/internal/events"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/maps"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/phone"

	"github.com/google/uuid"
)

const (
	leadNotFoundMsg        = "lead not found"
	leadServiceNotFoundMsg = "lead service not found"
)

// Repository defines the data access interface needed by the management service.
// This is a consumer-driven interface - only what management needs.
type Repository interface {
	repository.LeadReader
	repository.LeadWriter
	repository.LeadViewTracker
	repository.ActivityLogger
	repository.LeadServiceReader
	repository.LeadServiceWriter
	repository.MetricsReader
}

// Service handles lead management operations (CRUD).
type Service struct {
	repo     Repository
	eventBus events.Bus
	maps     *maps.Service
}

// New creates a new lead management service.
func New(repo Repository, eventBus events.Bus, mapsService *maps.Service) *Service {
	return &Service{repo: repo, eventBus: eventBus, maps: mapsService}
}

// Create creates a new lead.
func (s *Service) Create(ctx context.Context, req transport.CreateLeadRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	req.Phone = phone.NormalizeE164(req.Phone)

	params := repository.CreateLeadParams{
		OrganizationID:     tenantID,
		ConsumerFirstName:  req.FirstName,
		ConsumerLastName:   req.LastName,
		ConsumerPhone:      req.Phone,
		ConsumerRole:       string(req.ConsumerRole),
		AddressStreet:      req.Street,
		AddressHouseNumber: req.HouseNumber,
		AddressZipCode:     req.ZipCode,
		AddressCity:        req.City,
		Latitude:           req.Latitude,
		Longitude:          req.Longitude,
		Source:             toPtr(req.Source),
	}

	if req.AssigneeID.Set {
		params.AssignedAgentID = req.AssigneeID.Value
	}

	if req.Email != "" {
		params.ConsumerEmail = &req.Email
	}

	lead, err := s.repo.Create(ctx, params)
	if err != nil {
		return transport.LeadResponse{}, err
	}

	// Create the initial service for the lead
	_, err = s.repo.CreateLeadService(ctx, repository.CreateLeadServiceParams{
		LeadID:         lead.ID,
		OrganizationID: tenantID,
		ServiceType:    string(req.ServiceType),
		ConsumerNote:   toPtr(req.ConsumerNote),
	})
	if err != nil {
		return transport.LeadResponse{}, err
	}

	s.eventBus.Publish(ctx, events.LeadCreated{
		BaseEvent:       events.NewBaseEvent(),
		LeadID:          lead.ID,
		TenantID:        tenantID,
		AssignedAgentID: lead.AssignedAgentID,
		ServiceType:     string(req.ServiceType),
	})

	services, _ := s.repo.ListLeadServices(ctx, lead.ID, tenantID)
	return ToLeadResponseWithServices(lead, services), nil
}

// GetByID retrieves a lead by ID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (transport.LeadResponse, error) {
	lead, services, err := s.repo.GetByIDWithServices(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	return ToLeadResponseWithServices(lead, services), nil
}

// Update updates a lead's information.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req transport.UpdateLeadRequest, actorID uuid.UUID, tenantID uuid.UUID, actorRoles []string) (transport.LeadResponse, error) {
	params, current, err := s.prepareAssigneeUpdate(ctx, id, tenantID, req, actorID, actorRoles)
	if err != nil {
		return transport.LeadResponse{}, err
	}

	addressUpdateRequested, err := s.applyAddressGeocode(ctx, id, tenantID, req, &params, &current)
	if err != nil {
		return transport.LeadResponse{}, err
	}

	applyUpdateFields(&params, req, !addressUpdateRequested)

	lead, err := s.repo.Update(ctx, id, tenantID, params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if req.AssigneeID.Set && current != nil {
		if !equalUUIDPtrs(current.AssignedAgentID, req.AssigneeID.Value) {
			_ = s.repo.AddActivity(ctx, id, tenantID, actorID, "assigned", map[string]interface{}{
				"from": current.AssignedAgentID,
				"to":   req.AssigneeID.Value,
			})
		}
	}

	services, _ := s.repo.ListLeadServices(ctx, lead.ID, tenantID)
	return ToLeadResponseWithServices(lead, services), nil
}

// Delete soft-deletes a lead.
func (s *Service) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	err := s.repo.Delete(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound(leadNotFoundMsg)
		}
		return err
	}
	return nil
}

// BulkDelete deletes multiple leads.
func (s *Service) BulkDelete(ctx context.Context, ids []uuid.UUID, tenantID uuid.UUID) (int, error) {
	deletedCount, err := s.repo.BulkDelete(ctx, ids, tenantID)
	if err != nil {
		return 0, err
	}
	if deletedCount == 0 {
		return 0, apperr.NotFound("no leads found to delete")
	}
	return deletedCount, nil
}

// List retrieves a paginated list of leads.
func (s *Service) List(ctx context.Context, req transport.ListLeadsRequest, tenantID uuid.UUID) (transport.LeadListResponse, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	params, err := buildListParams(req)
	if err != nil {
		return transport.LeadListResponse{}, err
	}
	params.OrganizationID = tenantID

	leads, total, err := s.repo.List(ctx, params)
	if err != nil {
		return transport.LeadListResponse{}, err
	}

	items := make([]transport.LeadResponse, len(leads))
	for i, lead := range leads {
		services, _ := s.repo.ListLeadServices(ctx, lead.ID, tenantID)
		items[i] = ToLeadResponseWithServices(lead, services)
	}

	totalPages := (total + req.PageSize - 1) / req.PageSize

	return transport.LeadListResponse{
		Items:      items,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	}, nil
}

func buildListParams(req transport.ListLeadsRequest) (repository.ListParams, error) {
	params := repository.ListParams{
		Search:    req.Search,
		Offset:    (req.Page - 1) * req.PageSize,
		Limit:     req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	if req.Status != nil {
		status := string(*req.Status)
		params.Status = &status
	}
	if req.ServiceType != nil {
		serviceType := string(*req.ServiceType)
		params.ServiceType = &serviceType
	}

	params.FirstName = optionalString(req.FirstName)
	params.LastName = optionalString(req.LastName)
	params.Phone = optionalString(req.Phone)
	params.Email = optionalString(req.Email)
	if req.Role != nil {
		role := string(*req.Role)
		params.Role = &role
	}
	params.Street = optionalString(req.Street)
	params.HouseNumber = optionalString(req.HouseNumber)
	params.ZipCode = optionalString(req.ZipCode)
	params.City = optionalString(req.City)
	params.AssignedAgentID = req.AssignedAgentID

	createdFrom, createdTo, err := parseDateRange(req.CreatedAtFrom, req.CreatedAtTo)
	if err != nil {
		return repository.ListParams{}, apperr.Validation(err.Error())
	}
	params.CreatedAtFrom = createdFrom
	params.CreatedAtTo = createdTo

	return params, nil
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

func parseDateRange(from string, to string) (*time.Time, *time.Time, error) {
	const dateLayout = "2006-01-02"

	var start *time.Time
	var end *time.Time

	if from != "" {
		parsed, err := time.Parse(dateLayout, from)
		if err != nil {
			return nil, nil, err
		}
		start = &parsed
	}

	if to != "" {
		parsed, err := time.Parse(dateLayout, to)
		if err != nil {
			return nil, nil, err
		}
		endExclusive := parsed.AddDate(0, 0, 1)
		end = &endExclusive
	}

	if start != nil && end != nil && start.After(*end) {
		return nil, nil, errors.New("createdAtFrom must be before createdAtTo")
	}

	return start, end, nil
}

// CheckDuplicate checks if a lead with the given phone already exists.
func (s *Service) CheckDuplicate(ctx context.Context, phoneNumber string, tenantID uuid.UUID) (transport.DuplicateCheckResponse, error) {
	lead, err := s.repo.GetByPhone(ctx, phoneNumber, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.DuplicateCheckResponse{IsDuplicate: false}, nil
		}
		return transport.DuplicateCheckResponse{}, err
	}

	resp := ToLeadResponse(lead)
	return transport.DuplicateCheckResponse{
		IsDuplicate:  true,
		ExistingLead: &resp,
	}, nil
}

// CheckReturningCustomer checks if a lead with the given phone or email already exists.
// This is used to detect returning customers when creating a new service request.
func (s *Service) CheckReturningCustomer(ctx context.Context, phoneNumber string, email string, tenantID uuid.UUID) (transport.ReturningCustomerResponse, error) {
	normalizedPhone := phone.NormalizeE164(phoneNumber)

	summary, services, err := s.repo.GetByPhoneOrEmail(ctx, normalizedPhone, email, tenantID)
	if err != nil {
		return transport.ReturningCustomerResponse{}, err
	}

	if summary == nil {
		return transport.ReturningCustomerResponse{Found: false}, nil
	}

	serviceBriefs := make([]transport.ServiceBrief, len(services))
	for i, svc := range services {
		serviceBriefs[i] = transport.ServiceBrief{
			ServiceType: transport.ServiceType(svc.ServiceType),
			Status:      transport.LeadStatus(svc.Status),
			CreatedAt:   svc.CreatedAt,
		}
	}

	return transport.ReturningCustomerResponse{
		Found:         true,
		LeadID:        &summary.ID,
		FullName:      summary.ConsumerName,
		TotalServices: summary.ServiceCount,
		Services:      serviceBriefs,
	}, nil
}

// Assign assigns or unassigns a lead to an agent.
func (s *Service) Assign(ctx context.Context, id uuid.UUID, assigneeID *uuid.UUID, actorID uuid.UUID, tenantID uuid.UUID, actorRoles []string) (transport.LeadResponse, error) {
	current, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if !hasRole(actorRoles, "admin") {
		if current.AssignedAgentID == nil || *current.AssignedAgentID != actorID {
			return transport.LeadResponse{}, apperr.Forbidden("forbidden")
		}
	}

	params := repository.UpdateLeadParams{
		AssignedAgentID:    assigneeID,
		AssignedAgentIDSet: true,
	}
	updated, err := s.repo.Update(ctx, id, tenantID, params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	_ = s.repo.AddActivity(ctx, id, tenantID, actorID, "assigned", map[string]interface{}{
		"from": current.AssignedAgentID,
		"to":   assigneeID,
	})

	return ToLeadResponse(updated), nil
}

// AssignIfUnassigned assigns a lead to the agent if it is currently unassigned.
func (s *Service) AssignIfUnassigned(ctx context.Context, id uuid.UUID, agentID uuid.UUID, tenantID uuid.UUID) error {
	lead, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return apperr.NotFound(leadNotFoundMsg)
		}
		return err
	}

	if lead.AssignedAgentID != nil {
		return apperr.Forbidden("lead is already assigned")
	}

	params := repository.UpdateLeadParams{
		AssignedAgentID:    &agentID,
		AssignedAgentIDSet: true,
	}

	if _, err := s.repo.Update(ctx, id, tenantID, params); err != nil {
		return err
	}

	_ = s.repo.AddActivity(ctx, id, tenantID, agentID, "assigned", map[string]interface{}{
		"from": nil,
		"to":   agentID,
	})

	return nil
}

// SetViewedBy marks a lead as viewed by a user.
func (s *Service) SetViewedBy(ctx context.Context, id uuid.UUID, userID uuid.UUID, tenantID uuid.UUID) error {
	return s.repo.SetViewedBy(ctx, id, tenantID, userID)
}

// GetLeadServiceByID retrieves a lead service by its ID.
func (s *Service) GetLeadServiceByID(ctx context.Context, serviceID uuid.UUID, tenantID uuid.UUID) (repository.LeadService, error) {
	svc, err := s.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return repository.LeadService{}, apperr.NotFound(leadServiceNotFoundMsg)
		}
		return repository.LeadService{}, err
	}
	return svc, nil
}

// AddService adds a new service to an existing lead.
func (s *Service) AddService(ctx context.Context, leadID uuid.UUID, req transport.AddServiceRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	lead, err := s.repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if req.CloseCurrentStatus {
		if err := s.repo.CloseAllActiveServices(ctx, leadID, tenantID); err != nil {
			return transport.LeadResponse{}, err
		}
	}

	_, err = s.repo.CreateLeadService(ctx, repository.CreateLeadServiceParams{
		LeadID:         leadID,
		OrganizationID: tenantID,
		ServiceType:    string(req.ServiceType),
		ConsumerNote:   toPtr(req.ConsumerNote),
		Source:         toPtr(req.Source),
	})
	if err != nil {
		return transport.LeadResponse{}, err
	}

	services, _ := s.repo.ListLeadServices(ctx, leadID, tenantID)
	return ToLeadResponseWithServices(lead, services), nil
}

// UpdateServiceStatus updates the status of a specific service.
func (s *Service) UpdateServiceStatus(ctx context.Context, leadID uuid.UUID, serviceID uuid.UUID, req transport.UpdateServiceStatusRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	svc, err := s.repo.GetLeadServiceByID(ctx, serviceID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadServiceNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}
	if svc.LeadID != leadID {
		return transport.LeadResponse{}, apperr.NotFound(leadServiceNotFoundMsg)
	}

	_, err = s.repo.UpdateServiceStatus(ctx, serviceID, tenantID, string(req.Status))
	if err != nil {
		return transport.LeadResponse{}, err
	}

	return s.GetByID(ctx, leadID, tenantID)
}

// UpdateStatus updates the status of the lead's current service.
func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, req transport.UpdateLeadStatusRequest, tenantID uuid.UUID) (transport.LeadResponse, error) {
	service, err := s.repo.GetCurrentLeadService(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) || errors.Is(err, repository.ErrNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	if _, err := s.repo.UpdateServiceStatus(ctx, service.ID, tenantID, string(req.Status)); err != nil {
		if errors.Is(err, repository.ErrServiceNotFound) {
			return transport.LeadResponse{}, apperr.NotFound(leadNotFoundMsg)
		}
		return transport.LeadResponse{}, err
	}

	return s.GetByID(ctx, id, tenantID)
}

// GetMetrics returns aggregated KPI metrics for the dashboard.
func (s *Service) GetMetrics(ctx context.Context, tenantID uuid.UUID) (transport.LeadMetricsResponse, error) {
	metrics, err := s.repo.GetMetrics(ctx, tenantID)
	if err != nil {
		return transport.LeadMetricsResponse{}, err
	}

	var disqualifiedRate float64
	var touchpointsPerLead float64
	if metrics.TotalLeads > 0 {
		disqualifiedRate = float64(metrics.DisqualifiedLeads) / float64(metrics.TotalLeads)
		touchpointsPerLead = float64(metrics.Touchpoints) / float64(metrics.TotalLeads)
	}

	return transport.LeadMetricsResponse{
		TotalLeads:          metrics.TotalLeads,
		ProjectedValueCents: metrics.ProjectedValueCents,
		DisqualifiedRate:    roundToOneDecimal(disqualifiedRate * 100),
		TouchpointsPerLead:  roundToOneDecimal(touchpointsPerLead),
	}, nil
}

// GetHeatmap returns geocoded lead points for the dashboard heatmap.
func (s *Service) GetHeatmap(ctx context.Context, startDate *time.Time, endDate *time.Time, tenantID uuid.UUID) (transport.LeadHeatmapResponse, error) {
	var endExclusive *time.Time
	if endDate != nil {
		end := endDate.AddDate(0, 0, 1)
		endExclusive = &end
	}

	points, err := s.repo.ListHeatmapPoints(ctx, tenantID, startDate, endExclusive)
	if err != nil {
		return transport.LeadHeatmapResponse{}, err
	}

	resp := transport.LeadHeatmapResponse{Points: make([]transport.LeadHeatmapPointResponse, 0, len(points))}
	for _, point := range points {
		resp.Points = append(resp.Points, transport.LeadHeatmapPointResponse{
			Latitude:  point.Latitude,
			Longitude: point.Longitude,
		})
	}

	return resp, nil
}

// GetActionItems returns urgent or recent leads for the dashboard widget.
func (s *Service) GetActionItems(ctx context.Context, page int, pageSize int, newLeadDays int, tenantID uuid.UUID) (transport.ActionItemsResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 5
	}
	if pageSize > 50 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize
	result, err := s.repo.ListActionItems(ctx, tenantID, newLeadDays, pageSize, offset)
	if err != nil {
		return transport.ActionItemsResponse{}, err
	}

	items := make([]transport.ActionItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		name := strings.TrimSpace(item.FirstName + " " + item.LastName)
		isUrgent := item.UrgencyLevel != nil && *item.UrgencyLevel == "High"
		items = append(items, transport.ActionItemResponse{
			ID:            item.ID,
			Name:          name,
			UrgencyReason: item.UrgencyReason,
			CreatedAt:     item.CreatedAt,
			IsUrgent:      isUrgent,
		})
	}

	return transport.ActionItemsResponse{
		Items:    items,
		Total:    result.Total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func roundToOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

func (s *Service) prepareAssigneeUpdate(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, req transport.UpdateLeadRequest, actorID uuid.UUID, actorRoles []string) (repository.UpdateLeadParams, *repository.Lead, error) {
	params := repository.UpdateLeadParams{}
	if !req.AssigneeID.Set {
		return params, nil, nil
	}

	lead, err := s.repo.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.UpdateLeadParams{}, nil, apperr.NotFound(leadNotFoundMsg)
		}
		return repository.UpdateLeadParams{}, nil, err
	}

	if !hasRole(actorRoles, "admin") {
		if lead.AssignedAgentID == nil || *lead.AssignedAgentID != actorID {
			return repository.UpdateLeadParams{}, nil, apperr.Forbidden("forbidden")
		}
	}

	params.AssignedAgentID = req.AssigneeID.Value
	params.AssignedAgentIDSet = true
	return params, &lead, nil
}

func applyUpdateFields(params *repository.UpdateLeadParams, req transport.UpdateLeadRequest, applyCoords bool) {
	if req.FirstName != nil {
		params.ConsumerFirstName = req.FirstName
	}
	if req.LastName != nil {
		params.ConsumerLastName = req.LastName
	}
	if req.Phone != nil {
		normalized := phone.NormalizeE164(*req.Phone)
		params.ConsumerPhone = &normalized
	}
	if req.Email != nil {
		params.ConsumerEmail = req.Email
	}
	if applyCoords {
		if req.Latitude != nil {
			params.Latitude = req.Latitude
		}
		if req.Longitude != nil {
			params.Longitude = req.Longitude
		}
	}
	if req.ConsumerRole != nil {
		role := string(*req.ConsumerRole)
		params.ConsumerRole = &role
	}
	if req.Street != nil {
		params.AddressStreet = req.Street
	}
	if req.HouseNumber != nil {
		params.AddressHouseNumber = req.HouseNumber
	}
	if req.ZipCode != nil {
		params.AddressZipCode = req.ZipCode
	}
	if req.City != nil {
		params.AddressCity = req.City
	}
}

func (s *Service) applyAddressGeocode(ctx context.Context, id uuid.UUID, tenantID uuid.UUID, req transport.UpdateLeadRequest, params *repository.UpdateLeadParams, current **repository.Lead) (bool, error) {
	if !hasAddressUpdate(req) {
		return false, nil
	}

	if *current == nil {
		lead, err := s.repo.GetByID(ctx, id, tenantID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return true, apperr.NotFound(leadNotFoundMsg)
			}
			return true, err
		}
		*current = &lead
	}

	updatedAddress, changed := buildUpdatedAddress(**current, req)
	if changed {
		if lat, lon, ok := s.geocodeAddress(ctx, updatedAddress); ok {
			params.Latitude = &lat
			params.Longitude = &lon
		}
	}

	return true, nil
}

type addressUpdate struct {
	street      string
	houseNumber string
	zipCode     string
	city        string
}

func hasAddressUpdate(req transport.UpdateLeadRequest) bool {
	return req.Street != nil || req.HouseNumber != nil || req.ZipCode != nil || req.City != nil
}

func buildUpdatedAddress(current repository.Lead, req transport.UpdateLeadRequest) (addressUpdate, bool) {
	updated := addressUpdate{
		street:      current.AddressStreet,
		houseNumber: current.AddressHouseNumber,
		zipCode:     current.AddressZipCode,
		city:        current.AddressCity,
	}

	changed := false
	if req.Street != nil {
		updated.street = strings.TrimSpace(*req.Street)
		changed = changed || updated.street != current.AddressStreet
	}
	if req.HouseNumber != nil {
		updated.houseNumber = strings.TrimSpace(*req.HouseNumber)
		changed = changed || updated.houseNumber != current.AddressHouseNumber
	}
	if req.ZipCode != nil {
		updated.zipCode = strings.TrimSpace(*req.ZipCode)
		changed = changed || updated.zipCode != current.AddressZipCode
	}
	if req.City != nil {
		updated.city = strings.TrimSpace(*req.City)
		changed = changed || updated.city != current.AddressCity
	}

	return updated, changed
}

func (s *Service) geocodeAddress(ctx context.Context, address addressUpdate) (float64, float64, bool) {
	if s.maps == nil {
		return 0, 0, false
	}

	if address.street == "" || address.city == "" {
		return 0, 0, false
	}

	query := formatGeocodeQuery(address)
	suggestions, err := s.maps.SearchAddress(ctx, query)
	if err != nil || len(suggestions) == 0 {
		return 0, 0, false
	}

	lat, err := strconv.ParseFloat(suggestions[0].Lat, 64)
	if err != nil {
		return 0, 0, false
	}
	lon, err := strconv.ParseFloat(suggestions[0].Lon, 64)
	if err != nil {
		return 0, 0, false
	}

	return lat, lon, true
}

func formatGeocodeQuery(address addressUpdate) string {
	streetPart := strings.TrimSpace(strings.Join([]string{address.street, address.houseNumber}, " "))
	cityPart := strings.TrimSpace(strings.Join([]string{address.zipCode, address.city}, " "))
	query := strings.TrimSpace(fmt.Sprintf("%s, %s", streetPart, cityPart))
	return strings.Trim(query, ", ")
}

func hasRole(roles []string, target string) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func equalUUIDPtrs(a *uuid.UUID, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func toPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
