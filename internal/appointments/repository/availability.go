package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	appointmentsdb "portal_final_backend/internal/appointments/db"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AvailabilityRule struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	Weekday        int
	StartTime      time.Time
	EndTime        time.Time
	Timezone       string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AvailabilityOverride struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	Date           time.Time
	IsAvailable    bool
	StartTime      *time.Time
	EndTime        *time.Time
	Timezone       string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func timeOfDayFromPg(value pgtype.Time) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	microsPerHour := int64(time.Hour / time.Microsecond)
	microsPerMinute := int64(time.Minute / time.Microsecond)
	microsPerSecond := int64(time.Second / time.Microsecond)

	hours := value.Microseconds / microsPerHour
	remaining := value.Microseconds % microsPerHour
	minutes := remaining / microsPerMinute
	remaining %= microsPerMinute
	seconds := remaining / microsPerSecond
	microseconds := remaining % microsPerSecond

	return time.Date(1, time.January, 1, int(hours), int(minutes), int(seconds), int(microseconds)*1000, time.UTC)
}

func optionalTimeOfDayFromPg(value pgtype.Time) *time.Time {
	if !value.Valid {
		return nil
	}
	timeOfDay := timeOfDayFromPg(value)
	return &timeOfDay
}

func availabilityRuleFromModel(model appointmentsdb.RacAppointmentAvailabilityRule) AvailabilityRule {
	return AvailabilityRule{
		ID:             uuid.UUID(model.ID.Bytes),
		OrganizationID: uuid.UUID(model.OrganizationID.Bytes),
		UserID:         uuid.UUID(model.UserID.Bytes),
		Weekday:        int(model.Weekday),
		StartTime:      timeOfDayFromPg(model.StartTime),
		EndTime:        timeOfDayFromPg(model.EndTime),
		Timezone:       model.Timezone,
		CreatedAt:      model.CreatedAt.Time,
		UpdatedAt:      model.UpdatedAt.Time,
	}
}

func availabilityOverrideFromModel(model appointmentsdb.RacAppointmentAvailabilityOverride) AvailabilityOverride {
	return AvailabilityOverride{
		ID:             uuid.UUID(model.ID.Bytes),
		OrganizationID: uuid.UUID(model.OrganizationID.Bytes),
		UserID:         uuid.UUID(model.UserID.Bytes),
		Date:           model.Date.Time,
		IsAvailable:    model.IsAvailable,
		StartTime:      optionalTimeOfDayFromPg(model.StartTime),
		EndTime:        optionalTimeOfDayFromPg(model.EndTime),
		Timezone:       model.Timezone,
		CreatedAt:      model.CreatedAt.Time,
		UpdatedAt:      model.UpdatedAt.Time,
	}
}

