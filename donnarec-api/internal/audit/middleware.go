// Package audit provides the generic audit interceptor for donnarec-api.
//
// AuditMiddleware intercepts every HTTP mutation (POST/PUT/PATCH/DELETE) plus
// any GET endpoint flagged as a PII-reveal endpoint, and writes one audit row
// per request via AuditService.AppendAuditEntry (FR-13, D-15, D-13).
//
// Middleware placement (Pattern D from PATTERNS.md):
//
//	router.Use(Recovery)
//	router.Use(RequestLogger)
//	router.Use(AuditMiddleware(auditSvc))   ← position 3, BEFORE RequireAuth
//	api.Use(RequireAuth())
//
// Placing audit before auth ensures that auth failure events (401/403) are also logged.
// The actor will be empty if the request is not authenticated — the middleware handles this.
//
// ANTI-PATTERN PROHIBITION (Foundational Rule 2):
//
//	ห้ามเขียน audit entry ใน goroutine แยก — ต้อง synchronous เสมอ.
//	On audit error: log via zap but do NOT abort the request (c.Abort is forbidden here).
package audit

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuditMiddleware returns a Gin HandlerFunc that writes one audit row for every
// mutation request (POST/PUT/PATCH/DELETE) plus PII-reveal GETs.
//
// It MUST be placed BEFORE RequireAuth in the router chain (Pattern D) so that
// authentication-failure events (401/403) are captured in the audit log.
//
// On audit write error: the error is logged via the service's zap logger, but the
// user request is NOT aborted. Audit failures must not degrade user experience.
func AuditMiddleware(svc *AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Determine whether this request should be audited before calling Next().
		isReveal := isPIIRevealEndpoint(c)
		if c.Request.Method == http.MethodGet && !isReveal {
			// Plain read-only request — no audit (D-15: audit covers mutations + auth events only)
			c.Next()
			return
		}

		// Execute the handler chain first so we capture the full response status
		// and any audit_before / audit_after values set by the handler.
		c.Next()

		// --- post-handler: build and write audit entry ---

		// Extract actor from the Gin context (set by RequireAuth middleware).
		// If RequireAuth hasn't run yet (audit is before auth) or failed (401/403),
		// actor fields will be empty strings. The audit row still records IP/path/timestamp.
		var actorID, actorEmail string
		if raw, exists := c.Get("claims"); exists {
			if claims, ok := raw.(auth.KeycloakClaims); ok {
				actorID = claims.Subject
				// Use ActorIdentity() so actor_email falls back to
				// preferred_username when "email" is absent from a Keycloak
				// access token (CR-03) — avoids empty audit actor on FR-13.
				actorEmail = claims.ActorIdentity()
			}
		}

		// Build the action string: e.g. "user.create", "pii.reveal", "item.delete"
		action := deriveAction(c.Request.Method, c.FullPath(), isReveal)

		entry := AuditEntry{
			ActorID:    actorID,
			ActorEmail: actorEmail,
			Action:     action,
			Resource:   c.FullPath(),
			IPAddress:  c.ClientIP(),
		}

		// Capture before/after snapshots set by the handler (optional).
		// Handlers call c.Set("audit_before", <value>) / c.Set("audit_after", <value>)
		// to provide state snapshots for the audit record.
		if before, ok := c.Get("audit_before"); ok {
			if b, err := json.Marshal(before); err == nil {
				entry.BeforeJSON = b
			}
		}
		if after, ok := c.Get("audit_after"); ok {
			if b, err := json.Marshal(after); err == nil {
				entry.AfterJSON = b
			}
		}

		// Write the audit entry — synchronous, same goroutine, no goroutine spawn.
		// Error is logged but MUST NOT abort the already-complete request.
		if err := svc.AppendAuditEntry(c.Request.Context(), entry); err != nil {
			svc.logger.Error("audit write failed",
				zap.String("action", action),
				zap.String("resource", c.FullPath()),
				zap.Error(err),
				// ห้าม log actor_id / actor_email ใน error (อาจเป็น PII ใน context นี้)
			)
			// Do NOT call c.Abort() — audit failure must not degrade user-facing response
		}
	}
}

// isPIIRevealEndpoint returns true if the resolved Gin route path ends with "/reveal",
// indicating that the handler will expose sensitive PII fields (national ID, tax ID).
// All such endpoints must be audited even when the method is GET (D-13, D-14).
//
// Phase 3 donor endpoints (e.g. GET /api/donors/:id/reveal) use this suffix.
// This function is the mechanism — the actual donor endpoint wiring happens in Phase 3.
func isPIIRevealEndpoint(c *gin.Context) bool {
	path := c.FullPath()
	return strings.HasSuffix(path, "/reveal")
}

// deriveAction builds a human-readable, dot-separated action string from the
// HTTP method and route path. Examples:
//
//	POST   /api/admin/users         → "user.create"
//	PUT    /api/admin/users/:id     → "user.update"
//	PATCH  /api/admin/users/:id     → "user.update"
//	DELETE /api/admin/users/:id     → "user.delete"
//	GET    /api/donors/:id/reveal   → "pii.reveal"
//	POST   /api/receipts            → "receipt.create"
func deriveAction(method, route string, isReveal bool) string {
	if isReveal {
		return "pii.reveal"
	}

	// Extract resource noun from the route path.
	noun := extractNoun(route)

	switch method {
	case http.MethodPost:
		return noun + ".create"
	case http.MethodPut, http.MethodPatch:
		return noun + ".update"
	case http.MethodDelete:
		return noun + ".delete"
	case http.MethodGet:
		return noun + ".read"
	default:
		return strings.ToLower(method) + "." + noun
	}
}

// extractNoun returns the primary resource noun from a Gin route path.
// It walks path segments backward, skipping parameter placeholders (":id", "*rest"),
// and returns the first concrete segment in singular form.
// Examples:
//
//	/api/admin/users        → "user"
//	/api/admin/users/:id    → "user"
//	/api/receipts/:id       → "receipt"
//	/api/items              → "item"
func extractNoun(route string) string {
	if route == "" {
		return "unknown"
	}
	parts := strings.Split(strings.Trim(route, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		seg := parts[i]
		if seg == "" || strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "*") {
			continue
		}
		// Skip common non-resource path prefixes
		switch seg {
		case "api", "admin", "v1", "v2":
			continue
		}
		return singularize(seg)
	}
	return "unknown"
}

// singularize applies minimal English singularization for common resource names
// used in this system. This is intentionally simple.
func singularize(s string) string {
	switch {
	case strings.HasSuffix(s, "ies") && len(s) > 3:
		return s[:len(s)-3] + "y" // "categories" → "category"
	case strings.HasSuffix(s, "s") && len(s) > 1:
		// Simple dedup: "users" → "user", "receipts" → "receipt"
		// Exception: words that end in 's' but aren't plural (e.g. "status", "address")
		return s[:len(s)-1]
	default:
		return s
	}
}
