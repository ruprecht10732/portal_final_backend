package agent

import (
	"errors"
	"fmt"
	"log"
	"portal_final_backend/internal/leads/domain"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/phone"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
)

func handleUpdateLeadServiceType(ctx tool.Context, deps *ToolDependencies, input UpdateLeadServiceTypeInput) (UpdateLeadServiceTypeOutput, error) {
	leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: invalidLeadIDMessage}, err
	}
	leadServiceID, err := parseUUID(input.LeadServiceID, invalidLeadServiceIDMessage)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: invalidLeadServiceIDMessage}, err
	}
	serviceType := strings.TrimSpace(input.ServiceType)
	if serviceType == "" {
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Missing service type"}, fmt.Errorf("missing service type")
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	leadService, err := deps.Repo.GetLeadServiceByID(ctx, leadServiceID, tenantID)
	if err != nil {
		return UpdateLeadServiceTypeOutput{Success: false, Message: leadServiceNotFoundMessage}, err
	}
	if leadService.LeadID != leadID {
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Lead service does not belong to lead"}, fmt.Errorf("lead service mismatch")
	}

	// Stability guard: service type changes are only allowed during initial triage.
	// Gatekeeper re-runs on many changes (notes/attachments); without this guard the
	// LLM can "flip-flop" service type on ambiguous new info.
	if leadService.PipelineStage != domain.PipelineStageTriage {
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Service type is locked after Triage"}, nil
	}

	_, err = deps.Repo.UpdateLeadServiceType(ctx, leadServiceID, tenantID, serviceType)
	if err != nil {
		if errors.Is(err, repository.ErrServiceTypeNotFound) {
			return UpdateLeadServiceTypeOutput{Success: false, Message: "Service type not found or inactive"}, nil
		}
		return UpdateLeadServiceTypeOutput{Success: false, Message: "Failed to update service type"}, err
	}

	actorType, actorName := deps.GetActor()
	reasonText := strings.TrimSpace(input.Reason)
	if reasonText == "" {
		reasonText = "Diensttype aangepast"
	}
	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      &leadServiceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      repository.EventTypeServiceTypeChange,
		Title:          repository.EventTitleServiceTypeUpdated,
		Summary:        &reasonText,
		Metadata: repository.ServiceTypeChangeMetadata{
			OldServiceType: leadService.ServiceType,
			NewServiceType: serviceType,
			Reason:         input.Reason,
		}.ToMap(),
	})

	log.Printf(
		"gatekeeper UpdateLeadServiceType: leadId=%s serviceId=%s from=%s to=%s",
		leadID,
		leadServiceID,
		leadService.ServiceType,
		serviceType,
	)

	return UpdateLeadServiceTypeOutput{Success: true, Message: "Service type updated"}, nil
}

// leadDetailsBuilder encapsulates field update logic for handleUpdateLeadDetails
type leadDetailsBuilder struct {
	params        repository.UpdateLeadParams
	updatedFields []string
}

func newLeadDetailsBuilder() *leadDetailsBuilder {
	return &leadDetailsBuilder{
		params:        repository.UpdateLeadParams{},
		updatedFields: make([]string, 0, 10),
	}
}

func (b *leadDetailsBuilder) setStringField(input *string, current string, fieldName string, setter func(*string)) error {
	if input == nil {
		return nil
	}
	value := strings.TrimSpace(*input)
	if value == "" {
		return fmt.Errorf(invalidFieldFormat, fieldName)
	}
	setter(&value)
	if value != current {
		b.updatedFields = append(b.updatedFields, fieldName)
	}
	return nil
}

func (b *leadDetailsBuilder) setOptionalStringField(input *string, current *string, fieldName string, setter func(*string)) error {
	if input == nil {
		return nil
	}
	value := strings.TrimSpace(*input)
	if value == "" {
		return fmt.Errorf(invalidFieldFormat, fieldName)
	}
	setter(&value)
	if current == nil || *current != value {
		b.updatedFields = append(b.updatedFields, fieldName)
	}
	return nil
}

func (b *leadDetailsBuilder) setPhoneField(input *string, current string) error {
	if input == nil {
		return nil
	}
	value := phone.NormalizeE164(strings.TrimSpace(*input))
	if value == "" {
		return fmt.Errorf("invalid phone")
	}
	b.params.ConsumerPhone = &value
	if value != current {
		b.updatedFields = append(b.updatedFields, "phone")
	}
	return nil
}

func (b *leadDetailsBuilder) setConsumerRole(input *string, current string) error {
	if input == nil {
		return nil
	}
	role, err := normalizeConsumerRole(*input)
	if err != nil {
		return fmt.Errorf("invalid consumer role")
	}
	b.params.ConsumerRole = &role
	if role != current {
		b.updatedFields = append(b.updatedFields, "consumerRole")
	}
	return nil
}

func (b *leadDetailsBuilder) setAssignee(input *string, current *uuid.UUID) error {
	if input == nil {
		return nil
	}
	value := strings.TrimSpace(*input)
	if value == "" {
		return fmt.Errorf("invalid assigneeId")
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid assigneeId")
	}
	b.params.AssignedAgentID = &parsed
	b.params.AssignedAgentIDSet = true
	if current == nil || *current != parsed {
		b.updatedFields = append(b.updatedFields, "assigneeId")
	}
	return nil
}

func (b *leadDetailsBuilder) setCoordinate(input *float64, current *float64, fieldName string, min, max float64, setter func(*float64)) error {
	if input == nil {
		return nil
	}
	if *input < min || *input > max {
		return fmt.Errorf(invalidFieldFormat, fieldName)
	}
	setter(input)
	if current == nil || *current != *input {
		b.updatedFields = append(b.updatedFields, fieldName)
	}
	return nil
}

