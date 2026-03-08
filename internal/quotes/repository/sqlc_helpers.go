package repository

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func toPgUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: value != uuid.Nil}
}

func toPgUUIDPtr(value *uuid.UUID) pgtype.UUID {
	if value == nil || *value == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *value, Valid: true}
}

func toPgTextPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func toPgTimestampPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func toPgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func toPgFloat8Ptr(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func optionalUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	identifier := uuid.UUID(value.Bytes)
	return &identifier
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

func optionalFloat(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	floatValue := value.Float64
	return &floatValue
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
