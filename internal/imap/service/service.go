package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"portal_final_backend/internal/events"
	identityrepo "portal_final_backend/internal/identity/repository"
	"portal_final_backend/internal/imap/client"
	"portal_final_backend/internal/imap/repository"
	"portal_final_backend/internal/imap/sanitize"
	"portal_final_backend/internal/imap/transport"
	"portal_final_backend/internal/imapcrypto"
	"portal_final_backend/internal/scheduler"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
	gomail "github.com/wneessen/go-mail"
)

const (
	defaultFolder           = "INBOX"
	defaultSyncBatchSize    = 50
	maxSyncErrorLength      = 1000
	defaultPeriodicSyncTTL  = 10 * time.Minute
	errIMAPKeyNotConfigured = "imap encryption key not configured"
	errConnectIMAPServer    = "failed to connect to imap server"
)

type Service struct {
	repo          *repository.Repository
	scheduler     IMAPSyncScheduler
	encryptionKey []byte
	lockMap       sync.Map
	eventBus      events.Bus
}

type IMAPSyncScheduler interface {
	EnqueueIMAPSyncAccount(ctx context.Context, payload scheduler.IMAPSyncAccountPayload) error
}

func New(repo *repository.Repository, _ *identityrepo.Repository, bus events.Bus) *Service {
	return &Service{repo: repo, eventBus: bus}
}

func (s *Service) SetSMTPEncryptionKey(_ []byte) {
	// SMTP credentials are stored encrypted with the same key as IMAP credentials.
}

func (s *Service) SetEncryptionKey(key []byte) {
	s.encryptionKey = key
}

func (s *Service) SetScheduler(scheduler IMAPSyncScheduler) {
	s.scheduler = scheduler
}

func (s *Service) CreateAccount(ctx context.Context, userID uuid.UUID, req transport.CreateAccountRequest) (repository.Account, error) {
	if len(s.encryptionKey) != 32 {
		return repository.Account{}, apperr.Internal(errIMAPKeyNotConfigured)
	}
	encryptedPassword, err := imapcrypto.Encrypt(req.IMAPPassword, s.encryptionKey)
	if err != nil {
		return repository.Account{}, apperr.Internal("failed to encrypt imap password")
	}
	folderName := normalizeFolder(req.FolderName)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	smtpHost, smtpPort, smtpUsername, smtpPasswordEncrypted, smtpFromEmail, smtpFromName, err := s.resolveSMTPSettings(smtpSettingsInput{
		emailAddress: strings.TrimSpace(req.EmailAddress),
		imapHost:     strings.TrimSpace(req.IMAPHost),
		imapPassword: strings.TrimSpace(req.IMAPPassword),
		reqHost:      req.SMTPHost,
		reqPort:      req.SMTPPort,
		reqUsername:  req.SMTPUsername,
		reqPassword:  req.SMTPPassword,
		reqFromEmail: req.SMTPFromEmail,
		reqFromName:  req.SMTPFromName,
	})
	if err != nil {
		return repository.Account{}, err
	}
	return s.repo.CreateAccount(ctx, repository.CreateAccountInput{
		UserID:                userID,
		EmailAddress:          strings.TrimSpace(req.EmailAddress),
		IMAPHost:              strings.TrimSpace(req.IMAPHost),
		IMAPPort:              req.IMAPPort,
		IMAPUsername:          strings.TrimSpace(req.IMAPUsername),
		IMAPPasswordEncrypted: encryptedPassword,
		SMTPHost:              smtpHost,
		SMTPPort:              smtpPort,
		SMTPUsername:          smtpUsername,
		SMTPPasswordEncrypted: smtpPasswordEncrypted,
		SMTPFromEmail:         smtpFromEmail,
		SMTPFromName:          smtpFromName,
		FolderName:            folderName,
		Enabled:               enabled,
	})
}

func (s *Service) ListAccounts(ctx context.Context, userID uuid.UUID) ([]repository.Account, error) {
	return s.repo.ListAccountsByUser(ctx, userID)
}

