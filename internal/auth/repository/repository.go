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

var ErrNotFound = errors.New("not found")
var ErrInvalidRole = errors.New("invalid role")
var ErrSuperAdminAlreadyAssigned = errors.New("superadmin already assigned")

const (
	TokenTypeEmailVerify   = "EMAIL_VERIFY"
	TokenTypePasswordReset = "PASSWORD_RESET"
	listUsersQuery         = `
		SELECT
			u.id,
			u.email,
			u.first_name,
			u.last_name,
			COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), ARRAY[]::text[])::text[] AS roles
		FROM RAC_users u
		LEFT JOIN RAC_user_roles ur ON ur.user_id = u.id
		LEFT JOIN RAC_roles r ON r.id = ur.role_id
		GROUP BY u.id
		ORDER BY u.email
	`
	listUsersByOrganizationQuery = `
		SELECT
			u.id,
			u.email,
			u.first_name,
			u.last_name,
			COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), ARRAY[]::text[])::text[] AS roles
		FROM RAC_organization_members om
		JOIN RAC_users u ON u.id = om.user_id
		LEFT JOIN RAC_user_roles ur ON ur.user_id = u.id
		LEFT JOIN RAC_roles r ON r.id = ur.role_id
		WHERE om.organization_id = $1
		GROUP BY u.id
		ORDER BY u.email
	`
)

type Repository struct {
	pool    *pgxpool.Pool
	queries *authdb.Queries
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, queries: authdb.New(pool)}
}

