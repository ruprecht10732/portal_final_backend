package transport

import "time"

type CreateInviteRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type CreateInviteResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type InviteResponse struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	ExpiresAt time.Time  `json:"expiresAt"`
	CreatedAt time.Time  `json:"createdAt"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
}

type ListInvitesResponse struct {
	Invites []InviteResponse `json:"invites"`
}

type UpdateInviteRequest struct {
	Email  *string `json:"email" validate:"omitempty,email"`
	Resend bool    `json:"resend"`
}

type UpdateInviteResponse struct {
	Invite InviteResponse `json:"invite"`
	Token  *string        `json:"token,omitempty"`
}

type UpdateOrganizationRequest struct {
	Name         *string `json:"name" validate:"omitempty,max=120"`
	Email        *string `json:"email" validate:"omitempty,email"`
	Phone        *string `json:"phone" validate:"omitempty,max=50"`
	VatNumber    *string `json:"vatNumber" validate:"omitempty,max=20"`
	KvkNumber    *string `json:"kvkNumber" validate:"omitempty,max=20"`
	AddressLine1 *string `json:"addressLine1" validate:"omitempty,max=200"`
	AddressLine2 *string `json:"addressLine2" validate:"omitempty,max=200"`
	PostalCode   *string `json:"postalCode" validate:"omitempty,max=20"`
	City         *string `json:"city" validate:"omitempty,max=120"`
	Country      *string `json:"country" validate:"omitempty,max=120"`
}

type OrganizationResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Email           *string `json:"email,omitempty"`
	Phone           *string `json:"phone,omitempty"`
	VatNumber       *string `json:"vatNumber,omitempty"`
	KvkNumber       *string `json:"kvkNumber,omitempty"`
	AddressLine1    *string `json:"addressLine1,omitempty"`
	AddressLine2    *string `json:"addressLine2,omitempty"`
	PostalCode      *string `json:"postalCode,omitempty"`
	City            *string `json:"city,omitempty"`
	Country         *string `json:"country,omitempty"`
	LogoFileKey     *string `json:"logoFileKey,omitempty"`
	LogoFileName    *string `json:"logoFileName,omitempty"`
	LogoContentType *string `json:"logoContentType,omitempty"`
	LogoSizeBytes   *int64  `json:"logoSizeBytes,omitempty"`
}

// OrgLogoPresignRequest is the request for a presigned organization logo upload URL.
type OrgLogoPresignRequest struct {
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=100"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
}

// OrgLogoPresignResponse returns a presigned logo upload URL.
type OrgLogoPresignResponse struct {
	UploadURL string `json:"uploadUrl"`
	FileKey   string `json:"fileKey"`
	ExpiresAt int64  `json:"expiresAt"`
}

// SetOrgLogoRequest stores logo metadata after upload.
type SetOrgLogoRequest struct {
	FileKey     string `json:"fileKey" validate:"required,min=1,max=500"`
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=100"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
}

// OrgLogoDownloadResponse returns a presigned download URL.
type OrgLogoDownloadResponse struct {
	DownloadURL string `json:"downloadUrl"`
	ExpiresAt   int64  `json:"expiresAt"`
}

// OrganizationSettingsResponse returns the organization's quote defaults.
type OrganizationSettingsResponse struct {
	QuotePaymentDays            int     `json:"quotePaymentDays"`
	QuoteValidDays              int     `json:"quoteValidDays"`
	WhatsAppDeviceID            *string `json:"whatsAppDeviceId,omitempty"`
	WhatsAppWelcomeDelayMinutes int     `json:"whatsAppWelcomeDelayMinutes"`
	SMTPConfigured              bool    `json:"smtpConfigured"`
}

// UpdateOrganizationSettingsRequest updates quote default settings.
type UpdateOrganizationSettingsRequest struct {
	QuotePaymentDays *int `json:"quotePaymentDays" validate:"omitempty,min=1,max=365"`
	QuoteValidDays   *int `json:"quoteValidDays" validate:"omitempty,min=1,max=365"`
	// 0 = send immediately, otherwise delay before sending the automated WhatsApp welcome.
	WhatsAppWelcomeDelayMinutes *int `json:"whatsAppWelcomeDelayMinutes" validate:"omitempty,min=0,max=1440"`
}

// WhatsAppStatusResponse describes the current WhatsApp device state for an organization.
type WhatsAppStatusResponse struct {
	State       string `json:"state"`
	Message     string `json:"message"`
	CanSend     bool   `json:"canSend"`
	NeedsReauth bool   `json:"needsReauth"`
}

// SetSMTPRequest is the request to configure tenant SMTP settings.
type SetSMTPRequest struct {
	Host      string `json:"host" validate:"required,hostname|ip,max=255"`
	Port      int    `json:"port" validate:"required,min=1,max=65535"`
	Username  string `json:"username" validate:"required,max=255"`
	Password  string `json:"password" validate:"omitempty,max=500"`
	FromEmail string `json:"fromEmail" validate:"required,email,max=255"`
	FromName  string `json:"fromName" validate:"required,max=120"`
}

// SMTPStatusResponse returns the current SMTP configuration status (password is never exposed).
type SMTPStatusResponse struct {
	Configured bool    `json:"configured"`
	Host       *string `json:"host,omitempty"`
	Port       *int    `json:"port,omitempty"`
	Username   *string `json:"username,omitempty"`
	FromEmail  *string `json:"fromEmail,omitempty"`
	FromName   *string `json:"fromName,omitempty"`
}

// TestSMTPRequest is the request to send a test email via the configured SMTP.
type TestSMTPRequest struct {
	ToEmail string `json:"toEmail" validate:"required,email"`
}

// DetectSMTPRequest is the request to auto-detect SMTP settings from an email address.
type DetectSMTPRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// DetectSMTPResponse returns the auto-detected SMTP settings.
type DetectSMTPResponse struct {
	Detected bool    `json:"detected"`
	Provider *string `json:"provider,omitempty"`
	Host     *string `json:"host,omitempty"`
	Port     *int    `json:"port,omitempty"`
	Username *string `json:"username,omitempty"`
	Security *string `json:"security,omitempty"` // "STARTTLS" or "SSL/TLS"
}

// Notification workflow rules
type NotificationWorkflowRuleResponse struct {
	Trigger      string  `json:"trigger"`
	Channel      string  `json:"channel"`
	Audience     string  `json:"audience"`
	Enabled      bool    `json:"enabled"`
	DelayMinutes int     `json:"delayMinutes"`
	LeadSource   *string `json:"leadSource,omitempty"`
	TemplateText *string `json:"templateText,omitempty"`
}

type ListNotificationWorkflowsResponse struct {
	Workflows []NotificationWorkflowRuleResponse `json:"workflows"`
}

type UpsertNotificationWorkflowRuleRequest struct {
	Trigger      string  `json:"trigger" validate:"required,max=100"`
	Channel      string  `json:"channel" validate:"omitempty,max=20"`
	Audience     string  `json:"audience" validate:"omitempty,max=20"`
	Enabled      bool    `json:"enabled"`
	DelayMinutes int     `json:"delayMinutes" validate:"min=0,max=1440"`
	LeadSource   *string `json:"leadSource,omitempty" validate:"omitempty,max=50"`
	TemplateText *string `json:"templateText,omitempty" validate:"omitempty,max=4000"`
}

type ReplaceNotificationWorkflowsRequest struct {
	Workflows []UpsertNotificationWorkflowRuleRequest `json:"workflows" validate:"required,dive"`
}
