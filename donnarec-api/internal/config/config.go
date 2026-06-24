// Package config loads donnarec-api configuration from environment variables.
// All configuration is read at startup; no hot-reload in MVP.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds the full application configuration loaded from environment variables.
type Config struct {
	// Server
	Port string

	// Database
	DatabaseURL string

	// Keycloak (OIDC authN)
	KeycloakBaseURL  string
	KeycloakRealm    string
	KeycloakClientID string

	// Encryption (PDPA — NFR-02)
	// DonarecKEK is the hex-encoded 32-byte Key Encryption Key for envelope encryption.
	// Must be exactly 64 hex characters. Kept in env (D-26 MVP); migrate to KMS later.
	DonarecKEK string

	// Retention (D-18)
	Retention RetentionConfig
}

// RetentionConfig holds data-retention policy defaults loaded from environment.
// These are defaults; per-record retain_until is derived at insert time.
// Confirm final values with DPO before production rollout (D-18 note).
type RetentionConfig struct {
	// DonationRetainDays: how many days to retain donation records (default: 1825 = 5 years)
	DonationRetainDays int
	// AuditLogRetainDays: how many days to retain audit log entries (default: 3650 = 10 years)
	AuditLogRetainDays int
	// DefaultLegalBasis: legal basis enum value for new records (default: "tax_obligation")
	DefaultLegalBasis string
}

// Load reads configuration from environment variables and returns a Config.
// Returns an error if any required variable is missing or invalid.
func Load() (*Config, error) {
	cfg := &Config{
		Port:             getEnvStr("PORT", "8000"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		KeycloakBaseURL:  os.Getenv("KEYCLOAK_BASE_URL"),
		KeycloakRealm:    getEnvStr("KEYCLOAK_REALM", "donnarec"),
		KeycloakClientID: getEnvStr("KEYCLOAK_CLIENT_ID", "donnarec-backend"),
		DonarecKEK:       os.Getenv("DONAREC_KEK"),
		Retention: RetentionConfig{
			DonationRetainDays: getEnvInt("RETENTION_DONATION_DAYS", 1825),
			AuditLogRetainDays: getEnvInt("RETENTION_AUDIT_DAYS", 3650),
			DefaultLegalBasis:  getEnvStr("RETENTION_DEFAULT_LEGAL_BASIS", "tax_obligation"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate checks that all required fields are set.
func (c *Config) validate() error {
	required := map[string]string{
		"DATABASE_URL":      c.DatabaseURL,
		"KEYCLOAK_BASE_URL": c.KeycloakBaseURL,
		"DONAREC_KEK":       c.DonarecKEK,
	}
	for name, val := range required {
		if val == "" {
			return fmt.Errorf("required environment variable %s is not set", name)
		}
	}

	if len(c.DonarecKEK) != 64 {
		return fmt.Errorf("DONAREC_KEK must be exactly 64 hex characters (32 bytes); got %d characters", len(c.DonarecKEK))
	}

	return nil
}

// InsecureDatabaseTLS reports whether the configured DATABASE_URL uses
// sslmode=disable against a NON-localhost host (IN-04). That combination is a
// PDPA/NFR-02 risk: traffic to a remote Postgres would be unencrypted. It is
// acceptable only for the local docker-compose stack (localhost/127.0.0.1/::1).
//
// Returns (insecure, host). insecure is false when the URL is unparseable,
// when sslmode is not "disable", or when the host is local.
func (c *Config) InsecureDatabaseTLS() (bool, string) {
	u, err := url.Parse(c.DatabaseURL)
	if err != nil {
		// Can't parse — don't claim insecurity we can't prove.
		return false, ""
	}

	sslmode := u.Query().Get("sslmode")
	if !strings.EqualFold(sslmode, "disable") {
		return false, u.Hostname()
	}

	host := u.Hostname()
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1", "":
		// Local dev — sslmode=disable is acceptable here.
		return false, host
	default:
		return true, host
	}
}

func getEnvStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
