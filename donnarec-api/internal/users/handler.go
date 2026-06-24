// Package users provides HTTP handlers for user management endpoints.
package users

import (
	"net/http"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// UserHandler handles HTTP requests for user management endpoints.
// All endpoints require authentication (handled by middleware before reaching here).
type UserHandler struct {
	svc      *UserService
	validate *validator.Validate
	logger   *zap.Logger
}

// NewUserHandler creates a UserHandler with the given dependencies.
func NewUserHandler(svc *UserService, logger *zap.Logger) *UserHandler {
	return &UserHandler{
		svc:      svc,
		validate: validator.New(),
		logger:   logger,
	}
}

// Me returns the authenticated user's identity from their JWT claims.
// GET /api/me
//
// This endpoint does NOT require a DB lookup — it reads directly from the
// claims set by RequireAuth middleware. It is useful for front-end session
// initialization and as an e2e auth smoke test.
func (h *UserHandler) Me(c *gin.Context) {
	raw, exists := c.Get("claims")
	if !exists {
		// Should not happen if RequireAuth is applied, but guard defensively
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}

	claims, ok := raw.(auth.KeycloakClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"sub":   claims.Subject,
		"email": claims.Email,
		"roles": claims.RealmAccess.Roles,
	})
}

// CreateUserRequest is the request body for creating a new user.
// Full validation tags enforce input correctness at the API boundary.
type CreateUserRequest struct {
	Email           string   `json:"email"            validate:"required,email,max=255"`
	DisplayName     string   `json:"display_name"     validate:"required,min=2,max=100"`
	KeycloakSubject string   `json:"keycloak_subject" validate:"required,min=1,max=255"`
	Roles           []string `json:"roles"            validate:"required,min=1,dive,oneof=maker checker admin"`
}

// CreateUser creates a new user account (Admin-only — D-01).
// POST /api/admin/users
//
// The endpoint is protected by RequireRoles(RoleAdmin) at the router level.
// Full CRUD validation is completed in plan 01-04; this endpoint covers the
// core happy path required for Phase 1 walking skeleton.
//
// TODO(01-04): complete CRUD — update, list, deactivate endpoints.
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request_body"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "validation_failed",
			"details": err.Error(),
		})
		return
	}

	// Convert string roles to UserRole type for type-safety
	roles := make([]UserRole, 0, len(req.Roles))
	for _, r := range req.Roles {
		roles = append(roles, UserRole(r))
	}

	user, err := h.svc.CreateUser(c.Request.Context(), CreateUserParams{
		Email:           req.Email,
		DisplayName:     req.DisplayName,
		KeycloakSubject: req.KeycloakSubject,
		Roles:           roles,
	})
	if err != nil {
		h.logger.Error("failed to create user in handler",
			zap.String("operation", "CreateUser"),
			zap.Error(err),
			// ห้าม log PII: no email or national_id in error logs (Pattern C)
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user_creation_failed"})
		return
	}

	// Set audit marker for the audit middleware (wired in plan 01-02)
	c.Set("audit_after", user)

	c.JSON(http.StatusCreated, gin.H{"data": user})
}
