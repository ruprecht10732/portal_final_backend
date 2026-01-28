package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env              string
	HTTPAddr         string
	DatabaseURL      string
	JWTAccessSecret  string
	JWTRefreshSecret string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	VerifyTokenTTL   time.Duration
	ResetTokenTTL    time.Duration
	CORSAllowAll     bool
	CORSOrigins      []string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	corsOrigins := splitCSV(getEnv("CORS_ORIGINS", "http://localhost:4200"))
	corsAllowAll := strings.EqualFold(getEnv("CORS_ALLOW_ALL", "false"), "true")
	if containsWildcard(corsOrigins) {
		corsAllowAll = true
	}

	cfg := &Config{
		Env:              getEnv("APP_ENV", "development"),
		HTTPAddr:         getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		JWTAccessSecret:  getEnv("JWT_ACCESS_SECRET", ""),
		JWTRefreshSecret: getEnv("JWT_REFRESH_SECRET", ""),
		AccessTokenTTL:   mustDuration(getEnv("JWT_ACCESS_TTL", "15m")),
		RefreshTokenTTL:  mustDuration(getEnv("JWT_REFRESH_TTL", "720h")),
		VerifyTokenTTL:   mustDuration(getEnv("VERIFY_TOKEN_TTL", "30m")),
		ResetTokenTTL:    mustDuration(getEnv("RESET_TOKEN_TTL", "30m")),
		CORSAllowAll:     corsAllowAll,
		CORSOrigins:      corsOrigins,
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTAccessSecret == "" || cfg.JWTRefreshSecret == "" {
		return nil, fmt.Errorf("JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required")
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
