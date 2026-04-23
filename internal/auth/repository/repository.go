package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	authdb "portal_final_backend/internal/auth/db"
)

// Exported standard errors
var (
	ErrNotFound                  = errors.New("not found")
	ErrInvalidRole               = errors.New("invalid role")
	ErrSuperAdminAlreadyAssigned = errors.New("superadmin already assigned")
)

// Supported token types
const (
	TokenTypeEmailVerify   = "EMAIL_VERIFY"
	TokenTypePasswordReset = "PASSWORD_RESET"
)

// Repository implements the AuthRepository interface using PostgreSQL.
type Repository struct {
	pool    *pgxpool.Pool
	queries *authdb.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: authdb.New(pool),
	}
}

func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// =============================================================================
// Domain Models
// =============================================================================

type User struct {
	ID                    uuid.UUID
	Email                 string
	PasswordHash          string
	EmailVerified         bool
	FirstName             *string
	LastName              *string
	Phone                 *string
	OnboardingCompletedAt *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type UserWithRoles struct {
	ID        uuid.UUID
	Email     string
	FirstName *string
	LastName  *string
	Roles     []string
}

// authUserFields is an internal DTO to standardize mapping from various sqlc row types.
type authUserFields struct {
	ID                    pgtype.UUID
	Email                 string
	PasswordHash          string
	EmailVerified         bool
	FirstName             pgtype.Text
	LastName              pgtype.Text
	Phone                 pgtype.Text
	OnboardingCompletedAt pgtype.Timestamptz
	CreatedAt             pgtype.Timestamptz
	UpdatedAt             pgtype.Timestamptz
}

// =============================================================================
// User Management ($O(\log N)$ assuming standard B-Tree indexing on ID/Email)
// =============================================================================

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	// Safe defer: If tx is committed, Rollback is a safe no-op in pgx.
	defer func() { _ = tx.Rollback(ctx) }()

	user, err := r.CreateUserTx(ctx, tx, email, passwordHash)
	if err != nil {
		return User{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return User{}, err
	}

	return user, nil
}

func (r *Repository) CreateUserTx(ctx context.Context, tx pgx.Tx, email, passwordHash string) (User, error) {
	q := r.queries.WithTx(tx)
	row, err := q.CreateUser(ctx, authdb.CreateUserParams{Email: email, PasswordHash: passwordHash})
	if err != nil {
		return User{}, err
	}

	if err = q.EnsureUserSettings(ctx, toPgUUID(row.ID.Bytes)); err != nil {
		return User{}, err
	}

	return userFromAuthRow(authUserFields{
		ID:                    row.ID,
		Email:                 row.Email,
		PasswordHash:          row.PasswordHash,
		EmailVerified:         row.IsEmailVerified,
		FirstName:             row.FirstName,
		LastName:              row.LastName,
		Phone:                 row.Phone,
		OnboardingCompletedAt: row.OnboardingCompletedAt,
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}), nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return User{}, handlePgxError(err)
	}
	return userFromAuthRow(authUserFields{
		ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified,
		FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone,
		OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID uuid.UUID) (User, error) {
	row, err := r.queries.GetUserByID(ctx, toPgUUID(userID))
	if err != nil {
		return User{}, handlePgxError(err)
	}
	return userFromAuthRow(authUserFields{
		ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified,
		FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone,
		OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

func (r *Repository) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	return r.queries.MarkEmailVerified(ctx, toPgUUID(userID))
}

func (r *Repository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	return r.queries.UpdatePassword(ctx, authdb.UpdatePasswordParams{ID: toPgUUID(userID), PasswordHash: passwordHash})
}

func (r *Repository) UpdateUserEmail(ctx context.Context, userID uuid.UUID, email string) (User, error) {
	row, err := r.queries.UpdateUserEmail(ctx, authdb.UpdateUserEmailParams{ID: toPgUUID(userID), Email: email})
	if err != nil {
		return User{}, err
	}
	return userFromAuthRow(authUserFields{
		ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified,
		FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone,
		OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

func (r *Repository) UpdateUserNames(ctx context.Context, userID uuid.UUID, firstName, lastName *string) (User, error) {
	row, err := r.queries.UpdateUserNames(ctx, authdb.UpdateUserNamesParams{ID: toPgUUID(userID), FirstName: toPgText(firstName), LastName: toPgText(lastName)})
	if err != nil {
		return User{}, err
	}
	return userFromAuthRow(authUserFields{
		ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified,
		FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone,
		OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

func (r *Repository) UpdateUserPhone(ctx context.Context, userID uuid.UUID, phone *string) (User, error) {
	var phoneValue string
	if phone != nil {
		phoneValue = *phone
	}

	row, err := r.queries.UpdateUserPhone(ctx, authdb.UpdateUserPhoneParams{ID: toPgUUID(userID), Phone: phoneValue})
	if err != nil {
		return User{}, err
	}
	return userFromAuthRow(authUserFields{
		ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified,
		FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone,
		OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

// =============================================================================
// User Settings
// =============================================================================

func (r *Repository) EnsureUserSettings(ctx context.Context, userID uuid.UUID) error {
	return r.queries.EnsureUserSettings(ctx, toPgUUID(userID))
}

func (r *Repository) GetUserSettings(ctx context.Context, userID uuid.UUID) (string, error) {
	lang, err := r.queries.GetUserSettings(ctx, toPgUUID(userID))
	return lang, handlePgxError(err)
}

func (r *Repository) UpdateUserSettings(ctx context.Context, userID uuid.UUID, preferredLanguage string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.queries.WithTx(tx)
	if err = q.UpsertUserSettings(ctx, authdb.UpsertUserSettingsParams{
		UserID:            toPgUUID(userID),
		PreferredLanguage: preferredLanguage,
	}); err != nil {
		return err
	}

	if err = q.TouchUserUpdatedAt(ctx, toPgUUID(userID)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// =============================================================================
// Token Management ($O(\log N)$ lookup speed)
// =============================================================================

func (r *Repository) CreateUserToken(ctx context.Context, userID uuid.UUID, tokenHash, tokenType string, expiresAt time.Time) error {
	return r.queries.CreateUserToken(ctx, authdb.CreateUserTokenParams{
		UserID:    toPgUUID(userID),
		TokenHash: tokenHash,
		Type:      tokenType,
		ExpiresAt: toPgTimestamp(expiresAt),
	})
}

func (r *Repository) GetUserToken(ctx context.Context, tokenHash, tokenType string) (uuid.UUID, time.Time, error) {
	row, err := r.queries.GetUserToken(ctx, authdb.GetUserTokenParams{TokenHash: tokenHash, Type: tokenType})
	if err != nil {
		return uuid.UUID{}, time.Time{}, handlePgxError(err)
	}
	return row.UserID.Bytes, row.ExpiresAt.Time, nil
}

func (r *Repository) UseUserToken(ctx context.Context, tokenHash, tokenType string) error {
	return r.queries.UseUserToken(ctx, authdb.UseUserTokenParams{TokenHash: tokenHash, Type: tokenType})
}

func (r *Repository) CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	return r.queries.CreateRefreshToken(ctx, authdb.CreateRefreshTokenParams{
		UserID:    toPgUUID(userID),
		TokenHash: tokenHash,
		ExpiresAt: toPgTimestamp(expiresAt),
	})
}

func (r *Repository) GetRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, time.Time, error) {
	row, err := r.queries.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return uuid.UUID{}, time.Time{}, handlePgxError(err)
	}
	return row.UserID.Bytes, row.ExpiresAt.Time, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	return r.queries.RevokeRefreshToken(ctx, tokenHash)
}

func (r *Repository) RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	return r.queries.RevokeAllRefreshTokens(ctx, toPgUUID(userID))
}

// =============================================================================
// Role Management (RBAC)
// =============================================================================

func (r *Repository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return r.queries.GetUserRoles(ctx, toPgUUID(userID))
}

func (r *Repository) SetUserRoles(ctx context.Context, userID uuid.UUID, roles []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := r.SetUserRolesTx(ctx, tx, userID, roles); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) SetUserRolesTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, roles []string) error {
	if len(roles) == 0 {
		return ErrInvalidRole
	}

	q := r.queries.WithTx(tx)
	validRoles, err := q.GetValidRoles(ctx, roles)
	if err != nil {
		return err
	}

	// Data sanitization and integrity check
	if len(validRoles) != len(uniqueStrings(roles)) {
		return ErrInvalidRole
	}

	if err := q.DeleteUserRoles(ctx, toPgUUID(userID)); err != nil {
		return err
	}

	if err := q.InsertUserRoles(ctx, authdb.InsertUserRolesParams{UserID: toPgUUID(userID), Column2: roles}); err != nil {
		return mapRoleWriteError(err)
	}

	return nil
}

func (r *Repository) HasAnyUserWithRole(ctx context.Context, role string) (bool, error) {
	// Uses raw query due to `EXISTS` check optimization ($O(1)$ short-circuit if index on name exists)
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM RAC_user_roles ur
			JOIN RAC_roles r ON r.id = ur.role_id
			WHERE r.name = $1
		)`

	var exists bool
	if err := r.pool.QueryRow(ctx, query, role).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// =============================================================================
// Bulk Operations & Onboarding
// =============================================================================

// ListUsers performs an $O(N)$ sequential scan across the users table.
func (r *Repository) ListUsers(ctx context.Context) ([]UserWithRoles, error) {
	rows, err := r.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]UserWithRoles, 0, len(rows))
	for _, row := range rows {
		users = append(users, userWithRolesFromAuthRow(row.ID, row.Email, row.FirstName, row.LastName, row.Roles))
	}
	return users, nil
}

func (r *Repository) ListUsersByOrganization(ctx context.Context, organizationID uuid.UUID) ([]UserWithRoles, error) {
	rows, err := r.queries.ListUsersByOrganization(ctx, toPgUUID(organizationID))
	if err != nil {
		return nil, err
	}

	users := make([]UserWithRoles, 0, len(rows))
	for _, row := range rows {
		users = append(users, userWithRolesFromAuthRow(row.ID, row.Email, row.FirstName, row.LastName, row.Roles))
	}
	return users, nil
}

func (r *Repository) MarkOnboardingComplete(ctx context.Context, userID uuid.UUID) error {
	return r.queries.MarkOnboardingComplete(ctx, toPgUUID(userID))
}

// =============================================================================
// WebAuthn Credential Methods
// =============================================================================

type WebAuthnCredential struct {
	ID              []byte
	UserID          uuid.UUID
	PublicKey       []byte
	AttestationType string
	Transport       []string
	FlagsJSON       []byte
	AAGUID          []byte
	SignCount       uint32
	CloneWarning    bool
	Nickname        string
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}

func (r *Repository) CreateWebAuthnCredential(ctx context.Context, cred WebAuthnCredential) error {
	return r.queries.CreateWebAuthnCredential(ctx, authdb.CreateWebAuthnCredentialParams{
		ID:              cred.ID,
		UserID:          toPgUUID(cred.UserID),
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		Transport:       cred.Transport,
		FlagsJson:       cred.FlagsJSON,
		Aaguid:          cred.AAGUID,
		SignCount:       int64(cred.SignCount),
		CloneWarning:    cred.CloneWarning,
		Nickname:        cred.Nickname,
	})
}

func (r *Repository) ListWebAuthnCredentialsByUser(ctx context.Context, userID uuid.UUID) ([]WebAuthnCredential, error) {
	rows, err := r.queries.ListWebAuthnCredentialsByUser(ctx, toPgUUID(userID))
	if err != nil {
		return nil, err
	}
	creds := make([]WebAuthnCredential, len(rows))
	for i, row := range rows {
		creds[i] = webauthnCredFromRow(row)
	}
	return creds, nil
}

func (r *Repository) UpdateWebAuthnCredentialSignCount(ctx context.Context, credID []byte, signCount uint32, cloneWarning bool) error {
	return r.queries.UpdateWebAuthnCredentialSignCount(ctx, authdb.UpdateWebAuthnCredentialSignCountParams{
		ID:           credID,
		SignCount:    int64(signCount),
		CloneWarning: cloneWarning,
	})
}

func (r *Repository) UpdateWebAuthnCredentialNickname(ctx context.Context, credID []byte, userID uuid.UUID, nickname string) error {
	return r.queries.UpdateWebAuthnCredentialNickname(ctx, authdb.UpdateWebAuthnCredentialNicknameParams{
		ID:       credID,
		UserID:   toPgUUID(userID),
		Nickname: nickname,
	})
}

func (r *Repository) DeleteWebAuthnCredential(ctx context.Context, credID []byte, userID uuid.UUID) error {
	return r.queries.DeleteWebAuthnCredential(ctx, authdb.DeleteWebAuthnCredentialParams{
		ID:     credID,
		UserID: toPgUUID(userID),
	})
}

func (r *Repository) GetUserByWebAuthnCredentialID(ctx context.Context, credID []byte) (User, error) {
	row, err := r.queries.GetUserByWebAuthnCredentialID(ctx, credID)
	if err != nil {
		return User{}, handlePgxError(err)
	}
	return userFromAuthRow(authUserFields{
		ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified,
		FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone,
		OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}), nil
}

// =============================================================================
// Internal Helpers & Mappers
// =============================================================================

// handlePgxError centrally manages pgx specific errors mapping to domain errors
func handlePgxError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func mapRoleWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "ux_single_superadmin_user" {
		return ErrSuperAdminAlreadyAssigned
	}
	return err
}

// uniqueStrings operates in $O(N)$ Time and $O(N)$ Space
func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func toPgUUID(id uuid.UUID) pgtype.UUID            { return pgtype.UUID{Bytes: id, Valid: true} }
func toPgTimestamp(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
func optionalString(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
func optionalTime(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func userFromAuthRow(fields authUserFields) User {
	return User{
		ID:                    fields.ID.Bytes,
		Email:                 fields.Email,
		PasswordHash:          fields.PasswordHash,
		EmailVerified:         fields.EmailVerified,
		FirstName:             optionalString(fields.FirstName),
		LastName:              optionalString(fields.LastName),
		Phone:                 optionalString(fields.Phone),
		OnboardingCompletedAt: optionalTime(fields.OnboardingCompletedAt),
		CreatedAt:             fields.CreatedAt.Time,
		UpdatedAt:             fields.UpdatedAt.Time,
	}
}

func userWithRolesFromAuthRow(id pgtype.UUID, email string, firstName, lastName pgtype.Text, roles []string) UserWithRoles {
	return UserWithRoles{
		ID:        id.Bytes,
		Email:     email,
		FirstName: optionalString(firstName),
		LastName:  optionalString(lastName),
		Roles:     roles,
	}
}

func webauthnCredFromRow(row authdb.RacWebauthnCredential) WebAuthnCredential {
	return WebAuthnCredential{
		ID:              row.ID,
		UserID:          row.UserID.Bytes,
		PublicKey:       row.PublicKey,
		AttestationType: row.AttestationType,
		Transport:       row.Transport,
		FlagsJSON:       row.FlagsJson,
		AAGUID:          row.Aaguid,
		SignCount:       uint32(row.SignCount),
		CloneWarning:    row.CloneWarning,
		Nickname:        row.Nickname,
		CreatedAt:       row.CreatedAt.Time,
		LastUsedAt:      optionalTime(row.LastUsedAt),
	}
}