func (r *Repository) CreateAvailabilityRule(ctx context.Context, rule AvailabilityRule) (*AvailabilityRule, error) {
	row, err := r.queries.CreateAvailabilityRule(ctx, appointmentsdb.CreateAvailabilityRuleParams{
		ID:             toPgUUID(rule.ID),
		OrganizationID: toPgUUID(rule.OrganizationID),
		UserID:         toPgUUID(rule.UserID),
		Weekday:        int16(rule.Weekday),
		StartTime:      toPgTimeOfDay(rule.StartTime),
		EndTime:        toPgTimeOfDay(rule.EndTime),
		Timezone:       rule.Timezone,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create availability rule: %w", err)
	}

	saved := availabilityRuleFromModel(appointmentsdb.RacAppointmentAvailabilityRule{ID: row.ID, UserID: row.UserID, Weekday: row.Weekday, StartTime: row.StartTime, EndTime: row.EndTime, Timezone: row.Timezone, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID})
	return &saved, nil
}

func (r *Repository) ListAvailabilityRules(ctx context.Context, organizationID uuid.UUID, userID uuid.UUID) ([]AvailabilityRule, error) {
	rows, err := r.queries.ListAvailabilityRules(ctx, appointmentsdb.ListAvailabilityRulesParams{
		OrganizationID: toPgUUID(organizationID),
		UserID:         toPgUUID(userID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list availability rules: %w", err)
	}

	items := make([]AvailabilityRule, 0, len(rows))
	for _, row := range rows {
		items = append(items, availabilityRuleFromModel(appointmentsdb.RacAppointmentAvailabilityRule{ID: row.ID, UserID: row.UserID, Weekday: row.Weekday, StartTime: row.StartTime, EndTime: row.EndTime, Timezone: row.Timezone, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID}))
	}

	return items, nil
}

func (r *Repository) ListAvailabilityRuleUserIDs(ctx context.Context, organizationID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.queries.ListAvailabilityRuleUserIDs(ctx, toPgUUID(organizationID))
	if err != nil {
		return nil, fmt.Errorf("failed to list availability rule users: %w", err)
	}

	items := make([]uuid.UUID, 0, len(rows))
	for _, id := range rows {
		items = append(items, uuid.UUID(id.Bytes))
	}

	return items, nil
}

func (r *Repository) GetAvailabilityRuleByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (*AvailabilityRule, error) {
	row, err := r.queries.GetAvailabilityRuleByID(ctx, appointmentsdb.GetAvailabilityRuleByIDParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("availability rule not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get availability rule: %w", err)
	}

	item := availabilityRuleFromModel(appointmentsdb.RacAppointmentAvailabilityRule{ID: row.ID, UserID: row.UserID, Weekday: row.Weekday, StartTime: row.StartTime, EndTime: row.EndTime, Timezone: row.Timezone, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID})
	return &item, nil
}

func (r *Repository) DeleteAvailabilityRule(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	_, err := r.queries.DeleteAvailabilityRule(ctx, appointmentsdb.DeleteAvailabilityRuleParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return fmt.Errorf("failed to delete availability rule: %w", err)
	}
	return nil
}

func (r *Repository) UpdateAvailabilityRule(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, rule AvailabilityRule) (*AvailabilityRule, error) {
	row, err := r.queries.UpdateAvailabilityRule(ctx, appointmentsdb.UpdateAvailabilityRuleParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
		Weekday:        int16(rule.Weekday),
		StartTime:      toPgTimeOfDay(rule.StartTime),
		EndTime:        toPgTimeOfDay(rule.EndTime),
		Timezone:       rule.Timezone,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("availability rule not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update availability rule: %w", err)
	}

	saved := availabilityRuleFromModel(appointmentsdb.RacAppointmentAvailabilityRule{ID: row.ID, UserID: row.UserID, Weekday: row.Weekday, StartTime: row.StartTime, EndTime: row.EndTime, Timezone: row.Timezone, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID})
	return &saved, nil
}

func (r *Repository) CreateAvailabilityOverride(ctx context.Context, override AvailabilityOverride) (*AvailabilityOverride, error) {
	row, err := r.queries.CreateAvailabilityOverride(ctx, appointmentsdb.CreateAvailabilityOverrideParams{
		ID:             toPgUUID(override.ID),
		OrganizationID: toPgUUID(override.OrganizationID),
		UserID:         toPgUUID(override.UserID),
		Date:           toPgDate(override.Date),
		IsAvailable:    override.IsAvailable,
		StartTime:      toPgTimeOfDayPtr(override.StartTime),
		EndTime:        toPgTimeOfDayPtr(override.EndTime),
		Timezone:       override.Timezone,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create availability override: %w", err)
	}

	saved := availabilityOverrideFromModel(appointmentsdb.RacAppointmentAvailabilityOverride{ID: row.ID, UserID: row.UserID, Date: row.Date, IsAvailable: row.IsAvailable, StartTime: row.StartTime, EndTime: row.EndTime, Timezone: row.Timezone, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID})
	return &saved, nil
}

func (r *Repository) ListAvailabilityOverrides(ctx context.Context, organizationID uuid.UUID, userID uuid.UUID, startDate *time.Time, endDate *time.Time) ([]AvailabilityOverride, error) {
	rows, err := r.queries.ListAvailabilityOverrides(ctx, appointmentsdb.ListAvailabilityOverridesParams{
		OrganizationID: toPgUUID(organizationID),
		UserID:         toPgUUID(userID),
		Column3:        toPgDatePtr(startDate),
		Column4:        toPgDatePtr(endDate),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list availability overrides: %w", err)
	}

	items := make([]AvailabilityOverride, 0, len(rows))
	for _, row := range rows {
		items = append(items, availabilityOverrideFromModel(appointmentsdb.RacAppointmentAvailabilityOverride{ID: row.ID, UserID: row.UserID, Date: row.Date, IsAvailable: row.IsAvailable, StartTime: row.StartTime, EndTime: row.EndTime, Timezone: row.Timezone, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID}))
	}

	return items, nil
}

func (r *Repository) GetAvailabilityOverrideByID(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (*AvailabilityOverride, error) {
	row, err := r.queries.GetAvailabilityOverrideByID(ctx, appointmentsdb.GetAvailabilityOverrideByIDParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("availability override not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get availability override: %w", err)
	}

	item := availabilityOverrideFromModel(appointmentsdb.RacAppointmentAvailabilityOverride{ID: row.ID, UserID: row.UserID, Date: row.Date, IsAvailable: row.IsAvailable, StartTime: row.StartTime, EndTime: row.EndTime, Timezone: row.Timezone, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID})
	return &item, nil
}

func (r *Repository) DeleteAvailabilityOverride(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) error {
	_, err := r.queries.DeleteAvailabilityOverride(ctx, appointmentsdb.DeleteAvailabilityOverrideParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		return fmt.Errorf("failed to delete availability override: %w", err)
	}
	return nil
}

func (r *Repository) UpdateAvailabilityOverride(ctx context.Context, id uuid.UUID, organizationID uuid.UUID, override AvailabilityOverride) (*AvailabilityOverride, error) {
	row, err := r.queries.UpdateAvailabilityOverride(ctx, appointmentsdb.UpdateAvailabilityOverrideParams{
		ID:             toPgUUID(id),
		OrganizationID: toPgUUID(organizationID),
		Date:           toPgDate(override.Date),
		IsAvailable:    override.IsAvailable,
		StartTime:      toPgTimeOfDayPtr(override.StartTime),
		EndTime:        toPgTimeOfDayPtr(override.EndTime),
		Timezone:       override.Timezone,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("availability override not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update availability override: %w", err)
	}

	saved := availabilityOverrideFromModel(appointmentsdb.RacAppointmentAvailabilityOverride{ID: row.ID, UserID: row.UserID, Date: row.Date, IsAvailable: row.IsAvailable, StartTime: row.StartTime, EndTime: row.EndTime, Timezone: row.Timezone, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OrganizationID: row.OrganizationID})
	return &saved, nil
}