func (s *Service) UpdateAccount(ctx context.Context, userID, accountID uuid.UUID, req transport.UpdateAccountRequest) (repository.Account, error) {
	var encryptedPassword *string
	if req.IMAPPassword != nil && strings.TrimSpace(*req.IMAPPassword) != "" {
		if len(s.encryptionKey) != 32 {
			return repository.Account{}, apperr.Internal(errIMAPKeyNotConfigured)
		}
		enc, err := imapcrypto.Encrypt(strings.TrimSpace(*req.IMAPPassword), s.encryptionKey)
		if err != nil {
			return repository.Account{}, apperr.Internal("failed to encrypt imap password")
		}
		encryptedPassword = &enc
	}
	smtpHost, smtpPort, smtpUsername, smtpPasswordEncrypted, smtpFromEmail, smtpFromName, err := s.resolveSMTPUpdate(smtpUpdateInput{
		reqHost:      req.SMTPHost,
		reqPort:      req.SMTPPort,
		reqUsername:  req.SMTPUsername,
		reqPassword:  req.SMTPPassword,
		reqFromEmail: req.SMTPFromEmail,
		reqFromName:  req.SMTPFromName,
	})
	if err != nil {
		return repository.Account{}, err
	}

	folder := req.FolderName
	if folder != nil {
		value := normalizeFolder(*folder)
		folder = &value
	}

	return s.repo.UpdateAccountByUser(ctx, accountID, userID, repository.UpdateAccountInput{
		EmailAddress:          trimPtr(req.EmailAddress),
		IMAPHost:              trimPtr(req.IMAPHost),
		IMAPPort:              req.IMAPPort,
		IMAPUsername:          trimPtr(req.IMAPUsername),
		IMAPPasswordEncrypted: encryptedPassword,
		SMTPHost:              smtpHost,
		SMTPPort:              smtpPort,
		SMTPUsername:          smtpUsername,
		SMTPPasswordEncrypted: smtpPasswordEncrypted,
		SMTPFromEmail:         smtpFromEmail,
		SMTPFromName:          smtpFromName,
		FolderName:            folder,
		Enabled:               req.Enabled,
	})
}

func (s *Service) DeleteAccount(ctx context.Context, userID, accountID uuid.UUID) error {
	return s.repo.DeleteAccountByUser(ctx, accountID, userID)
}

func (s *Service) TestAccountConnection(ctx context.Context, userID, accountID uuid.UUID) error {
	account, err := s.repo.GetAccountByUser(ctx, accountID, userID)
	if err != nil {
		return err
	}
	password, err := s.decryptPassword(account.IMAPPasswordEncrypted)
	if err != nil {
		return err
	}

	imapClient, err := client.New(client.Config{
		Username: account.IMAPUsername,
		Password: password,
		Host:     account.IMAPHost,
		Port:     account.IMAPPort,
	})
	if err != nil {
		return apperr.BadRequest(errConnectIMAPServer)
	}
	defer func() { _ = imapClient.Close() }()

	if err := imapClient.TestConnection(); err != nil {
		return apperr.BadRequest("imap connection test failed")
	}
	_ = s.repo.ClearAccountSyncError(ctx, account.ID)
	return nil
}

func (s *Service) SyncAccount(ctx context.Context, userID, accountID uuid.UUID) error {
	account, err := s.repo.GetAccountByUser(ctx, accountID, userID)
	if err != nil {
		return err
	}
	if s.scheduler != nil {
		payload := scheduler.IMAPSyncAccountPayload{
			AccountID: account.ID.String(),
			UserID:    account.UserID.String(),
		}
		if err := s.scheduler.EnqueueIMAPSyncAccount(ctx, payload); err == nil {
			return nil
		}
	}
	return s.syncAccount(ctx, account)
}

func (s *Service) SyncEligibleAccounts(ctx context.Context) error {
	accounts, err := s.repo.ListAccountsNeedingSync(ctx, defaultPeriodicSyncTTL, 50)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if err := s.syncAccount(ctx, account); err != nil {
			_ = s.repo.SetAccountSyncError(ctx, account.ID, limitError(err.Error()))
		}
	}
	return nil
}