func (b *leadDetailsBuilder) setWhatsAppOptedIn(input *bool, current bool) {
	if input == nil {
		return
	}
	b.params.WhatsAppOptedIn = input
	b.params.WhatsAppOptedInSet = true
	if *input != current {
		b.updatedFields = append(b.updatedFields, "whatsAppOptedIn")
	}
}

func (b *leadDetailsBuilder) buildFromInput(input UpdateLeadDetailsInput, current repository.Lead) error {
	if err := b.setStringField(input.FirstName, current.ConsumerFirstName, "firstName", func(v *string) { b.params.ConsumerFirstName = v }); err != nil {
		return err
	}
	if err := b.setStringField(input.LastName, current.ConsumerLastName, "lastName", func(v *string) { b.params.ConsumerLastName = v }); err != nil {
		return err
	}
	if err := b.setPhoneField(input.Phone, current.ConsumerPhone); err != nil {
		return err
	}
	if err := b.setOptionalStringField(input.Email, current.ConsumerEmail, "email", func(v *string) { b.params.ConsumerEmail = v }); err != nil {
		return err
	}
	if err := b.setAssignee(input.AssigneeID, current.AssignedAgentID); err != nil {
		return err
	}
	if err := b.setConsumerRole(input.ConsumerRole, current.ConsumerRole); err != nil {
		return err
	}
	if err := b.setStringField(input.Street, current.AddressStreet, "street", func(v *string) { b.params.AddressStreet = v }); err != nil {
		return err
	}
	if err := b.setStringField(input.HouseNumber, current.AddressHouseNumber, "houseNumber", func(v *string) { b.params.AddressHouseNumber = v }); err != nil {
		return err
	}
	if err := b.setStringField(input.ZipCode, current.AddressZipCode, "zipCode", func(v *string) { b.params.AddressZipCode = v }); err != nil {
		return err
	}
	if err := b.setStringField(input.City, current.AddressCity, "city", func(v *string) { b.params.AddressCity = v }); err != nil {
		return err
	}
	if err := b.setCoordinate(input.Latitude, current.Latitude, "latitude", -90, 90, func(v *float64) { b.params.Latitude = v }); err != nil {
		return err
	}
	if err := b.setCoordinate(input.Longitude, current.Longitude, "longitude", -180, 180, func(v *float64) { b.params.Longitude = v }); err != nil {
		return err
	}
	b.setWhatsAppOptedIn(input.WhatsAppOptedIn, current.WhatsAppOptedIn)
	return nil
}

func handleUpdateLeadDetails(ctx tool.Context, deps *ToolDependencies, input UpdateLeadDetailsInput) (UpdateLeadDetailsOutput, error) {
	leadID, err := parseUUID(input.LeadID, invalidLeadIDMessage)
	if err != nil {
		return UpdateLeadDetailsOutput{Success: false, Message: invalidLeadIDMessage}, err
	}

	tenantID, err := getTenantID(deps)
	if err != nil {
		return UpdateLeadDetailsOutput{Success: false, Message: missingTenantContextMessage}, err
	}

	current, err := deps.Repo.GetByID(ctx, leadID, tenantID)
	if err != nil {
		return UpdateLeadDetailsOutput{Success: false, Message: leadNotFoundMessage}, err
	}

	builder := newLeadDetailsBuilder()
	if err := builder.buildFromInput(input, current); err != nil {
		return UpdateLeadDetailsOutput{Success: false, Message: err.Error()}, err
	}

	if len(builder.updatedFields) == 0 {
		return UpdateLeadDetailsOutput{Success: true, Message: "No updates required"}, nil
	}

	_, err = deps.Repo.Update(ctx, leadID, tenantID, builder.params)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return UpdateLeadDetailsOutput{Success: false, Message: leadNotFoundMessage}, err
		}
		return UpdateLeadDetailsOutput{Success: false, Message: "Failed to update lead"}, err
	}

	recordLeadDetailsUpdate(ctx, deps, leadID, tenantID, builder.updatedFields, input.Reason, input.Confidence)
	return UpdateLeadDetailsOutput{Success: true, Message: "Lead updated", UpdatedFields: builder.updatedFields}, nil
}

func recordLeadDetailsUpdate(ctx tool.Context, deps *ToolDependencies, leadID, tenantID uuid.UUID, updatedFields []string, reason string, confidence *float64) {
	actorType, actorName := deps.GetActor()
	reasonText := strings.TrimSpace(reason)
	if reasonText == "" {
		reasonText = "Leadgegevens bijgewerkt"
	}

	var serviceID *uuid.UUID
	if _, svcID, ok := deps.GetLeadContext(); ok {
		serviceID = &svcID
	}

	_, _ = deps.Repo.CreateTimelineEvent(ctx, repository.CreateTimelineEventParams{
		LeadID:         leadID,
		ServiceID:      serviceID,
		OrganizationID: tenantID,
		ActorType:      actorType,
		ActorName:      actorName,
		EventType:      repository.EventTypeLeadUpdate,
		Title:          repository.EventTitleLeadDetailsUpdated,
		Summary:        &reasonText,
		Metadata: repository.LeadUpdateMetadata{
			UpdatedFields: updatedFields,
			Confidence:    confidence,
		}.ToMap(),
	})

	log.Printf("gatekeeper UpdateLeadDetails: leadId=%s fields=%v reason=%s", leadID, updatedFields, reasonText)
}
