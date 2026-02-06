// Package config provides application configuration loading.
// This is part of the platform layer and contains no business logic.
package config

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// =============================================================================
// Module-Specific Config Interfaces (Principle of Least Privilege)
// =============================================================================

// DatabaseConfig provides database connection settings.
type DatabaseConfig interface {
	GetDatabaseURL() string
}

// JWTConfig provides JWT validation settings for middleware.
type JWTConfig interface {
	GetJWTAccessSecret() string
}

// AuthServiceConfig provides settings needed by the auth service.
type AuthServiceConfig interface {
	JWTConfig
	GetAccessTokenTTL() time.Duration
	GetRefreshTokenTTL() time.Duration
	GetVerifyTokenTTL() time.Duration
	GetResetTokenTTL() time.Duration
}

// CookieConfig provides settings for refresh token cookies.
type CookieConfig interface {
	GetRefreshCookieName() string
	GetRefreshCookieDomain() string
	GetRefreshCookiePath() string
	GetRefreshCookieSecure() bool
	GetRefreshCookieSameSite() http.SameSite
	GetRefreshTokenTTL() time.Duration
}

// EmailConfig provides settings for email sending.
type EmailConfig interface {
	GetEmailEnabled() bool
	GetBrevoAPIKey() string
	GetEmailFromName() string
	GetEmailFromAddress() string
}

// NotificationConfig provides settings for the notification module.
type NotificationConfig interface {
	GetAppBaseURL() string
}

// HTTPConfig provides settings for the HTTP server.
type HTTPConfig interface {
	GetHTTPAddr() string
	GetCORSAllowAll() bool
	GetCORSOrigins() []string
	GetCORSAllowCreds() bool
}

// MinIOConfig provides settings for MinIO S3-compatible storage.
type MinIOConfig interface {
	GetMinIOEndpoint() string
	GetMinIOAccessKey() string
	GetMinIOSecretKey() string
	GetMinIOUseSSL() bool
	GetMinIOMaxFileSize() int64
	GetMinioBucketLeadServiceAttachments() string
	GetMinioBucketCatalogAssets() string
	GetMinioBucketPartnerLogos() string
	GetMinioBucketOrganizationLogos() string
	GetMinioBucketQuotePDFs() string
	IsMinIOEnabled() bool
}

// GotenbergConfig provides settings for the Gotenberg HTML-to-PDF service.
type GotenbergConfig interface {
	GetGotenbergURL() string
	GetGotenbergUsername() string
	GetGotenbergPassword() string
	IsGotenbergEnabled() bool
}

// EnergyLabelConfig provides settings for EP-Online energy label API.
type EnergyLabelConfig interface {
	GetEPOnlineAPIKey() string
	IsEnergyLabelEnabled() bool
}

// QdrantConfig provides settings for Qdrant vector database.
type QdrantConfig interface {
	GetQdrantURL() string
	GetQdrantAPIKey() string
	GetQdrantCollection() string
	IsQdrantEnabled() bool
}

// EmbeddingConfig provides settings for the embedding API service.
type EmbeddingConfig interface {
	GetEmbeddingAPIURL() string
	GetEmbeddingAPIKey() string
	IsEmbeddingEnabled() bool
}

// CatalogEmbeddingConfig provides settings for catalog embedding indexing.
type CatalogEmbeddingConfig interface {
	GetCatalogEmbeddingAPIURL() string
	GetCatalogEmbeddingAPIKey() string
	GetCatalogEmbeddingCollection() string
	IsCatalogEmbeddingEnabled() bool
}

// =============================================================================
// Main Config Struct
// =============================================================================

