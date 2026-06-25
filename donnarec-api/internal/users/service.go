// Package users provides user management services for donnarec-api.
package users

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
)

// UserRole mirrors the DB enum for type safety in Go code.
type UserRole string

const (
	RoleMaker   UserRole = "maker"
	RoleChecker UserRole = "checker"
	RoleAdmin   UserRole = "admin"
)

// User is the application-level user model returned from the service layer.
// It includes roles loaded via a separate query (not a JOIN, to keep sqlc simple).
type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	DisplayName     string     `json:"display_name"`
	KeycloakSubject string     `json:"keycloak_subject"`
	IsActive        bool       `json:"is_active"`
	LegalHold       bool       `json:"legal_hold"`
	Roles           []UserRole `json:"roles"`
}

// CreateUserParams holds the inputs needed to create a new user.
type CreateUserParams struct {
	Email           string
	DisplayName     string
	KeycloakSubject string
	Roles           []UserRole
}

// UserService encapsulates user management logic.
// It uses constructor injection for all dependencies (no global state).
type UserService struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	logger  *zap.Logger
}

// NewUserService creates a UserService with the given dependencies.
func NewUserService(pool *pgxpool.Pool, queries *db.Queries, logger *zap.Logger) *UserService {
	return &UserService{
		pool:    pool,
		queries: queries,
		logger:  logger,
	}
}

// CreateUser inserts a new user row and assigns their roles, all within a single transaction.
// It returns the created user with roles populated.
//
// AUDIT NOTE (WR-01): the user-creation audit row is currently written by the
// AuditMiddleware in its OWN transaction AFTER this mutation commits (best-effort,
// post-commit). To make the audit write atomic with the mutation, accept an
// audit.AuditService here and call AppendAuditEntryTx(ctx, tx, entry) inside the
// WithTx below. That in-transaction wiring is deliberately deferred to a follow-up
// phase — do not assume same-transaction auditing is in force for this handler.
func (s *UserService) CreateUser(ctx context.Context, params CreateUserParams) (*User, error) {
	if len(params.Roles) == 0 {
		return nil, fmt.Errorf("at least one role must be assigned")
	}

	var result *User
	err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		// Insert user row
		row, err := qtx.CreateUser(ctx, db.CreateUserParams{
			Email:           params.Email,
			DisplayName:     params.DisplayName,
			KeycloakSubject: params.KeycloakSubject,
		})
		if err != nil {
			s.logger.Error("failed to create user",
				zap.String("operation", "CreateUser"),
				zap.Error(err),
				// ห้าม log PII ตาม Pattern C: ไม่ log email/national_id
			)
			return fmt.Errorf("create user: %w", err)
		}

		result = &User{
			ID:              row.ID.String(),
			Email:           row.Email,
			DisplayName:     row.DisplayName,
			KeycloakSubject: row.KeycloakSubject,
			IsActive:        row.IsActive,
			LegalHold:       row.LegalHold,
			Roles:           make([]UserRole, 0, len(params.Roles)),
		}

		// Assign roles (one INSERT per role; ON CONFLICT DO NOTHING for idempotency).
		// AssignRole is :one with RETURNING, so a conflict (DO NOTHING → zero rows)
		// surfaces as pgx.ErrNoRows. That case means "role already assigned" and is a
		// no-op success, NOT a failure — treat it as success so re-assignment stays
		// idempotent and does not abort the transaction (WR-02).
		for _, role := range params.Roles {
			_, err := qtx.AssignRole(ctx, db.AssignRoleParams{
				UserID: row.ID,
				Role:   db.UserRoleEnum(role),
			})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("assign role %q to user %s: %w", role, row.ID, err)
			}
			result.Roles = append(result.Roles, role)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.logger.Info("user created",
		zap.String("user_id", result.ID),
		// ห้าม log PII (national_id, etc.) ตาม Pattern C
	)

	return result, nil
}

// GetUser retrieves a user by ID and populates their current roles.
func (s *UserService) GetUser(ctx context.Context, userID string) (*User, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(userID); err != nil {
		return nil, fmt.Errorf("invalid user ID %q: %w", userID, err)
	}

	row, err := s.queries.GetUserByID(ctx, pgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	roleRows, err := s.queries.ListRolesForUser(ctx, pgUUID)
	if err != nil {
		return nil, fmt.Errorf("list roles for user %s: %w", userID, err)
	}

	roles := make([]UserRole, 0, len(roleRows))
	for _, r := range roleRows {
		roles = append(roles, UserRole(r))
	}

	return &User{
		ID:              row.ID.String(),
		Email:           row.Email,
		DisplayName:     row.DisplayName,
		KeycloakSubject: row.KeycloakSubject,
		IsActive:        row.IsActive,
		LegalHold:       row.LegalHold,
		Roles:           roles,
	}, nil
}