func (s *Service) ListMessages(ctx context.Context, userID, accountID uuid.UUID, query transport.ListMessagesQuery) (repository.ListMessagesResult, error) {
	return s.repo.ListMessagesByUser(ctx, repository.ListMessagesParams{
		UserID:    userID,
		AccountID: accountID,
		Page:      query.Page,
		PageSize:  query.PageSize,
	})
}

func (s *Service) GetUnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.repo.CountUnreadMessagesByUser(ctx, userID)
}

func (s *Service) DeleteMessage(ctx context.Context, userID, accountID uuid.UUID, uid int64) error {
	account, err := s.repo.GetAccountByUser(ctx, accountID, userID)
	if err != nil {
		return err
	}
	password, err := s.decryptPassword(account.IMAPPasswordEncrypted)
	if err != nil {
		return err
	}

	imapClient, err := client.New(client.Config{
		Username: account.IMAPUsername,
		Password: password,
		Host:     account.IMAPHost,
		Port:     account.IMAPPort,
	})
	if err != nil {
		return apperr.BadRequest(errConnectIMAPServer)
	}
	defer func() { _ = imapClient.Close() }()

	if err := imapClient.MoveToTrashAndDelete(account.FolderName, uid); err != nil {
		return apperr.BadRequest("failed to delete message on imap server")
	}
	if err := s.repo.DeleteMessageMetadataByUID(ctx, accountID, uid); err != nil {
		return err
	}
	return nil
}

