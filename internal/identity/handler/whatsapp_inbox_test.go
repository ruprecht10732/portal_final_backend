package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"portal_final_backend/internal/identity/service"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const expectedStatusMessage = "expected status %d, got %d"

func TestListWhatsAppMessagesByChatJIDRejectsInvalidLimit(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	router := newWhatsAppInboxTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/chat/31612345678@s.whatsapp.net/messages?limit=0", nil)

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(expectedStatusMessage, http.StatusBadRequest, recorder.Code)
	}
}

func TestListWhatsAppMessagesByChatJIDRejectsInvalidOffset(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	router := newWhatsAppInboxTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/chat/31612345678@s.whatsapp.net/messages?offset=-1", nil)

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(expectedStatusMessage, http.StatusBadRequest, recorder.Code)
	}
}

func TestListWhatsAppMessagesByChatJIDRejectsGroupChats(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	router := newWhatsAppInboxTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/chat/120363000000000000@g.us/messages", nil)

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(expectedStatusMessage, http.StatusBadRequest, recorder.Code)
	}
}

func newWhatsAppInboxTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &Handler{svc: &service.Service{}}
	userID := uuid.New()
	tenantID := uuid.New()
	router.Use(func(c *gin.Context) {
		c.Set(httpkit.ContextUserIDKey, userID)
		c.Set(httpkit.ContextTenantIDKey, tenantID)
		c.Next()
	})
	router.GET("/chat/:chatJID/messages", handler.ListWhatsAppMessagesByChatJID)
	return router
}