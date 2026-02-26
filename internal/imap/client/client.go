package client

import (
	"fmt"
	"strings"
	"sync"
	"time"

	goimap "github.com/BrianLeishman/go-imap"
)

type Config struct {
	Username string
	Password string
	Host     string
	Port     int
}

type MessageMetadata struct {
	UID            int64
	MessageID      *string
	FromName       *string
	FromAddress    *string
	Subject        string
	SentAt         *time.Time
	ReceivedAt     *time.Time
	Snippet        *string
	SizeBytes      int64
	Seen           bool
	Flagged        bool
	Answered       bool
	Deleted        bool
	HasAttachments bool
}

type MessageContent struct {
	UID         int64
	MessageID   *string
	Subject     string
	FromName    *string
	FromAddress *string
	ReplyTo     []Address
	To          []Address
	CC          []Address
	SentAt      *time.Time
	ReceivedAt  *time.Time
	HTML        *string
	Text        *string
}

type Address struct {
	Name    string
	Address string
}

type Client struct {
	dialer *goimap.Dialer
}

var configureLibraryOnce sync.Once

func configureLibraryDefaults() {
	configureLibraryOnce.Do(func() {
		// Keep retries bounded to avoid minute-long request stalls on flaky links.
		goimap.RetryCount = 2
		goimap.DialTimeout = 8 * time.Second
		goimap.CommandTimeout = 15 * time.Second
	})
}

func New(cfg Config) (*Client, error) {
	configureLibraryDefaults()
	dialer, err := goimap.New(cfg.Username, cfg.Password, cfg.Host, cfg.Port)
	if err != nil {
		return nil, err
	}
	return &Client{dialer: dialer}, nil
}

func (c *Client) Close() error {
	if c == nil || c.dialer == nil {
		return nil
	}
	return c.dialer.Close()
}

func (c *Client) TestConnection() error {
	if c == nil || c.dialer == nil {
		return fmt.Errorf("imap client is not initialized")
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		_, err := c.dialer.GetFolders()
		if err == nil {
			return nil
		}
		lastErr = err
		// Some providers occasionally drop a session with EOF; retry once.
		if !isEOFError(err) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return lastErr
}

func (c *Client) SyncFolderMetadata(folder string, maxMessages int) ([]MessageMetadata, error) {
	if c == nil || c.dialer == nil {
		return nil, fmt.Errorf("imap client is not initialized")
	}
	if err := c.dialer.SelectFolder(folder); err != nil {
		return nil, err
	}
	if maxMessages <= 0 {
		maxMessages = 100
	}

	uids, err := c.dialer.GetLastNUIDs(maxMessages)
	if err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return []MessageMetadata{}, nil
	}

	overviewMap, err := c.dialer.GetOverviews(uids...)
	if err != nil {
		return nil, err
	}

	items := make([]MessageMetadata, 0, len(overviewMap))
	for uid, email := range overviewMap {
		fromName, fromAddress := parseFrom(email.From)
		snippet := firstSnippet(email.Text)
		items = append(items, MessageMetadata{
			UID:            int64(uid),
			MessageID:      optionalString(email.MessageID),
			FromName:       fromName,
			FromAddress:    fromAddress,
			Subject:        email.Subject,
			SentAt:         optionalTime(email.Sent),
			ReceivedAt:     optionalTime(email.Received),
			Snippet:        snippet,
			SizeBytes:      int64(email.Size),
			Seen:           containsFlag(email.Flags, "\\Seen"),
			Flagged:        containsFlag(email.Flags, "\\Flagged"),
			Answered:       containsFlag(email.Flags, "\\Answered"),
			Deleted:        containsFlag(email.Flags, "\\Deleted"),
			HasAttachments: len(email.Attachments) > 0,
		})
	}

	return items, nil
}

func (c *Client) MoveToTrashAndDelete(folder string, uid int64) error {
	if c == nil || c.dialer == nil {
		return fmt.Errorf("imap client is not initialized")
	}
	if err := c.dialer.SelectFolder(folder); err != nil {
		return err
	}

	const trashFolder = "Trash"
	if err := c.dialer.MoveEmail(int(uid), trashFolder); err == nil {
		return nil
	}

	if err := c.dialer.DeleteEmail(int(uid)); err != nil {
		return err
	}
	return c.dialer.Expunge()
}

