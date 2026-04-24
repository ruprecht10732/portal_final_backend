package transport

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDateUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantNil bool
	}{
		{
			name:  "date only",
			input: `{"validUntil":"2026-05-24"}`,
			want:  time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "rfc3339",
			input: `{"validUntil":"2026-05-24T10:30:00Z"}`,
			want:  time.Date(2026, 5, 24, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "rfc3339 with nano",
			input: `{"validUntil":"2026-05-24T10:30:00.123456789Z"}`,
			want:  time.Date(2026, 5, 24, 10, 30, 0, 123456789, time.UTC),
		},
		{
			name:    "null",
			input:   `{"validUntil":null}`,
			wantNil: true,
		},
		{
			name:    "missing",
			input:   `{}`,
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req UpdateQuoteRequest
			if err := json.Unmarshal([]byte(tc.input), &req); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if tc.wantNil {
				if req.ValidUntil != nil {
					t.Fatalf("expected nil, got %v", req.ValidUntil)
				}
				return
			}
			if req.ValidUntil == nil {
				t.Fatal("expected non-nil ValidUntil")
			}
			if !req.ValidUntil.Equal(tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, req.ValidUntil.Time)
			}
		})
	}
}