// Config holds all application configuration values.
type Config struct {
	Env                               string
	HTTPAddr                          string
	DatabaseURL                       string
	JWTAccessSecret                   string
	JWTRefreshSecret                  string
	AccessTokenTTL                    time.Duration
	RefreshTokenTTL                   time.Duration
	VerifyTokenTTL                    time.Duration
	ResetTokenTTL                     time.Duration
	CORSAllowAll                      bool
	CORSOrigins                       []string
	CORSAllowCreds                    bool
	AppBaseURL                        string
	EmailEnabled                      bool
	BrevoAPIKey                       string
	EmailFromName                     string
	EmailFromAddress                  string
	RefreshCookieName                 string
	RefreshCookieDomain               string
	RefreshCookiePath                 string
	RefreshCookieSecure               bool
	RefreshCookieSameSite             http.SameSite
	MoonshotAPIKey                    string
	EPOnlineAPIKey                    string
	MinIOEndpoint                     string
	MinIOAccessKey                    string
	MinIOSecretKey                    string
	MinIOUseSSL                       bool
	MinIOMaxFileSize                  int64
	MinioBucketLeadServiceAttachments string
	MinioBucketCatalogAssets          string
	MinioBucketPartnerLogos           string
	MinioBucketOrganizationLogos      string
	MinioBucketQuotePDFs              string
	GotenbergURL                      string
	GotenbergUsername                  string
	GotenbergPassword                  string
	QdrantURL                         string
	QdrantAPIKey                      string
	QdrantCollection                  string
	EmbeddingAPIURL                   string
	EmbeddingAPIKey                   string
	CatalogEmbeddingAPIURL            string
	CatalogEmbeddingAPIKey            string
	CatalogEmbeddingCollection        string
}

// =============================================================================
// Interface Implementations
// =============================================================================

// DatabaseConfig implementation
func (c *Config) GetDatabaseURL() string { return c.DatabaseURL }

// JWTConfig implementation
func (c *Config) GetJWTAccessSecret() string { return c.JWTAccessSecret }

// AuthServiceConfig implementation
func (c *Config) GetAccessTokenTTL() time.Duration  { return c.AccessTokenTTL }
func (c *Config) GetRefreshTokenTTL() time.Duration { return c.RefreshTokenTTL }
func (c *Config) GetVerifyTokenTTL() time.Duration  { return c.VerifyTokenTTL }
func (c *Config) GetResetTokenTTL() time.Duration   { return c.ResetTokenTTL }

// CookieConfig implementation
func (c *Config) GetRefreshCookieName() string            { return c.RefreshCookieName }
func (c *Config) GetRefreshCookieDomain() string          { return c.RefreshCookieDomain }
func (c *Config) GetRefreshCookiePath() string            { return c.RefreshCookiePath }
func (c *Config) GetRefreshCookieSecure() bool            { return c.RefreshCookieSecure }
func (c *Config) GetRefreshCookieSameSite() http.SameSite { return c.RefreshCookieSameSite }

// EmailConfig implementation
func (c *Config) GetEmailEnabled() bool       { return c.EmailEnabled }
func (c *Config) GetBrevoAPIKey() string      { return c.BrevoAPIKey }
func (c *Config) GetEmailFromName() string    { return c.EmailFromName }
func (c *Config) GetEmailFromAddress() string { return c.EmailFromAddress }

// NotificationConfig implementation
func (c *Config) GetAppBaseURL() string { return c.AppBaseURL }

// HTTPConfig implementation
func (c *Config) GetHTTPAddr() string      { return c.HTTPAddr }
func (c *Config) GetCORSAllowAll() bool    { return c.CORSAllowAll }
func (c *Config) GetCORSOrigins() []string { return c.CORSOrigins }
func (c *Config) GetCORSAllowCreds() bool  { return c.CORSAllowCreds }

// MinIOConfig implementation
func (c *Config) GetMinIOEndpoint() string   { return c.MinIOEndpoint }
func (c *Config) GetMinIOAccessKey() string  { return c.MinIOAccessKey }
func (c *Config) GetMinIOSecretKey() string  { return c.MinIOSecretKey }
func (c *Config) GetMinIOUseSSL() bool       { return c.MinIOUseSSL }
func (c *Config) GetMinIOMaxFileSize() int64 { return c.MinIOMaxFileSize }
func (c *Config) GetMinioBucketLeadServiceAttachments() string {
	return c.MinioBucketLeadServiceAttachments
}
func (c *Config) GetMinioBucketCatalogAssets() string {
	return c.MinioBucketCatalogAssets
}
func (c *Config) GetMinioBucketPartnerLogos() string {
	return c.MinioBucketPartnerLogos
}
func (c *Config) GetMinioBucketOrganizationLogos() string {
	return c.MinioBucketOrganizationLogos
}
func (c *Config) GetMinioBucketQuotePDFs() string {
	return c.MinioBucketQuotePDFs
}
func (c *Config) IsMinIOEnabled() bool { return c.MinIOEndpoint != "" }

