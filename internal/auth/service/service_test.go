package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestSignJWTIncludesEmailSubjectAndExpiry(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	userID := uuid.New()
	tenantID := uuid.New()
	roles := []string{"admin", "user"}
	secret := "test-secret"

	tokenString, err := svc.signJWT(userID, "dev_user@company.com", &tenantID, roles, 15*time.Minute, accessTokenType, secret)
	if err != nil {
		t.Fatalf("signJWT returned error: %v", err)
	}

	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("failed to parse signed token: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("expected signed token to be valid")
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("expected jwt.MapClaims")
	}
	if got := claims["sub"]; got != userID.String() {
		t.Fatalf("expected sub=%q, got %v", userID.String(), got)
	}
	if got := claims["email"]; got != "dev_user@company.com" {
		t.Fatalf("expected email claim to be present, got %v", got)
	}
	if got := claims["type"]; got != accessTokenType {
		t.Fatalf("expected type=%q, got %v", accessTokenType, got)
	}
	if got := claims["tenant_id"]; got != tenantID.String() {
		t.Fatalf("expected tenant_id=%q, got %v", tenantID.String(), got)
	}
	if _, ok := claims["exp"].(float64); !ok {
		t.Fatalf("expected exp claim to be numeric, got %T", claims["exp"])
	}
	if _, ok := claims["iat"].(float64); !ok {
		t.Fatalf("expected iat claim to be numeric, got %T", claims["iat"])
	}
	if _, ok := claims["jti"].(string); !ok {
		t.Fatalf("expected jti claim to be string, got %T", claims["jti"])
	}

	claimRoles, ok := claims["roles"].([]interface{})
	if !ok {
		t.Fatalf("expected roles claim to be an array, got %T", claims["roles"])
	}
	if len(claimRoles) != len(roles) {
		t.Fatalf("expected %d roles, got %d", len(roles), len(claimRoles))
	}
}