func (s *Service) SetMessageSeen(ctx context.Context, userID, accountID uuid.UUID, uid int64, seen bool) error {
	account, err := s.repo.GetAccountByUser(ctx, accountID, userID)
	if err != nil {
		return err
	}
	password, err := s.decryptPassword(account.IMAPPasswordEncrypted)
	if err != nil {
		return err
	}

	imapClient, err := client.New(client.Config{
		Username: account.IMAPUsername,
		Password: password,
		Host:     account.IMAPHost,
		Port:     account.IMAPPort,
	})
	if err != nil {
		return apperr.BadRequest(errConnectIMAPServer)
	}
	defer func() { _ = imapClient.Close() }()

	if err := imapClient.SetSeen(account.FolderName, uid, seen); err != nil {
		if client.IsTransientError(err) {
			return apperr.Internal("temporary imap server error; please retry")
		}
		return apperr.BadRequest("failed to update message flags on imap server")
	}
	if err := s.repo.UpdateMessageSeenByUID(ctx, accountID, uid, seen); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetMessageContent(ctx context.Context, userID, accountID uuid.UUID, uid int64) (transport.MessageContentResponse, error) {
	account, err := s.repo.GetAccountByUser(ctx, accountID, userID)
	if err != nil {
		return transport.MessageContentResponse{}, err
	}
	password, err := s.decryptPassword(account.IMAPPasswordEncrypted)
	if err != nil {
		return transport.MessageContentResponse{}, err
	}

	imapClient, err := client.New(client.Config{
		Username: account.IMAPUsername,
		Password: password,
		Host:     account.IMAPHost,
		Port:     account.IMAPPort,
	})
	if err != nil {
		return transport.MessageContentResponse{}, apperr.BadRequest(errConnectIMAPServer)
	}
	defer func() { _ = imapClient.Close() }()

	content, err := imapClient.GetMessageContent(account.FolderName, uid)
	if err != nil {
		if client.IsTransientError(err) {
			return transport.MessageContentResponse{}, apperr.Internal("temporary imap server error; please retry")
		}
		return transport.MessageContentResponse{}, apperr.BadRequest("failed to fetch message content")
	}

	var safeHTML *string
	if content.HTML != nil && strings.TrimSpace(*content.HTML) != "" {
		sanitized := sanitize.SanitizeHTML(*content.HTML)
		if strings.TrimSpace(sanitized) != "" {
			safeHTML = &sanitized
		}
	}
	bodyText := content.Text
	if bodyText != nil {
		trimmed := strings.TrimSpace(*bodyText)
		bodyText = &trimmed
	}

	return transport.MessageContentResponse{
		AccountID:   account.ID.String(),
		UID:         uid,
		MessageID:   content.MessageID,
		Subject:     content.Subject,
		FromName:    content.FromName,
		FromAddress: content.FromAddress,
		ReplyTo:     addressesToStrings(content.ReplyTo),
		To:          addressesToStrings(content.To),
		CC:          addressesToStrings(content.CC),
		SentAt:      content.SentAt,
		ReceivedAt:  content.ReceivedAt,
		BodyHTML:    safeHTML,
		BodyText:    bodyText,
	}, nil
}

func (s *Service) SendMessage(ctx context.Context, userID, accountID uuid.UUID, req transport.SendMessageRequest) error {
	account, err := s.repo.GetAccountByUser(ctx, accountID, userID)
	if err != nil {
		return err
	}
	parentMsgID, refs, err := s.replyHeadersFromUID(account, req.InReply)
	if err != nil {
		return err
	}
	return s.sendViaAccountSMTP(ctx, account, smtpSendRequest{
		To:         req.To,
		Cc:         req.Cc,
		Subject:    req.Subject,
		Body:       req.Body,
		IsHTML:     req.IsHTML,
		InReplyTo:  parentMsgID,
		References: refs,
	})
}

func (s *Service) ReplyMessage(ctx context.Context, userID, accountID uuid.UUID, uid int64, req transport.ReplyRequest, includeAll bool) error {
	content, account, err := s.loadMessageForReply(ctx, userID, accountID, uid)
	if err != nil {
		return err
	}
	recipients := pickReplyRecipients(content, includeAll, account.EmailAddress)
	if len(recipients.To) == 0 {
		return apperr.BadRequest("no valid reply recipients found")
	}
	subject := normalizeReplySubject(content.Subject)
	if err := s.sendViaAccountSMTP(ctx, account, smtpSendRequest{
		To:         recipients.To,
		Cc:         recipients.Cc,
		Subject:    subject,
		Body:       req.Body,
		IsHTML:     req.IsHTML,
		InReplyTo:  content.MessageID,
		References: buildReferences(content.MessageID),
	}); err != nil {
		return err
	}
	return s.markAnswered(ctx, account, uid)
}

type smtpEnvelope struct {
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

type smtpSendRequest struct {
	To         []string
	Cc         []string
	Subject    string
	Body       string
	IsHTML     *bool
	InReplyTo  *string
	References *string
}

func (s *Service) sendViaAccountSMTP(
	ctx context.Context,
	account repository.Account,
	req smtpSendRequest,
) error {
	env, err := s.loadSMTPSender(account)
	if err != nil {
		return err
	}
	msg := gomail.NewMsg()
	if err := msg.FromFormat(env.FromName, env.FromEmail); err != nil {
		return apperr.Validation("invalid smtp from address")
	}
	if err := msg.To(req.To...); err != nil {
		return apperr.Validation("invalid recipient list")
	}
	if len(req.Cc) > 0 {
		if err := msg.Cc(req.Cc...); err != nil {
			return apperr.Validation("invalid cc recipient list")
		}
	}
	msg.Subject(req.Subject)
	useHTML := true
	if req.IsHTML != nil {
		useHTML = *req.IsHTML
	}
	if useHTML {
		msg.SetBodyString(gomail.TypeTextHTML, req.Body)
	} else {
		msg.SetBodyString(gomail.TypeTextPlain, req.Body)
	}
	if req.InReplyTo != nil && strings.TrimSpace(*req.InReplyTo) != "" {
		msg.SetGenHeader("In-Reply-To", strings.TrimSpace(*req.InReplyTo))
	}
	if req.References != nil && strings.TrimSpace(*req.References) != "" {
		msg.SetGenHeader("References", strings.TrimSpace(*req.References))
	}
	clientSMTP, err := gomail.NewClient(env.Host,
		gomail.WithPort(env.Port),
		gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		gomail.WithUsername(env.Username),
		gomail.WithPassword(env.Password),
		gomail.WithTLSPortPolicy(gomail.TLSOpportunistic),
		gomail.WithTimeout(15*time.Second),
		gomail.WithDialContextFunc(func(dctx context.Context, _ string, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(dctx, "tcp4", addr)
		}),
	)
	if err != nil {
		return apperr.Internal("failed to create smtp client")
	}
	if err := clientSMTP.DialAndSendWithContext(ctx, msg); err != nil {
		return apperr.BadRequest("failed to send email")
	}
	return nil
}

func (s *Service) loadSMTPSender(account repository.Account) (smtpEnvelope, error) {
	if account.SMTPHost == nil || account.SMTPPort == nil || account.SMTPUsername == nil || account.SMTPPasswordEncrypted == nil || account.SMTPFromEmail == nil {
		return smtpEnvelope{}, apperr.BadRequest("smtp is not configured for this inbox account")
	}
	password, err := s.decryptPassword(*account.SMTPPasswordEncrypted)
	if err != nil {
		return smtpEnvelope{}, err
	}
	fromName := strings.TrimSpace(*account.SMTPFromEmail)
	if account.SMTPFromName != nil && strings.TrimSpace(*account.SMTPFromName) != "" {
		fromName = strings.TrimSpace(*account.SMTPFromName)
	}
	return smtpEnvelope{
		Host:      strings.TrimSpace(*account.SMTPHost),
		Port:      *account.SMTPPort,
		Username:  strings.TrimSpace(*account.SMTPUsername),
		Password:  password,
		FromEmail: strings.TrimSpace(*account.SMTPFromEmail),
		FromName:  fromName,
	}, nil
}

type smtpSettingsInput struct {
	emailAddress string
	imapHost     string
	imapPassword string
	reqHost      *string
	reqPort      *int
	reqUsername  *string
	reqPassword  *string
	reqFromEmail *string
	reqFromName  *string
}

type smtpUpdateInput struct {
	reqHost      *string
	reqPort      *int
	reqUsername  *string
	reqPassword  *string
	reqFromEmail *string
	reqFromName  *string
}

func (s *Service) resolveSMTPSettings(input smtpSettingsInput) (*string, *int, *string, *string, *string, *string, error) {
	host := trimPtr(input.reqHost)
	if host == nil || *host == "" {
		defaultHost := input.imapHost
		host = &defaultHost
	}
	port := input.reqPort
	if port == nil || *port <= 0 {
		defaultPort := 587
		port = &defaultPort
	}
	username := trimPtr(input.reqUsername)
	if username == nil || *username == "" {
		username = &input.emailAddress
	}
	smtpPassword := strings.TrimSpace(input.imapPassword)
	if input.reqPassword != nil && strings.TrimSpace(*input.reqPassword) != "" {
		smtpPassword = strings.TrimSpace(*input.reqPassword)
	}
	if smtpPassword == "" {
		return nil, nil, nil, nil, nil, nil, apperr.Validation("smtp password or imap password is required")
	}
	enc, err := imapcrypto.Encrypt(smtpPassword, s.encryptionKey)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, apperr.Internal("failed to encrypt smtp password")
	}
	passwordEncrypted := &enc
	fromEmail := trimPtr(input.reqFromEmail)
	if fromEmail == nil || *fromEmail == "" {
		fromEmail = &input.emailAddress
	}
	fromName := trimPtr(input.reqFromName)
	return host, port, username, passwordEncrypted, fromEmail, fromName, nil
}

func (s *Service) resolveSMTPUpdate(input smtpUpdateInput) (*string, *int, *string, *string, *string, *string, error) {
	var passwordEncrypted *string
	if input.reqPassword != nil {
		if strings.TrimSpace(*input.reqPassword) == "" {
			return nil, nil, nil, nil, nil, nil, apperr.Validation("smtp password cannot be empty")
		}
		enc, err := imapcrypto.Encrypt(strings.TrimSpace(*input.reqPassword), s.encryptionKey)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, apperr.Internal("failed to encrypt smtp password")
		}
		passwordEncrypted = &enc
	}
	return trimPtr(input.reqHost), input.reqPort, trimPtr(input.reqUsername), passwordEncrypted, trimPtr(input.reqFromEmail), trimPtr(input.reqFromName), nil
}

