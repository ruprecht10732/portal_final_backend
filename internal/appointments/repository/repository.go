package repository

import (
	"context"
	"errors"
	"time"

	appointmentsdb "portal_final_backend/internal/appointments/db"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Appointment represents the appointment database model
type Appointment struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	LeadID         *uuid.UUID
	LeadServiceID  *uuid.UUID
	Type           string
	Title          string
	Description    *string
	Location       *string
	MeetingLink    *string
	StartTime      time.Time
	EndTime        time.Time
	Status         string
	AllDay         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type LeadInfo struct {
	ID          uuid.UUID
	FirstName   string
	LastName    string
	Phone       string
	Street      string
	HouseNumber string
	City        string
}

type Repository struct {
	pool    *pgxpool.Pool
	queries *appointmentsdb.Queries
}

const appointmentNotFoundMsg = "appointment not found"

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: appointmentsdb.New(pool)}
}

// --- Type Conversion Helpers ---

func toPgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }
func toPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return toPgUUID(*id)
}

func toPgUUIDSlice(ids []uuid.UUID) []pgtype.UUID {
	items := make([]pgtype.UUID, len(ids))
	for i, id := range ids {
		items[i] = toPgUUID(id)
	}
	return items
}

func toPgText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func toPgTimestamp(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
func toPgTimestampPtr(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return toPgTimestamp(*v)
}

func toPgDate(v time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func toPgDatePtr(v *time.Time) pgtype.Date {
	if v == nil {
		return pgtype.Date{}
	}
	return toPgDate(*v)
}

func toPgTimeOfDay(v time.Time) pgtype.Time {
	h, m, s := v.Clock()
	us := int64(h)*3600e6 + int64(m)*60e6 + int64(s)*1e6 + int64(v.Nanosecond()/1000)
	return pgtype.Time{Microseconds: us, Valid: true}
}

func toPgTimeOfDayPtr(v *time.Time) pgtype.Time {
	if v == nil {
		return pgtype.Time{}
	}
	return toPgTimeOfDay(*v)
}

func optionalUUID(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

func optionalString(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

// --- Operations ---

func (r *Repository) Create(ctx context.Context, a *Appointment) error {
	return r.queries.CreateAppointment(ctx, appointmentsdb.CreateAppointmentParams{
		ID:             toPgUUID(a.ID),
		OrganizationID: toPgUUID(a.OrganizationID),
		UserID:         toPgUUID(a.UserID),
		LeadID:         toPgUUIDPtr(a.LeadID),
		LeadServiceID:  toPgUUIDPtr(a.LeadServiceID),
		Type:           a.Type,
		Title:          a.Title,
		Description:    toPgText(a.Description),
		Location:       toPgText(a.Location),
		MeetingLink:    toPgText(a.MeetingLink),
		StartTime:      toPgTimestamp(a.StartTime),
		EndTime:        toPgTimestamp(a.EndTime),
		Status:         a.Status,
		AllDay:         a.AllDay,
		CreatedAt:      toPgTimestamp(a.CreatedAt),
		UpdatedAt:      toPgTimestamp(a.UpdatedAt),
	})
}

func (r *Repository) GetByID(ctx context.Context, id, organizationID uuid.UUID) (*Appointment, error) {
	row, err := r.queries.GetAppointmentByID(ctx, appointmentsdb.GetAppointmentByIDParams{
		ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound(appointmentNotFoundMsg)
		}
		return nil, err
	}
	return &Appointment{
		ID:             uuid.UUID(row.ID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		UserID:         uuid.UUID(row.UserID.Bytes),
		LeadID:         optionalUUID(row.LeadID),
		LeadServiceID:  optionalUUID(row.LeadServiceID),
		Type:           row.Type,
		Title:          row.Title,
		Description:    optionalString(row.Description),
		Location:       optionalString(row.Location),
		MeetingLink:    optionalString(row.MeetingLink),
		StartTime:      row.StartTime.Time,
		EndTime:        row.EndTime.Time,
		Status:         row.Status,
		AllDay:         row.AllDay,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}, nil
}

func (r *Repository) GetByLeadServiceID(ctx context.Context, leadServiceID, organizationID uuid.UUID) (*Appointment, error) {
	row, err := r.queries.GetAppointmentByLeadServiceID(ctx, appointmentsdb.GetAppointmentByLeadServiceIDParams{
		LeadServiceID: toPgUUID(leadServiceID), OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &Appointment{
		ID:             uuid.UUID(row.ID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		UserID:         uuid.UUID(row.UserID.Bytes),
		LeadID:         optionalUUID(row.LeadID),
		LeadServiceID:  optionalUUID(row.LeadServiceID),
		Type:           row.Type,
		Title:          row.Title,
		Description:    optionalString(row.Description),
		Location:       optionalString(row.Location),
		MeetingLink:    optionalString(row.MeetingLink),
		StartTime:      row.StartTime.Time,
		EndTime:        row.EndTime.Time,
		Status:         row.Status,
		AllDay:         row.AllDay,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}, nil
}

func (r *Repository) GetNextScheduledVisit(ctx context.Context, leadID, organizationID uuid.UUID) (*Appointment, error) {
	row, err := r.queries.GetNextUpcomingScheduledVisitByLead(ctx, appointmentsdb.GetNextUpcomingScheduledVisitByLeadParams{
		LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

		latestRow, latestErr := r.queries.GetLatestScheduledVisitByLead(ctx, appointmentsdb.GetLatestScheduledVisitByLeadParams{
			LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID),
		})
		if latestErr != nil {
			if errors.Is(latestErr, pgx.ErrNoRows) {
				return nil, nil
			}
			return nil, latestErr
		}
		return &Appointment{
			ID:             uuid.UUID(latestRow.ID.Bytes),
			OrganizationID: uuid.UUID(latestRow.OrganizationID.Bytes),
			UserID:         uuid.UUID(latestRow.UserID.Bytes),
			LeadID:         optionalUUID(latestRow.LeadID),
			LeadServiceID:  optionalUUID(latestRow.LeadServiceID),
			Type:           latestRow.Type,
			Title:          latestRow.Title,
			Description:    optionalString(latestRow.Description),
			Location:       optionalString(latestRow.Location),
			MeetingLink:    optionalString(latestRow.MeetingLink),
			StartTime:      latestRow.StartTime.Time,
			EndTime:        latestRow.EndTime.Time,
			Status:         latestRow.Status,
			AllDay:         latestRow.AllDay,
			CreatedAt:      latestRow.CreatedAt.Time,
			UpdatedAt:      latestRow.UpdatedAt.Time,
		}, nil
	}
	return &Appointment{
		ID:             uuid.UUID(row.ID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		UserID:         uuid.UUID(row.UserID.Bytes),
		LeadID:         optionalUUID(row.LeadID),
		LeadServiceID:  optionalUUID(row.LeadServiceID),
		Type:           row.Type,
		Title:          row.Title,
		Description:    optionalString(row.Description),
		Location:       optionalString(row.Location),
		MeetingLink:    optionalString(row.MeetingLink),
		StartTime:      row.StartTime.Time,
		EndTime:        row.EndTime.Time,
		Status:         row.Status,
		AllDay:         row.AllDay,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}, nil
}

func (r *Repository) GetNextRequestedVisit(ctx context.Context, leadID, organizationID uuid.UUID) (*Appointment, error) {
	row, err := r.queries.GetNextRequestedVisitByLead(ctx, appointmentsdb.GetNextRequestedVisitByLeadParams{
		LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &Appointment{
		ID:             uuid.UUID(row.ID.Bytes),
		OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
		UserID:         uuid.UUID(row.UserID.Bytes),
		LeadID:         optionalUUID(row.LeadID),
		LeadServiceID:  optionalUUID(row.LeadServiceID),
		Type:           row.Type,
		Title:          row.Title,
		Description:    optionalString(row.Description),
		Location:       optionalString(row.Location),
		MeetingLink:    optionalString(row.MeetingLink),
		StartTime:      row.StartTime.Time,
		EndTime:        row.EndTime.Time,
		Status:         row.Status,
		AllDay:         row.AllDay,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}, nil
}

func (r *Repository) ListLeadVisitsByStatus(ctx context.Context, leadID, organizationID uuid.UUID, statuses []string) ([]Appointment, error) {
	if len(statuses) == 0 {
		return []Appointment{}, nil
	}

	rows, err := r.queries.ListLeadVisitsByStatus(ctx, appointmentsdb.ListLeadVisitsByStatusParams{
		LeadID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID), Column3: statuses,
	})
	if err != nil {
		return nil, err
	}

	items := make([]Appointment, 0, len(rows))
	for _, row := range rows {
		items = append(items, Appointment{
			ID:             uuid.UUID(row.ID.Bytes),
			OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
			UserID:         uuid.UUID(row.UserID.Bytes),
			LeadID:         optionalUUID(row.LeadID),
			LeadServiceID:  optionalUUID(row.LeadServiceID),
			Type:           row.Type,
			Title:          row.Title,
			Description:    optionalString(row.Description),
			Location:       optionalString(row.Location),
			MeetingLink:    optionalString(row.MeetingLink),
			StartTime:      row.StartTime.Time,
			EndTime:        row.EndTime.Time,
			Status:         row.Status,
			AllDay:         row.AllDay,
			CreatedAt:      row.CreatedAt.Time,
			UpdatedAt:      row.UpdatedAt.Time,
		})
	}
	return items, nil
}

func (r *Repository) Update(ctx context.Context, a *Appointment) error {
	res, err := r.queries.UpdateAppointment(ctx, appointmentsdb.UpdateAppointmentParams{
		ID: toPgUUID(a.ID), Title: a.Title, Description: toPgText(a.Description),
		Location: toPgText(a.Location), MeetingLink: toPgText(a.MeetingLink),
		StartTime: toPgTimestamp(a.StartTime), EndTime: toPgTimestamp(a.EndTime),
		AllDay: a.AllDay, UpdatedAt: toPgTimestamp(a.UpdatedAt), OrganizationID: toPgUUID(a.OrganizationID),
	})
	return r.affected(res, err)
}

func (r *Repository) UpdateStatus(ctx context.Context, id, organizationID uuid.UUID, status string) error {
	res, err := r.queries.UpdateAppointmentStatus(ctx, appointmentsdb.UpdateAppointmentStatusParams{
		ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID), Status: status, UpdatedAt: toPgTimestamp(time.Now()),
	})
	return r.affected(res, err)
}

func (r *Repository) Delete(ctx context.Context, id, organizationID uuid.UUID) error {
	res, err := r.queries.DeleteAppointment(ctx, appointmentsdb.DeleteAppointmentParams{
		ID: toPgUUID(id), OrganizationID: toPgUUID(organizationID),
	})
	return r.affected(res, err)
}

func (r *Repository) affected(res int64, err error) error {
	if err != nil {
		return err
	}
	if res == 0 {
		return apperr.NotFound(appointmentNotFoundMsg)
	}
	return nil
}

// --- Listing & Filters ---

type ListParams struct {
	OrganizationID            uuid.UUID
	UserID, LeadID            *uuid.UUID
	Type, Status              *string
	StartFrom, StartTo        *time.Time
	Search, SortBy, SortOrder string
	Page, PageSize            int
}

type ListResult struct {
	Items                             []Appointment
	Total, Page, PageSize, TotalPages int
}

func (r *Repository) List(ctx context.Context, p ListParams) (*ListResult, error) {
	f := appointmentsdb.CountAppointmentsParams{
		OrganizationID: toPgUUID(p.OrganizationID),
		UserID:         toPgUUIDPtr(p.UserID),
		LeadID:         toPgUUIDPtr(p.LeadID),
		Type:           toPgText(p.Type),
		Status:         toPgText(p.Status),
		StartFrom:      toPgTimestampPtr(p.StartFrom),
		StartTo:        toPgTimestampPtr(p.StartTo),
		Search:         optionalSearchParam(p.Search),
	}

	sortBy, _ := resolveSort(p.SortBy, "startTime")
	sortOrder, _ := resolveSort(p.SortOrder, "asc")

	total, err := r.queries.CountAppointments(ctx, f)
	if err != nil {
		return nil, err
	}

	offset := (p.Page - 1) * p.PageSize
	rows, err := r.queries.ListAppointments(ctx, appointmentsdb.ListAppointmentsParams{
		OrganizationID: f.OrganizationID, UserID: f.UserID, LeadID: f.LeadID,
		Type: f.Type, Status: f.Status, StartFrom: f.StartFrom, StartTo: f.StartTo,
		Search: f.Search, SortBy: sortBy, SortOrder: sortOrder,
		OffsetCount: int32(offset), LimitCount: int32(p.PageSize),
	})
	if err != nil {
		return nil, err
	}

	items := make([]Appointment, 0, len(rows))
	for _, row := range rows {
		items = append(items, Appointment{
			ID:             uuid.UUID(row.ID.Bytes),
			OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
			UserID:         uuid.UUID(row.UserID.Bytes),
			LeadID:         optionalUUID(row.LeadID),
			LeadServiceID:  optionalUUID(row.LeadServiceID),
			Type:           row.Type,
			Title:          row.Title,
			Description:    optionalString(row.Description),
			Location:       optionalString(row.Location),
			MeetingLink:    optionalString(row.MeetingLink),
			StartTime:      row.StartTime.Time,
			EndTime:        row.EndTime.Time,
			Status:         row.Status,
			AllDay:         row.AllDay,
			CreatedAt:      row.CreatedAt.Time,
			UpdatedAt:      row.UpdatedAt.Time,
		})
	}

	return &ListResult{
		Items: items, Total: int(total), Page: p.Page, PageSize: p.PageSize,
		TotalPages: (int(total) + p.PageSize - 1) / p.PageSize,
	}, nil
}

func optionalSearchParam(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + v + "%", Valid: true}
}

func resolveSort(val, def string) (string, error) {
	if val == "" {
		return def, nil
	}
	return val, nil
}

// --- Lead Info ---

func (r *Repository) GetLeadInfo(ctx context.Context, leadID, organizationID uuid.UUID) (*LeadInfo, error) {
	row, err := r.queries.GetAppointmentLeadInfo(ctx, appointmentsdb.GetAppointmentLeadInfoParams{
		ID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &LeadInfo{
		ID:          uuid.UUID(row.ID.Bytes),
		FirstName:   row.ConsumerFirstName,
		LastName:    row.ConsumerLastName,
		Phone:       row.ConsumerPhone,
		Street:      row.AddressStreet,
		HouseNumber: row.AddressHouseNumber,
		City:        row.AddressCity,
	}, nil
}

func (r *Repository) GetLeadEmail(ctx context.Context, leadID, organizationID uuid.UUID) (string, error) {
	email, err := r.queries.GetAppointmentLeadEmail(ctx, appointmentsdb.GetAppointmentLeadEmailParams{
		ID: toPgUUID(leadID), OrganizationID: toPgUUID(organizationID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return email, nil
}

func (r *Repository) GetLeadInfoBatch(ctx context.Context, leadIDs []uuid.UUID, orgID uuid.UUID) (map[uuid.UUID]*LeadInfo, error) {
	if len(leadIDs) == 0 {
		return make(map[uuid.UUID]*LeadInfo), nil
	}

	rows, err := r.queries.ListAppointmentLeadInfoByIDs(ctx, appointmentsdb.ListAppointmentLeadInfoByIDsParams{
		Column1: toPgUUIDSlice(leadIDs), OrganizationID: toPgUUID(orgID),
	})
	if err != nil {
		return nil, err
	}

	res := make(map[uuid.UUID]*LeadInfo, len(rows))
	for _, row := range rows {
		res[uuid.UUID(row.ID.Bytes)] = &LeadInfo{
			ID:          uuid.UUID(row.ID.Bytes),
			FirstName:   row.ConsumerFirstName,
			LastName:    row.ConsumerLastName,
			Phone:       row.ConsumerPhone,
			Street:      row.AddressStreet,
			HouseNumber: row.AddressHouseNumber,
			City:        row.AddressCity,
		}
	}
	return res, nil
}

func (r *Repository) ListForDateRange(ctx context.Context, orgID, userID uuid.UUID, start, end time.Time) ([]Appointment, error) {
	rows, err := r.queries.ListAppointmentsForDateRange(ctx, appointmentsdb.ListAppointmentsForDateRangeParams{
		OrganizationID: toPgUUID(orgID), UserID: toPgUUID(userID),
		EndTime: toPgTimestamp(start), StartTime: toPgTimestamp(end),
	})
	if err != nil {
		return nil, err
	}

	items := make([]Appointment, 0, len(rows))
	for _, row := range rows {
		items = append(items, Appointment{
			ID:             uuid.UUID(row.ID.Bytes),
			OrganizationID: uuid.UUID(row.OrganizationID.Bytes),
			UserID:         uuid.UUID(row.UserID.Bytes),
			LeadID:         optionalUUID(row.LeadID),
			LeadServiceID:  optionalUUID(row.LeadServiceID),
			Type:           row.Type,
			Title:          row.Title,
			Description:    optionalString(row.Description),
			Location:       optionalString(row.Location),
			MeetingLink:    optionalString(row.MeetingLink),
			StartTime:      row.StartTime.Time,
			EndTime:        row.EndTime.Time,
			Status:         row.Status,
			AllDay:         row.AllDay,
			CreatedAt:      row.CreatedAt.Time,
			UpdatedAt:      row.UpdatedAt.Time,
		})
	}
	return items, nil
}

func (a *Appointment) ToResponse(leadInfo *transport.AppointmentLeadInfo) transport.AppointmentResponse {
	return transport.AppointmentResponse{
		ID: a.ID, UserID: a.UserID, LeadID: a.LeadID, LeadServiceID: a.LeadServiceID,
		Type: transport.AppointmentType(a.Type), Title: a.Title, Description: a.Description,
		Location: a.Location, MeetingLink: a.MeetingLink, StartTime: a.StartTime, EndTime: a.EndTime,
		Status: transport.AppointmentStatus(a.Status), AllDay: a.AllDay, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, Lead: leadInfo,
	}
}
