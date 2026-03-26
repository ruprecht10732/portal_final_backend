package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"portal_final_backend/internal/auth/repository"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/config"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	webauthnSessionTTL         = 5 * time.Minute
	webauthnSessionKeyRegister = "webauthn:session:register:"
	webauthnSessionKeyLogin    = "webauthn:session:login:"
	errPasskeyNotConfigured    = "passkey support not configured"
)

// webauthnUser adapts our User model to the webauthn.User interface.
type webauthnUser struct {
	id          uuid.UUID
	email       string
	displayName string
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte                         { return u.id[:] }
func (u *webauthnUser) WebAuthnName() string                       { return u.email }
func (u *webauthnUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// InitWebAuthn initialises the WebAuthn relying party on the service.
// Must be called after New() by the module wiring code.
func (s *Service) InitWebAuthn(cfg config.WebAuthnConfig) error {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                  cfg.GetWebAuthnRPID(),
		RPDisplayName:         cfg.GetWebAuthnRPDisplayName(),
		RPOrigins:             cfg.GetWebAuthnRPOrigins(),
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationPreferred,
			RequireResidentKey: protocol.ResidentKeyRequired(),
		},
	})
	if err != nil {
		return fmt.Errorf("init webauthn: %w", err)
	}
	s.webauthn = wa
	return nil
}

// ---------------------------------------------------------------------------
// Registration (add passkey) – requires authenticated user
// ---------------------------------------------------------------------------

func (s *Service) BeginPasskeyRegistration(ctx context.Context, userID uuid.UUID) (interface{}, error) {
	if s.webauthn == nil || s.redis == nil {
		return nil, apperr.Internal(errPasskeyNotConfigured)
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, apperr.NotFound("user not found")
	}

	waCreds, err := s.loadWebAuthnCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}

	waUser := s.toWebAuthnUser(user, waCreds)

	excludeList := make([]protocol.CredentialDescriptor, len(waCreds))
	for i, c := range waCreds {
		excludeList[i] = c.Descriptor()
	}

	creation, session, err := s.webauthn.BeginRegistration(waUser,
		webauthn.WithExclusions(excludeList),
	)
	if err != nil {
		return nil, fmt.Errorf("begin registration: %w", err)
	}

	if err := s.storeWebAuthnSession(ctx, webauthnSessionKeyRegister+userID.String(), session); err != nil {
		return nil, err
	}

	return creation, nil
}

func (s *Service) FinishPasskeyRegistration(ctx context.Context, userID uuid.UUID, nickname string, body []byte) error {
	if s.webauthn == nil || s.redis == nil {
		return apperr.Internal(errPasskeyNotConfigured)
	}

	session, err := s.loadWebAuthnSession(ctx, webauthnSessionKeyRegister+userID.String())
	if err != nil {
		return apperr.Unauthorized("registration session expired or invalid")
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return apperr.NotFound("user not found")
	}

	waCreds, err := s.loadWebAuthnCredentials(ctx, userID)
	if err != nil {
		return err
	}

	waUser := s.toWebAuthnUser(user, waCreds)

	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	if err != nil {
		return apperr.BadRequest("invalid credential response: " + err.Error())
	}

	credential, err := s.webauthn.CreateCredential(waUser, *session, parsed)
	if err != nil {
		return apperr.BadRequest("credential verification failed: " + err.Error())
	}

	flagsJSON, _ := json.Marshal(credential.Flags)
	transports := make([]string, len(credential.Transport))
	for i, t := range credential.Transport {
		transports[i] = string(t)
	}

	if err := s.repo.CreateWebAuthnCredential(ctx, repository.WebAuthnCredential{
		ID:              credential.ID,
		UserID:          userID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		Transport:       transports,
		FlagsJSON:       flagsJSON,
		AAGUID:          credential.Authenticator.AAGUID,
		SignCount:       credential.Authenticator.SignCount,
		CloneWarning:    credential.Authenticator.CloneWarning,
		Nickname:        nickname,
	}); err != nil {
		return fmt.Errorf("save credential: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Login (discoverable / passkey login – no username needed)
// ---------------------------------------------------------------------------

func (s *Service) BeginPasskeyLogin(ctx context.Context) (interface{}, error) {
	if s.webauthn == nil || s.redis == nil {
		return nil, apperr.Internal(errPasskeyNotConfigured)
	}

	assertion, session, err := s.webauthn.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationPreferred),
	)
	if err != nil {
		return nil, fmt.Errorf("begin login: %w", err)
	}

	// Store session keyed by challenge (since we don't know the user yet).
	if err := s.storeWebAuthnSession(ctx, webauthnSessionKeyLogin+session.Challenge, session); err != nil {
		return nil, err
	}

	return assertion, nil
}

// FinishPasskeyLogin validates the login assertion and returns JWT tokens.
// The challenge parameter is sent back by the frontend so we can look up the session.
func (s *Service) FinishPasskeyLogin(ctx context.Context, challenge string, body []byte) (string, string, error) {
	if s.webauthn == nil || s.redis == nil {
		return "", "", apperr.Internal(errPasskeyNotConfigured)
	}

	session, err := s.loadWebAuthnSession(ctx, webauthnSessionKeyLogin+challenge)
	if err != nil {
		return "", "", apperr.Unauthorized("login session expired or invalid")
	}

	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	if err != nil {
		return "", "", apperr.BadRequest("invalid assertion response: " + err.Error())
	}

	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		userID, err := uuid.FromBytes(userHandle)
		if err != nil {
			return nil, fmt.Errorf("invalid user handle: %w", err)
		}

		user, err := s.repo.GetUserByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("user not found: %w", err)
		}

		waCreds, err := s.loadWebAuthnCredentials(ctx, userID)
		if err != nil {
			return nil, err
		}

		return s.toWebAuthnUser(user, waCreds), nil
	}

	waUser, credential, err := s.webauthn.ValidatePasskeyLogin(handler, *session, parsed)
	if err != nil {
		return "", "", apperr.Unauthorized("passkey authentication failed")
	}

	// Update sign count in DB.
	_ = s.repo.UpdateWebAuthnCredentialSignCount(ctx, credential.ID, credential.Authenticator.SignCount, credential.Authenticator.CloneWarning)

	userID, err := uuid.FromBytes(waUser.WebAuthnID())
	if err != nil {
		return "", "", fmt.Errorf("invalid user id: %w", err)
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return "", "", err
	}

	if !user.EmailVerified {
		return "", "", apperr.Forbidden("email not verified")
	}

	return s.issueTokens(ctx, userID, user.Email)
}