func (s *Service) loadMessageForReply(ctx context.Context, userID, accountID uuid.UUID, uid int64) (client.MessageContent, repository.Account, error) {
	account, err := s.repo.GetAccountByUser(ctx, accountID, userID)
	if err != nil {
		return client.MessageContent{}, repository.Account{}, err
	}
	password, err := s.decryptPassword(account.IMAPPasswordEncrypted)
	if err != nil {
		return client.MessageContent{}, repository.Account{}, err
	}
	imapClient, err := client.New(client.Config{
		Username: account.IMAPUsername,
		Password: password,
		Host:     account.IMAPHost,
		Port:     account.IMAPPort,
	})
	if err != nil {
		return client.MessageContent{}, repository.Account{}, apperr.BadRequest(errConnectIMAPServer)
	}
	defer func() { _ = imapClient.Close() }()
	content, err := imapClient.GetMessageContent(account.FolderName, uid)
	if err != nil {
		return client.MessageContent{}, repository.Account{}, apperr.BadRequest("failed to fetch message content")
	}
	return content, account, nil
}

func (s *Service) markAnswered(ctx context.Context, account repository.Account, uid int64) error {
	password, err := s.decryptPassword(account.IMAPPasswordEncrypted)
	if err != nil {
		return err
	}
	imapClient, err := client.New(client.Config{
		Username: account.IMAPUsername,
		Password: password,
		Host:     account.IMAPHost,
		Port:     account.IMAPPort,
	})
	if err != nil {
		return nil
	}
	defer func() { _ = imapClient.Close() }()
	_ = imapClient.SetAnswered(account.FolderName, uid, true)
	_ = s.repo.UpdateMessageAnsweredByUID(ctx, account.ID, uid, true)
	return nil
}