func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

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

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	queries := r.queries.WithTx(tx)
	row, err := queries.CreateUser(ctx, authdb.CreateUserParams{Email: email, PasswordHash: passwordHash})
	if err != nil {
		return User{}, err
	}

	if err = queries.EnsureUserSettings(ctx, toPgUUID(row.ID.Bytes)); err != nil {
		return User{}, err
	}

	if err = tx.Commit(ctx); err != nil {
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

func (r *Repository) CreateUserTx(ctx context.Context, tx pgx.Tx, email, passwordHash string) (User, error) {
	queries := r.queries.WithTx(tx)
	row, err := queries.CreateUser(ctx, authdb.CreateUserParams{Email: email, PasswordHash: passwordHash})
	if err != nil {
		return User{}, err
	}

	if err = queries.EnsureUserSettings(ctx, toPgUUID(row.ID.Bytes)); err != nil {
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
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromAuthRow(authUserFields{ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified, FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone, OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID uuid.UUID) (User, error) {
	row, err := r.queries.GetUserByID(ctx, toPgUUID(userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromAuthRow(authUserFields{ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified, FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone, OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
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
	return userFromAuthRow(authUserFields{ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified, FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone, OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

func (r *Repository) UpdateUserNames(ctx context.Context, userID uuid.UUID, firstName, lastName *string) (User, error) {
	row, err := r.queries.UpdateUserNames(ctx, authdb.UpdateUserNamesParams{ID: toPgUUID(userID), FirstName: toPgText(firstName), LastName: toPgText(lastName)})
	if err != nil {
		return User{}, err
	}
	return userFromAuthRow(authUserFields{ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified, FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone, OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil

}

func (r *Repository) UpdateUserPhone(ctx context.Context, userID uuid.UUID, phone *string) (User, error) {
	phoneValue := ""
	if phone != nil {
		phoneValue = *phone
	}

	row, err := r.queries.UpdateUserPhone(ctx, authdb.UpdateUserPhoneParams{ID: toPgUUID(userID), Phone: phoneValue})
	if err != nil {
		return User{}, err
	}
	return userFromAuthRow(authUserFields{ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, EmailVerified: row.IsEmailVerified, FirstName: row.FirstName, LastName: row.LastName, Phone: row.Phone, OnboardingCompletedAt: row.OnboardingCompletedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), nil
}

func (r *Repository) EnsureUserSettings(ctx context.Context, userID uuid.UUID) error {
	return r.queries.EnsureUserSettings(ctx, toPgUUID(userID))
}

func (r *Repository) GetUserSettings(ctx context.Context, userID uuid.UUID) (string, error) {
	preferredLanguage, err := r.queries.GetUserSettings(ctx, toPgUUID(userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return preferredLanguage, err
}

func (r *Repository) UpdateUserSettings(ctx context.Context, userID uuid.UUID, preferredLanguage string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	queries := r.queries.WithTx(tx)
	if err = queries.UpsertUserSettings(ctx, authdb.UpsertUserSettingsParams{
		UserID:            toPgUUID(userID),
		PreferredLanguage: preferredLanguage,
	}); err != nil {
		return err
	}

	if err = queries.TouchUserUpdatedAt(ctx, toPgUUID(userID)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) CreateUserToken(ctx context.Context, userID uuid.UUID, tokenHash string, tokenType string, expiresAt time.Time) error {
	return r.queries.CreateUserToken(ctx, authdb.CreateUserTokenParams{
		UserID:    toPgUUID(userID),
		TokenHash: tokenHash,
		Type:      tokenType,
		ExpiresAt: toPgTimestamp(expiresAt),
	})
}

func (r *Repository) GetUserToken(ctx context.Context, tokenHash string, tokenType string) (uuid.UUID, time.Time, error) {
	row, err := r.queries.GetUserToken(ctx, authdb.GetUserTokenParams{TokenHash: tokenHash, Type: tokenType})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, time.Time{}, ErrNotFound
	}
	if err != nil {
		return uuid.UUID{}, time.Time{}, err
	}
	return row.UserID.Bytes, row.ExpiresAt.Time, nil
}

func (r *Repository) UseUserToken(ctx context.Context, tokenHash string, tokenType string) error {
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
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, time.Time{}, ErrNotFound
	}
	if err != nil {
		return uuid.UUID{}, time.Time{}, err
	}
	return row.UserID.Bytes, row.ExpiresAt.Time, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	return r.queries.RevokeRefreshToken(ctx, tokenHash)
}

func (r *Repository) RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	return r.queries.RevokeAllRefreshTokens(ctx, toPgUUID(userID))
}

func (r *Repository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return r.queries.GetUserRoles(ctx, toPgUUID(userID))
}

func (r *Repository) SetUserRoles(ctx context.Context, userID uuid.UUID, roles []string) error {
	if len(roles) == 0 {
		return ErrInvalidRole
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	queries := r.queries.WithTx(tx)
	validRoles, err := queries.GetValidRoles(ctx, roles)
	if err != nil {
		return err
	}
	if len(validRoles) != len(uniqueStrings(roles)) {
		return ErrInvalidRole
	}

	if err := queries.DeleteUserRoles(ctx, toPgUUID(userID)); err != nil {
		return err
	}

	if err := queries.InsertUserRoles(ctx, authdb.InsertUserRolesParams{UserID: toPgUUID(userID), Column2: roles}); err != nil {
		return mapRoleWriteError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (r *Repository) SetUserRolesTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, roles []string) error {
	if len(roles) == 0 {
		return ErrInvalidRole
	}

	queries := r.queries.WithTx(tx)
	validRoles, err := queries.GetValidRoles(ctx, roles)
	if err != nil {
		return err
	}
	if len(validRoles) != len(uniqueStrings(roles)) {
		return ErrInvalidRole
	}

	if err := queries.DeleteUserRoles(ctx, toPgUUID(userID)); err != nil {
		return err
	}

	if err := queries.InsertUserRoles(ctx, authdb.InsertUserRolesParams{UserID: toPgUUID(userID), Column2: roles}); err != nil {
		return mapRoleWriteError(err)
	}

	return nil
}

func (r *Repository) HasAnyUserWithRole(ctx context.Context, role string) (bool, error) {
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

func mapRoleWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "ux_single_superadmin_user" {
		return ErrSuperAdminAlreadyAssigned
	}
	return err
}

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

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toPgText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func toPgTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
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
