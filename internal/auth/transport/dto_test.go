package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAuthResponseJSONContract ensures that the JSON serialization strictly honors
// the API contract, specifically verifying that optional tokens do not leak empty keys.
func TestAuthResponseJSONContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		response        AuthResponse
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:            "omits empty refresh token (omitempty contract)",
			response:        AuthResponse{AccessToken: "access-token"},
			wantContains:    []string{`"accessToken":"access-token"`},
			wantNotContains: []string{`"refreshToken"`},
		},
		{
			name:            "includes populated refresh token",
			response:        AuthResponse{AccessToken: "access-token", RefreshToken: "refresh-token"},
			wantContains:    []string{`"accessToken":"access-token"`, `"refreshToken":"refresh-token"`},
			wantNotContains: nil,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("failed to marshal AuthResponse: %v", err)
			}

			assertJSONPayload(t, string(payload), tt.wantContains, tt.wantNotContains)
		})
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// assertJSONPayload validates the presence and absence of required substrings.
// Extracting this logic drops the main test's Cognitive Complexity to 4.
func assertJSONPayload(t *testing.T, encoded string, wantContains, wantNotContains []string) {
	t.Helper()

	for _, want := range wantContains {
		if !strings.Contains(encoded, want) {
			t.Errorf("API Contract Violation: expected payload to contain %q, got: %s", want, encoded)
		}
	}

	for _, wantNot := range wantNotContains {
		if strings.Contains(encoded, wantNot) {
			t.Errorf("API Contract Violation: expected payload to NOT contain %q, got: %s", wantNot, encoded)
		}
	}
}