func (s *Service) replyHeadersFromUID(account repository.Account, uid *int64) (*string, *string, error) {
	if uid == nil {
		return nil, nil, nil
	}
	password, err := s.decryptPassword(account.IMAPPasswordEncrypted)
	if err != nil {
		return nil, nil, err
	}
	imapClient, err := client.New(client.Config{
		Username: account.IMAPUsername,
		Password: password,
		Host:     account.IMAPHost,
		Port:     account.IMAPPort,
	})
	if err != nil {
		return nil, nil, apperr.BadRequest(errConnectIMAPServer)
	}
	defer func() { _ = imapClient.Close() }()
	content, err := imapClient.GetMessageContent(account.FolderName, *uid)
	if err != nil {
		return nil, nil, apperr.BadRequest("failed to fetch parent message")
	}
	refs := buildReferences(content.MessageID)
	return content.MessageID, refs, nil
}

type replyRecipients struct {
	To []string
	Cc []string
}

func pickReplyRecipients(content client.MessageContent, includeAll bool, accountEmail string) replyRecipients {
	normalize := func(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
	self := normalize(accountEmail)
	seen := map[string]bool{}
	add := func(dest *[]string, addr string) {
		n := normalize(addr)
		if n == "" || n == self || seen[n] {
			return
		}
		seen[n] = true
		*dest = append(*dest, addr)
	}
	out := replyRecipients{To: []string{}, Cc: []string{}}
	if len(content.ReplyTo) > 0 {
		for _, a := range content.ReplyTo {
			add(&out.To, a.Address)
		}
	} else if content.FromAddress != nil {
		add(&out.To, *content.FromAddress)
	}
	if includeAll {
		for _, a := range content.To {
			add(&out.To, a.Address)
		}
		for _, a := range content.CC {
			add(&out.Cc, a.Address)
		}
	}
	return out
}

func normalizeReplySubject(subject string) string {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return "Re:"
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "re:") {
		return trimmed
	}
	return "Re: " + trimmed
}

