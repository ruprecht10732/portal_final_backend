package config

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env                   string
	HTTPAddr              string
	DatabaseURL           string
	JWTAccessSecret       string
	JWTRefreshSecret      string
	AccessTokenTTL        time.Duration
	RefreshTokenTTL       time.Duration
	VerifyTokenTTL        time.Duration
	ResetTokenTTL         time.Duration
	CORSAllowAll          bool
	CORSOrigins           []string
	CORSAllowCreds        bool
	AppBaseURL            string
	EmailEnabled          bool
	BrevoAPIKey           string
	EmailFromName         string
	EmailFromAddress      string
	RefreshCookieName     string
	RefreshCookieDomain   string
	RefreshCookiePath     string
	RefreshCookieSecure   bool
	RefreshCookieSameSite http.SameSite
}

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
		Env:                   getEnv("APP_ENV", "development"),
		HTTPAddr:              getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:           getEnv("DATABASE_URL", ""),
		JWTAccessSecret:       getEnv("JWT_ACCESS_SECRET", ""),
		JWTRefreshSecret:      getEnv("JWT_REFRESH_SECRET", ""),
		AccessTokenTTL:        mustDuration(getEnv("JWT_ACCESS_TTL", "15m")),
		RefreshTokenTTL:       mustDuration(getEnv("JWT_REFRESH_TTL", "720h")),
		VerifyTokenTTL:        mustDuration(getEnv("VERIFY_TOKEN_TTL", "30m")),
		ResetTokenTTL:         mustDuration(getEnv("RESET_TOKEN_TTL", "30m")),
		CORSAllowAll:          corsAllowAll,
		CORSOrigins:           corsOrigins,
		CORSAllowCreds:        strings.EqualFold(getEnv("CORS_ALLOW_CREDENTIALS", "true"), "true"),
		AppBaseURL:            getEnv("APP_BASE_URL", "http://localhost:4200"),
		EmailEnabled:          emailEnabled && brevoAPIKey != "",
		BrevoAPIKey:           brevoAPIKey,
		EmailFromName:         getEnv("EMAIL_FROM_NAME", "Portal"),
		EmailFromAddress:      getEnv("EMAIL_FROM_ADDRESS", ""),
		RefreshCookieName:     getEnv("REFRESH_COOKIE_NAME", "portal_refresh"),
		RefreshCookieDomain:   getEnv("REFRESH_COOKIE_DOMAIN", ""),
		RefreshCookiePath:     getEnv("REFRESH_COOKIE_PATH", "/api/v1/auth"),
		RefreshCookieSecure:   refreshCookieSecure,
		RefreshCookieSameSite: parseSameSite(getEnv("REFRESH_COOKIE_SAMESITE", "Lax")),
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
