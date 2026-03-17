package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuthResponseOmitsEmptyRefreshToken(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(AuthResponse{AccessToken: "access-token"})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	encoded := string(payload)
	if strings.Contains(encoded, "refreshToken") {
		t.Fatalf("expected refreshToken to be omitted when empty, got %s", encoded)
	}
	if !strings.Contains(encoded, `"accessToken":"access-token"`) {
		t.Fatalf("expected accessToken in payload, got %s", encoded)
	}
}

func TestAuthResponseIncludesRefreshTokenWhenPresent(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(AuthResponse{AccessToken: "access-token", RefreshToken: "refresh-token"})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	encoded := string(payload)
	if !strings.Contains(encoded, `"refreshToken":"refresh-token"`) {
		t.Fatalf("expected refreshToken in payload, got %s", encoded)
	}
}
