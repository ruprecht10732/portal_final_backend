package whatsapp

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"portal_final_backend/platform/logger"
)

func TestGetDeviceInfoReturnsAccountJID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/devices/org_test" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Device-Id"); got != "org_test" {
			t.Fatalf("expected X-Device-Id header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":200,"code":"SUCCESS","results":{"id":"org_test","display_name":"Robin","jid":"31619330634@s.whatsapp.net","state":"logged_in"}}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL:           server.URL,
		baseHost:          server.Listener.Addr().String(),
		apiKey:            "secret",
		apiKeyFingerprint: "fp",
		http:              &http.Client{Timeout: time.Second},
		log:               logger.New("development"),
	}

	info, err := client.GetDeviceInfo(context.Background(), "org_test")
	if err != nil {
		t.Fatalf("GetDeviceInfo returned error: %v", err)
	}
	if info.DeviceID != "org_test" {
		t.Fatalf("expected device id org_test, got %q", info.DeviceID)
	}
	if info.JID != "31619330634@s.whatsapp.net" {
		t.Fatalf("expected JID to be parsed, got %q", info.JID)
	}
	if info.State != "logged_in" {
		t.Fatalf("expected state logged_in, got %q", info.State)
	}
}

func TestFormatAuthHeaderUsesBasicPrefix(t *testing.T) {
	t.Parallel()

	got := formatAuthHeader("abc123")
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("abc123"))
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