func buildReferences(messageID *string) *string {
	if messageID == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*messageID)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func addressesToStrings(items []client.Address) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		addr := strings.TrimSpace(item.Address)
		if addr == "" {
			continue
		}
		out = append(out, addr)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Service) syncAccount(ctx context.Context, account repository.Account) error {
	if !s.tryLock(account.ID) {
		return nil
	}
	defer s.unlock(account.ID)

	password, err := s.decryptPassword(account.IMAPPasswordEncrypted)
	if err != nil {
		_ = s.repo.SetAccountSyncError(ctx, account.ID, limitError(err.Error()))
		return err
	}
	imapClient, err := client.New(client.Config{
		Username: account.IMAPUsername,
		Password: password,
		Host:     account.IMAPHost,
		Port:     account.IMAPPort,
	})
	if err != nil {
		_ = s.repo.SetAccountSyncError(ctx, account.ID, limitError(err.Error()))
		return err
	}
	defer func() { _ = imapClient.Close() }()

	metadata, err := imapClient.SyncFolderMetadata(account.FolderName, defaultSyncBatchSize)
	if err != nil {
		_ = s.repo.SetAccountSyncError(ctx, account.ID, limitError(err.Error()))
		return err
	}

	maxUID := s.getMaxUIDOrZero(ctx, account.ID, account.FolderName)

	inputs := make([]repository.UpsertMessageInput, 0, len(metadata))
	now := time.Now().UTC()
	for _, item := range metadata {
		s.publishNewEmailEvent(ctx, account, item, maxUID)

		inputs = append(inputs, repository.UpsertMessageInput{
			AccountID:      account.ID,
			FolderName:     account.FolderName,
			UID:            item.UID,
			MessageID:      item.MessageID,
			FromName:       item.FromName,
			FromAddress:    item.FromAddress,
			Subject:        item.Subject,
			SentAt:         item.SentAt,
			ReceivedAt:     item.ReceivedAt,
			Snippet:        item.Snippet,
			SizeBytes:      item.SizeBytes,
			Seen:           item.Seen,
			Flagged:        item.Flagged,
			Answered:       item.Answered,
			Deleted:        item.Deleted,
			HasAttachments: item.HasAttachments,
			SyncedAt:       now,
		})
	}
	if len(inputs) == 0 {
		_ = s.repo.MarkAccountSynced(ctx, account.ID, now)
		return nil
	}
	if err := s.repo.UpsertMessages(ctx, inputs); err != nil {
		_ = s.repo.SetAccountSyncError(ctx, account.ID, limitError(err.Error()))
		return err
	}

	return nil
}

func (s *Service) decryptPassword(encrypted string) (string, error) {
	if len(s.encryptionKey) != 32 {
		return "", apperr.Internal(errIMAPKeyNotConfigured)
	}
	password, err := imapcrypto.Decrypt(encrypted, s.encryptionKey)
	if err != nil {
		return "", apperr.Internal("failed to decrypt imap password")
	}
	return password, nil
}

func (s *Service) getMaxUIDOrZero(ctx context.Context, accountID uuid.UUID, folderName string) int64 {
	uid, err := s.repo.GetMaxUID(ctx, accountID, folderName)
	if err != nil {
		return 0
	}
	return uid
}

func (s *Service) publishNewEmailEvent(ctx context.Context, account repository.Account, item client.MessageMetadata, maxUID int64) {
	if item.UID <= maxUID || item.Seen || s.eventBus == nil {
		return
	}

	msgID := ""
	if item.MessageID != nil {
		msgID = *item.MessageID
	}
	fromAddr := ""
	if item.FromAddress != nil {
		fromAddr = *item.FromAddress
	}

	s.eventBus.Publish(ctx, events.NewEmailReceived{
		BaseEvent:   events.NewBaseEvent(),
		AccountID:   account.ID,
		UserID:      account.UserID,
		MessageID:   msgID,
		FromAddress: fromAddr,
		Subject:     item.Subject,
		UID:         item.UID,
	})
}

func (s *Service) tryLock(accountID uuid.UUID) bool {
	_, loaded := s.lockMap.LoadOrStore(accountID.String(), struct{}{})
	return !loaded
}

func (s *Service) unlock(accountID uuid.UUID) {
	s.lockMap.Delete(accountID.String())
}

func normalizeFolder(folder string) string {
	value := strings.TrimSpace(folder)
	if value == "" {
		return defaultFolder
	}
	return value
}

func trimPtr(input *string) *string {
	if input == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*input)
	return &trimmed
}

func limitError(errMsg string) string {
	if len(errMsg) <= maxSyncErrorLength {
		return errMsg
	}
	return fmt.Sprintf("%s...", errMsg[:maxSyncErrorLength-3])
}
