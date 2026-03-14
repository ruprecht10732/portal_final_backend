package httpkit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"portal_final_backend/platform/logger"

	"github.com/gin-gonic/gin"
)

func TestRequestCorrelationGeneratesRequestAndTraceIDs(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestCorrelation())
	engine.GET("/test", func(c *gin.Context) {
		requestID, _ := c.Request.Context().Value(logger.RequestIDKey).(string)
		traceID, _ := c.Request.Context().Value(logger.TraceIDKey).(string)
		c.JSON(http.StatusOK, gin.H{"requestId": requestID, "traceId": traceID})
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	requestID := recorder.Header().Get(headerRequestID)
	traceID := recorder.Header().Get(headerTraceID)
	if requestID == "" {
		t.Fatal("expected request id header to be set")
	}
	if traceID == "" {
		t.Fatal("expected trace id header to be set")
	}
	if traceID != requestID {
		t.Fatalf("expected generated trace id to match request id, got request=%q trace=%q", requestID, traceID)
	}
}

func TestRequestCorrelationPreservesInboundHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestCorrelation())
	engine.GET("/test", func(c *gin.Context) {
		requestID, _ := c.Request.Context().Value(logger.RequestIDKey).(string)
		traceID, _ := c.Request.Context().Value(logger.TraceIDKey).(string)
		c.JSON(http.StatusOK, gin.H{"requestId": requestID, "traceId": traceID})
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(headerRequestID, "req-123")
	req.Header.Set(headerTraceID, "trace-456")
	engine.ServeHTTP(recorder, req)

	if recorder.Header().Get(headerRequestID) != "req-123" {
		t.Fatalf("expected request id header to be preserved, got %q", recorder.Header().Get(headerRequestID))
	}
	if recorder.Header().Get(headerTraceID) != "trace-456" {
		t.Fatalf("expected trace id header to be preserved, got %q", recorder.Header().Get(headerTraceID))
	}
}