// GotenbergConfig implementation
func (c *Config) GetGotenbergURL() string      { return c.GotenbergURL }
func (c *Config) GetGotenbergUsername() string  { return c.GotenbergUsername }
func (c *Config) GetGotenbergPassword() string  { return c.GotenbergPassword }
func (c *Config) IsGotenbergEnabled() bool      { return c.GotenbergURL != "" }

// EnergyLabelConfig implementation
func (c *Config) GetEPOnlineAPIKey() string  { return c.EPOnlineAPIKey }
func (c *Config) IsEnergyLabelEnabled() bool { return c.EPOnlineAPIKey != "" }

// QdrantConfig implementation
func (c *Config) GetQdrantURL() string        { return c.QdrantURL }
func (c *Config) GetQdrantAPIKey() string     { return c.QdrantAPIKey }
func (c *Config) GetQdrantCollection() string { return c.QdrantCollection }
func (c *Config) IsQdrantEnabled() bool {
	return c.QdrantURL != "" && c.QdrantCollection != ""
}

// EmbeddingConfig implementation
func (c *Config) GetEmbeddingAPIURL() string { return c.EmbeddingAPIURL }
func (c *Config) GetEmbeddingAPIKey() string { return c.EmbeddingAPIKey }
func (c *Config) IsEmbeddingEnabled() bool   { return c.EmbeddingAPIURL != "" }

