package transport

import "time"

type CreateAccountRequest struct {
	EmailAddress string `json:"emailAddress" validate:"required,email,max=255"`
	IMAPHost     string `json:"imapHost" validate:"required,max=255"`
	IMAPPort     int    `json:"imapPort" validate:"required,min=1,max=65535"`
	IMAPUsername string `json:"imapUsername" validate:"required,max=255"`
	IMAPPassword string `json:"imapPassword" validate:"required,min=1,max=1024"`
	SMTPHost     *string `json:"smtpHost,omitempty" validate:"omitempty,max=255"`
	SMTPPort     *int    `json:"smtpPort,omitempty" validate:"omitempty,min=1,max=65535"`
	SMTPUsername *string `json:"smtpUsername,omitempty" validate:"omitempty,max=255"`
	SMTPPassword *string `json:"smtpPassword,omitempty" validate:"omitempty,min=1,max=1024"`
	SMTPFromEmail *string `json:"smtpFromEmail,omitempty" validate:"omitempty,email,max=255"`
	SMTPFromName  *string `json:"smtpFromName,omitempty" validate:"omitempty,max=255"`
	FolderName   string `json:"folderName,omitempty" validate:"omitempty,max=255"`
	Enabled      *bool  `json:"enabled,omitempty"`
}

type UpdateAccountRequest struct {
	EmailAddress *string `json:"emailAddress,omitempty" validate:"omitempty,email,max=255"`
	IMAPHost     *string `json:"imapHost,omitempty" validate:"omitempty,max=255"`
	IMAPPort     *int    `json:"imapPort,omitempty" validate:"omitempty,min=1,max=65535"`
	IMAPUsername *string `json:"imapUsername,omitempty" validate:"omitempty,max=255"`
	IMAPPassword *string `json:"imapPassword,omitempty" validate:"omitempty,min=1,max=1024"`
	SMTPHost     *string `json:"smtpHost,omitempty" validate:"omitempty,max=255"`
	SMTPPort     *int    `json:"smtpPort,omitempty" validate:"omitempty,min=1,max=65535"`
	SMTPUsername *string `json:"smtpUsername,omitempty" validate:"omitempty,max=255"`
	SMTPPassword *string `json:"smtpPassword,omitempty" validate:"omitempty,min=1,max=1024"`
	SMTPFromEmail *string `json:"smtpFromEmail,omitempty" validate:"omitempty,email,max=255"`
	SMTPFromName  *string `json:"smtpFromName,omitempty" validate:"omitempty,max=255"`
	FolderName   *string `json:"folderName,omitempty" validate:"omitempty,max=255"`
	Enabled      *bool   `json:"enabled,omitempty"`
}

type AccountResponse struct {
	ID           string     `json:"id"`
	UserID       string     `json:"userId"`
	EmailAddress string     `json:"emailAddress"`
	IMAPHost     string     `json:"imapHost"`
	IMAPPort     int        `json:"imapPort"`
	IMAPUsername string     `json:"imapUsername"`
	SMTPHost     *string    `json:"smtpHost,omitempty"`
	SMTPPort     *int       `json:"smtpPort,omitempty"`
	SMTPUsername *string    `json:"smtpUsername,omitempty"`
	SMTPFromEmail *string   `json:"smtpFromEmail,omitempty"`
	SMTPFromName  *string   `json:"smtpFromName,omitempty"`
	SMTPConfigured bool     `json:"smtpConfigured"`
	FolderName   string     `json:"folderName"`
	Enabled      bool       `json:"enabled"`
	LastSyncAt   *time.Time `json:"lastSyncAt,omitempty"`
	LastError    *string    `json:"lastError,omitempty"`
	LastErrorAt  *time.Time `json:"lastErrorAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type DetectAccountRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
}

type DetectAccountResponse struct {
	Detected bool    `json:"detected"`
	Provider *string `json:"provider,omitempty"`
	Host     *string `json:"host,omitempty"`
	Port     *int    `json:"port,omitempty"`
	Username *string `json:"username,omitempty"`
	Security *string `json:"security,omitempty"` // "STARTTLS" or "SSL/TLS"
}

type ListMessagesQuery struct {
	Page     int `form:"page" validate:"omitempty,min=1"`
	PageSize int `form:"pageSize" validate:"omitempty,min=1,max=200"`
}

type MessageResponse struct {
	ID             string     `json:"id"`
	AccountID      string     `json:"accountId"`
	FolderName     string     `json:"folderName"`
	UID            int64      `json:"uid"`
	MessageID      *string    `json:"messageId,omitempty"`
	FromName       *string    `json:"fromName,omitempty"`
	FromAddress    *string    `json:"fromAddress,omitempty"`
	Subject        string     `json:"subject"`
	SentAt         *time.Time `json:"sentAt,omitempty"`
	ReceivedAt     *time.Time `json:"receivedAt,omitempty"`
	Snippet        *string    `json:"snippet,omitempty"`
	SizeBytes      int64      `json:"sizeBytes"`
	Seen           bool       `json:"seen"`
	Flagged        bool       `json:"flagged"`
	Answered       bool       `json:"answered"`
	Deleted        bool       `json:"deleted"`
	HasAttachments bool       `json:"hasAttachments"`
	SyncedAt       time.Time  `json:"syncedAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type MessageContentResponse struct {
	AccountID   string     `json:"accountId"`
	UID         int64      `json:"uid"`
	MessageID   *string    `json:"messageId,omitempty"`
	Subject     string     `json:"subject"`
	FromName    *string    `json:"fromName,omitempty"`
	FromAddress *string    `json:"fromAddress,omitempty"`
	ReplyTo     []string   `json:"replyTo,omitempty"`
	To          []string   `json:"to,omitempty"`
	CC          []string   `json:"cc,omitempty"`
	SentAt      *time.Time `json:"sentAt,omitempty"`
	ReceivedAt  *time.Time `json:"receivedAt,omitempty"`
	BodyHTML    *string    `json:"bodyHtml,omitempty"`
	BodyText    *string    `json:"bodyText,omitempty"`
}

type SendMessageRequest struct {
	To      []string  `json:"to" validate:"required,min=1,dive,email,max=255"`
	Cc      []string  `json:"cc,omitempty" validate:"omitempty,dive,email,max=255"`
	Subject string    `json:"subject" validate:"required,max=998"`
	Body    string    `json:"body" validate:"required,max=200000"`
	IsHTML  *bool     `json:"isHtml,omitempty"`
	InReply *int64    `json:"inReplyToUid,omitempty" validate:"omitempty,min=1"`
}

type ReplyRequest struct {
	Body   string `json:"body" validate:"required,max=200000"`
	IsHTML *bool  `json:"isHtml,omitempty"`
}

type ListMessagesResponse struct {
	Items      []MessageResponse `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
}
