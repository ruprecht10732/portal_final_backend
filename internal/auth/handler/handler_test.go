package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformvalidator "portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
)

type testCookieConfig struct{}

const errDecodeResponseBody = "failed to decode response body: %v"

func (testCookieConfig) GetRefreshCookieName() string            { return "refresh_token" }
func (testCookieConfig) GetRefreshCookieDomain() string          { return "" }
func (testCookieConfig) GetRefreshCookiePath() string            { return "/" }
func (testCookieConfig) GetRefreshCookieSecure() bool            { return false }
func (testCookieConfig) GetRefreshCookieSameSite() http.SameSite { return http.SameSiteLaxMode }
func (testCookieConfig) GetRefreshTokenTTL() time.Duration       { return 24 * time.Hour }

func newTestHandler() *Handler {
	return New(nil, testCookieConfig{}, platformvalidator.New())
}

func TestRefreshReturnsUnauthorizedWhenNoTokenProvided(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{}`))
	context.Request.Header.Set("Content-Type", "application/json")

	newTestHandler().Refresh(context)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf(errDecodeResponseBody, err)
	}
	if response["error"] != "token invalid" {
		t.Fatalf("expected token invalid error, got %+v", response)
	}
}

func TestSignOutClearsRefreshCookieWithoutRefreshToken(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/auth/sign-out", strings.NewReader(`{}`))
	context.Request.Header.Set("Content-Type", "application/json")

	newTestHandler().SignOut(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	setCookie := recorder.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "refresh_token=") {
		t.Fatalf("expected refresh cookie to be cleared, got %q", setCookie)
	}
	if !strings.Contains(setCookie, "Max-Age=0") && !strings.Contains(setCookie, "Max-Age=-1") {
		t.Fatalf("expected refresh cookie expiration, got %q", setCookie)
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf(errDecodeResponseBody, err)
	}
	if response["message"] != "signed out" {
		t.Fatalf("expected signed out message, got %+v", response)
	}
}

func TestVerifyReturnsUnauthorizedWithoutIdentity(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/auth/verify", nil)

	newTestHandler().Verify(context)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf(errDecodeResponseBody, err)
	}
	if response["error"] != "unauthorized" {
		t.Fatalf("expected unauthorized error, got %+v", response)
	}
}