func (c *Client) SetSeen(folder string, uid int64, seen bool) error {
	if c == nil || c.dialer == nil {
		return fmt.Errorf("imap client is not initialized")
	}
	if err := c.dialer.SelectFolder(folder); err != nil {
		return err
	}
	err := c.setFlag(uid, "\\Seen", seen)
	return err
}

func (c *Client) SetAnswered(folder string, uid int64, answered bool) error {
	if c == nil || c.dialer == nil {
		return fmt.Errorf("imap client is not initialized")
	}
	if err := c.dialer.SelectFolder(folder); err != nil {
		return err
	}
	err := c.setFlag(uid, "\\Answered", answered)
	return err
}

func (c *Client) GetMessageContent(folder string, uid int64) (MessageContent, error) {
	if c == nil || c.dialer == nil {
		return MessageContent{}, fmt.Errorf("imap client is not initialized")
	}

	var (
		emails map[int]*goimap.Email
		err    error
	)
	for attempt := 0; attempt < 2; attempt++ {
		if err = c.dialer.SelectFolder(folder); err != nil {
			if !isTransientIMAPError(err) {
				return MessageContent{}, err
			}
		} else {
			emails, err = c.dialer.GetEmails(int(uid))
			if err == nil {
				break
			}
			if !isTransientIMAPError(err) {
				return MessageContent{}, err
			}
		}
		_ = c.dialer.Reconnect()
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		return MessageContent{}, err
	}
	email, ok := emails[int(uid)]
	if !ok || email == nil {
		return MessageContent{}, fmt.Errorf("message not found")
	}
	fromName, fromAddress := parseFrom(email.From)
	return MessageContent{
		UID:         uid,
		MessageID:   optionalString(email.MessageID),
		Subject:     strings.TrimSpace(email.Subject),
		FromName:    fromName,
		FromAddress: fromAddress,
		ReplyTo:     mapToAddresses(email.ReplyTo),
		To:          mapToAddresses(email.To),
		CC:          mapToAddresses(email.CC),
		SentAt:      optionalTime(email.Sent),
		ReceivedAt:  optionalTime(email.Received),
		HTML:        optionalString(email.HTML),
		Text:        optionalString(email.Text),
	}, nil
}

func (c *Client) setFlag(uid int64, flag string, enabled bool) error {
	op := "+"
	if !enabled {
		op = "-"
	}
	command := fmt.Sprintf(`UID STORE %d %sFLAGS.SILENT (%s)`, uid, op, flag)
	_, err := c.dialer.Exec(command, false, goimap.RetryCount, nil)
	return err
}

func containsFlag(flags []string, want string) bool {
	for _, flag := range flags {
		if strings.EqualFold(flag, want) {
			return true
		}
	}
	return false
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	v := value
	return &v
}

func firstSnippet(text string) *string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > 200 {
		snippet := trimmed[:200]
		return &snippet
	}
	return &trimmed
}

func parseFrom(raw map[string]string) (*string, *string) {
	if len(raw) == 0 {
		return nil, nil
	}

	for address, name := range raw {
		trimmedAddress := strings.TrimSpace(address)
		trimmedName := strings.TrimSpace(name)
		return optionalString(trimmedName), optionalString(trimmedAddress)
	}
	return nil, nil
}

func mapToAddresses(raw map[string]string) []Address {
	if len(raw) == 0 {
		return nil
	}
	items := make([]Address, 0, len(raw))
	for address, name := range raw {
		trimmedAddress := strings.TrimSpace(address)
		if trimmedAddress == "" {
			continue
		}
		items = append(items, Address{
			Name:    strings.TrimSpace(name),
			Address: trimmedAddress,
		})
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func isEOFError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "eof")
}

func isTransientIMAPError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "eof") {
		return true
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") {
		return true
	}
	if strings.Contains(msg, "connection reset") || strings.Contains(msg, "broken pipe") {
		return true
	}
	return false
}

// IsTransientError reports whether an IMAP failure is likely temporary.
func IsTransientError(err error) bool {
	return isTransientIMAPError(err)
}