// ---------------------------------------------------------------------------
// Credential management
// ---------------------------------------------------------------------------

type PasskeyInfo struct {
	ID         []byte     `json:"id"`
	Nickname   string     `json:"nickname"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

func (s *Service) ListPasskeys(ctx context.Context, userID uuid.UUID) ([]PasskeyInfo, error) {
	creds, err := s.repo.ListWebAuthnCredentialsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	infos := make([]PasskeyInfo, len(creds))
	for i, c := range creds {
		infos[i] = PasskeyInfo{
			ID:         c.ID,
			Nickname:   c.Nickname,
			CreatedAt:  c.CreatedAt,
			LastUsedAt: c.LastUsedAt,
		}
	}
	return infos, nil
}

func (s *Service) RenamePasskey(ctx context.Context, userID uuid.UUID, credID []byte, nickname string) error {
	return s.repo.UpdateWebAuthnCredentialNickname(ctx, credID, userID, nickname)
}

func (s *Service) DeletePasskey(ctx context.Context, userID uuid.UUID, credID []byte) error {
	return s.repo.DeleteWebAuthnCredential(ctx, credID, userID)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *Service) toWebAuthnUser(user repository.User, creds []webauthn.Credential) *webauthnUser {
	displayName := user.Email
	if user.FirstName != nil && user.LastName != nil {
		displayName = *user.FirstName + " " + *user.LastName
	}
	return &webauthnUser{
		id:          user.ID,
		email:       user.Email,
		displayName: displayName,
		credentials: creds,
	}
}

func (s *Service) loadWebAuthnCredentials(ctx context.Context, userID uuid.UUID) ([]webauthn.Credential, error) {
	rows, err := s.repo.ListWebAuthnCredentialsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	creds := make([]webauthn.Credential, len(rows))
	for i, row := range rows {
		var flags webauthn.CredentialFlags
		_ = json.Unmarshal(row.FlagsJSON, &flags)

		transports := make([]protocol.AuthenticatorTransport, len(row.Transport))
		for j, t := range row.Transport {
			transports[j] = protocol.AuthenticatorTransport(t)
		}

		creds[i] = webauthn.Credential{
			ID:              row.ID,
			PublicKey:       row.PublicKey,
			AttestationType: row.AttestationType,
			Transport:       transports,
			Flags:           flags,
			Authenticator: webauthn.Authenticator{
				AAGUID:       row.AAGUID,
				SignCount:    row.SignCount,
				CloneWarning: row.CloneWarning,
			},
		}
	}
	return creds, nil
}

func (s *Service) storeWebAuthnSession(ctx context.Context, key string, session *webauthn.SessionData) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return s.redis.Set(ctx, key, data, webauthnSessionTTL).Err()
}

func (s *Service) loadWebAuthnSession(ctx context.Context, key string) (*webauthn.SessionData, error) {
	data, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("load session: %w", err)
	}
	// Delete after reading (single-use).
	s.redis.Del(ctx, key)

	var session webauthn.SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &session, nil
}
