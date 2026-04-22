package tasks

import (
	"time"

	"github.com/google/uuid"
)

const (
	ScopeGlobal      = "global"
	ScopeLeadService = "lead_service"

	StatusOpen      = "open"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"

	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

type ReminderConfig struct {
	Enabled      bool       `json:"enabled"`
	RunAt        *time.Time `json:"runAt,omitempty"`
	RepeatDaily  bool       `json:"repeatDaily"`
	SendEmail    bool       `json:"sendEmail"`
	SendWhatsApp bool       `json:"sendWhatsApp"`
}

type TaskRecord struct {
	ID                uuid.UUID       `json:"id"`
	TenantID          uuid.UUID       `json:"tenantId"`
	ScopeType         string          `json:"scopeType"`
	LeadID            *uuid.UUID      `json:"leadId,omitempty"`
	LeadServiceID     *uuid.UUID      `json:"leadServiceId,omitempty"`
	AssignedUserID    uuid.UUID       `json:"assignedUserId"`
	CreatedByUserID   uuid.UUID       `json:"createdByUserId"`
	Title             string          `json:"title"`
	Description       string          `json:"description"`
	Status            string          `json:"status"`
	Priority          string          `json:"priority"`
	DueAt             *time.Time      `json:"dueAt,omitempty"`
	CompletedAt       *time.Time      `json:"completedAt,omitempty"`
	CancelledAt       *time.Time      `json:"cancelledAt,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	AssigneeEmail     string          `json:"assigneeEmail"`
	AssigneeFirstName *string         `json:"assigneeFirstName,omitempty"`
	AssigneeLastName  *string         `json:"assigneeLastName,omitempty"`
	Reminder          *ReminderRecord `json:"reminder,omitempty"`
}

type ReminderRecord struct {
	ID              uuid.UUID  `json:"id"`
	TaskID          uuid.UUID  `json:"taskId"`
	TenantID        uuid.UUID  `json:"tenantId"`
	Enabled         bool       `json:"enabled"`
	SendEmail       bool       `json:"sendEmail"`
	SendWhatsApp    bool       `json:"sendWhatsApp"`
	NextRunAt       *time.Time `json:"nextRunAt,omitempty"`
	RepeatDaily     bool       `json:"repeatDaily"`
	LastSentAt      *time.Time `json:"lastSentAt,omitempty"`
	LastTriggeredAt *time.Time `json:"lastTriggeredAt,omitempty"`
	LastError       *string    `json:"lastError,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type CreateTaskRequest struct {
	ScopeType      string          `json:"scopeType" validate:"required,oneof=global lead_service"`
	LeadID         *string         `json:"leadId,omitempty" validate:"omitempty,uuid4"`
	LeadServiceID  *string         `json:"leadServiceId,omitempty" validate:"omitempty,uuid4"`
	AssignedUserID string          `json:"assignedUserId" validate:"required,uuid4"`
	Title          string          `json:"title" validate:"required,max=200"`
	Description    *string         `json:"description,omitempty" validate:"omitempty,max=2000"`
	Priority       string          `json:"priority" validate:"omitempty,oneof=low normal high urgent"`
	DueAt          *time.Time      `json:"dueAt,omitempty"`
	Reminder       *ReminderConfig `json:"reminder,omitempty"`
}

type UpdateTaskRequest struct {
	Title            *string         `json:"title,omitempty" validate:"omitempty,max=200"`
	Description      *string         `json:"description,omitempty" validate:"omitempty,max=2000"`
	Priority         *string         `json:"priority,omitempty" validate:"omitempty,oneof=low normal high urgent"`
	DueAt            *time.Time      `json:"dueAt,omitempty"`
	ClearDueAt       bool            `json:"clearDueAt,omitempty"`
	AssignedUserID   *string         `json:"assignedUserId,omitempty" validate:"omitempty,uuid4"`
	Reminder         *ReminderConfig `json:"reminder,omitempty"`
	ClearReminder    bool            `json:"clearReminder,omitempty"`
}

type AssignTaskRequest struct {
	AssignedUserID string `json:"assignedUserId" validate:"required,uuid4"`
}

type ListTasksRequest struct {
	ScopeType      string `form:"scope" validate:"omitempty,oneof=global lead_service"`
	Status         string `form:"status" validate:"omitempty,oneof=open completed cancelled"`
	AssignedUserID string `form:"assignedUserId" validate:"omitempty,uuid4"`
	LeadID         string `form:"leadId" validate:"omitempty,uuid4"`
	LeadServiceID  string `form:"leadServiceId" validate:"omitempty,uuid4"`
	DueFrom        string `form:"dueFrom" validate:"omitempty"`
	DueTo          string `form:"dueTo" validate:"omitempty"`
	Limit          int    `form:"limit" validate:"omitempty,min=1,max=500"`
}

type listTasksFilter struct {
	ScopeType      string
	Status         string
	AssignedUserID *uuid.UUID
	LeadID         *uuid.UUID
	LeadServiceID  *uuid.UUID
	DueFrom        *time.Time
	DueTo          *time.Time
	Limit          int
}

type reminderProcessRecord struct {
	Task          TaskRecord
	Reminder      ReminderRecord
	AssigneePhone *string
}