// CatalogEmbeddingConfig implementation
func (c *Config) GetCatalogEmbeddingAPIURL() string { return c.CatalogEmbeddingAPIURL }
func (c *Config) GetCatalogEmbeddingAPIKey() string { return c.CatalogEmbeddingAPIKey }
func (c *Config) GetCatalogEmbeddingCollection() string {
	return c.CatalogEmbeddingCollection
}
func (c *Config) IsCatalogEmbeddingEnabled() bool {
	return c.CatalogEmbeddingAPIURL != ""
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	_ = godotenv.Load()

	corsOrigins := splitCSV(getEnv("CORS_ORIGINS", "http://localhost:4200"))
	corsAllowAll := strings.EqualFold(getEnv("CORS_ALLOW_ALL", "false"), "true")
	if containsWildcard(corsOrigins) {
		corsAllowAll = true
	}

	brevoAPIKey := getEnv("BREVO_API_KEY", "")
	emailEnabled := strings.EqualFold(getEnv("EMAIL_ENABLED", "true"), "true")

	refreshCookieSecure := strings.EqualFold(getEnv("REFRESH_COOKIE_SECURE", ""), "true")
	if getEnv("REFRESH_COOKIE_SECURE", "") == "" {
		refreshCookieSecure = strings.EqualFold(getEnv("APP_ENV", "development"), "production")
	}

	cfg := &Config{
		Env:                               getEnv("APP_ENV", "development"),
		HTTPAddr:                          getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:                       getEnv("DATABASE_URL", ""),
		JWTAccessSecret:                   getEnv("JWT_ACCESS_SECRET", ""),
		JWTRefreshSecret:                  getEnv("JWT_REFRESH_SECRET", ""),
		AccessTokenTTL:                    mustDuration(getEnv("JWT_ACCESS_TTL", "15m")),
		RefreshTokenTTL:                   mustDuration(getEnv("JWT_REFRESH_TTL", "720h")),
		VerifyTokenTTL:                    mustDuration(getEnv("VERIFY_TOKEN_TTL", "30m")),
		ResetTokenTTL:                     mustDuration(getEnv("RESET_TOKEN_TTL", "30m")),
		CORSAllowAll:                      corsAllowAll,
		CORSOrigins:                       corsOrigins,
		CORSAllowCreds:                    strings.EqualFold(getEnv("CORS_ALLOW_CREDENTIALS", "true"), "true"),
		AppBaseURL:                        getEnv("APP_BASE_URL", "http://localhost:4200"),
		EmailEnabled:                      emailEnabled && brevoAPIKey != "",
		BrevoAPIKey:                       brevoAPIKey,
		EmailFromName:                     getEnv("EMAIL_FROM_NAME", "Portal"),
		EmailFromAddress:                  getEnv("EMAIL_FROM_ADDRESS", ""),
		RefreshCookieName:                 getEnv("REFRESH_COOKIE_NAME", "portal_refresh"),
		RefreshCookieDomain:               getEnv("REFRESH_COOKIE_DOMAIN", ""),
		RefreshCookiePath:                 getEnv("REFRESH_COOKIE_PATH", "/api/v1/auth"),
		RefreshCookieSecure:               refreshCookieSecure,
		RefreshCookieSameSite:             parseSameSite(getEnv("REFRESH_COOKIE_SAMESITE", "Lax")),
		MoonshotAPIKey:                    getEnv("MOONSHOT_API_KEY", ""),
		EPOnlineAPIKey:                    getEnv("EP_ONLINE_API_KEY", ""),
		MinIOEndpoint:                     getEnv("MINIO_ENDPOINT", ""),
		MinIOAccessKey:                    getEnv("MINIO_ACCESS_KEY", ""),
		MinIOSecretKey:                    getEnv("MINIO_SECRET_KEY", ""),
		MinIOUseSSL:                       strings.EqualFold(getEnv("MINIO_USE_SSL", "false"), "true"),
		MinIOMaxFileSize:                  mustInt64(getEnv("MINIO_MAX_FILE_SIZE", "104857600")),
		MinioBucketLeadServiceAttachments: getEnv("MINIO_BUCKET_LEAD_SERVICE_ATTACHMENTS", "lead-service-attachments"),
		MinioBucketCatalogAssets:          getEnv("MINIO_BUCKET_CATALOG_ASSETS", "catalog-assets"),
		MinioBucketPartnerLogos:           getEnv("MINIO_BUCKET_PARTNER_LOGOS", "partner-logos"),
		MinioBucketOrganizationLogos:      getEnv("MINIO_BUCKET_ORGANIZATION_LOGOS", "organization-logos"),
		MinioBucketQuotePDFs:              getEnv("MINIO_BUCKET_QUOTE_PDFS", "quote-pdfs"),
		GotenbergURL:                      getEnv("GOTENBERG_URL", ""),
		GotenbergUsername:                  getEnv("GOTENBERG_USERNAME", ""),
		GotenbergPassword:                  getEnv("GOTENBERG_PASSWORD", ""),
		QdrantURL:                         getEnv("QDRANT_URL", ""),
		QdrantAPIKey:                      getEnv("QDRANT_API_KEY", ""),
		QdrantCollection:                  getEnv("QDRANT_COLLECTION", ""),
		EmbeddingAPIURL:                   getEnv("EMBEDDING_API_URL", ""),
		EmbeddingAPIKey:                   getEnv("EMBEDDING_API_KEY", ""),
		CatalogEmbeddingAPIURL:            getEnv("CATALOG_EMBEDDING_API_URL", ""),
		CatalogEmbeddingAPIKey:            getEnv("CATALOG_EMBEDDING_API_KEY", ""),
		CatalogEmbeddingCollection:        getEnv("CATALOG_EMBEDDING_COLLECTION", "catalog"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTAccessSecret == "" || cfg.JWTRefreshSecret == "" {
		return nil, fmt.Errorf("JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required")
	}
	if emailEnabled && cfg.BrevoAPIKey == "" {
		return nil, fmt.Errorf("BREVO_API_KEY is required when EMAIL_ENABLED is true")
	}
	if cfg.EmailEnabled && cfg.EmailFromAddress == "" {
		return nil, fmt.Errorf("EMAIL_FROM_ADDRESS is required when email is enabled")
	}
	if cfg.CORSAllowAll && cfg.CORSAllowCreds {
		return nil, fmt.Errorf("CORS_ALLOW_CREDENTIALS cannot be true when CORS_ALLOW_ALL is true")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func mustDuration(value string) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}
	return d
}

func mustInt64(value string) int64 {
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return result
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	results := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			results = append(results, trimmed)
		}
	}
	return results
}

func containsWildcard(values []string) bool {
	for _, value := range values {
		if value == "*" {
			return true
		}
	}
	return false
}

func parseSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}
