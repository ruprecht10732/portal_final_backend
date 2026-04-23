package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"portal_final_backend/internal/auth/transport"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *Handler) bind(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return false
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return false
	}
	return true
}

func getCredID(c *gin.Context) []byte {
	credID, err := base64.URLEncoding.DecodeString(c.Param("credentialId"))
	if err != nil || len(credID) == 0 {
		httpkit.Error(c, http.StatusBadRequest, "invalid credential id", nil)
		return nil
	}
	return credID
}

// ---------------------------------------------------------------------------
// Registration (protected – user adding a passkey)
// ---------------------------------------------------------------------------

func (h *Handler) BeginPasskeyRegistration(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	options, err := h.svc.BeginPasskeyRegistration(c.Request.Context(), id.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, options)
}

func (h *Handler) FinishPasskeyRegistration(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	var req transport.FinishPasskeyRegistrationRequest
	if !h.bind(c, &req) {
		return
	}

	credJSON, err := json.Marshal(req.Credential)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if httpkit.HandleError(c, h.svc.FinishPasskeyRegistration(c.Request.Context(), id.UserID(), req.Nickname, credJSON)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "passkey registered"})
}

// ---------------------------------------------------------------------------
// Login (public – username-less passkey login)
// ---------------------------------------------------------------------------

func (h *Handler) BeginPasskeyLogin(c *gin.Context) {
	options, err := h.svc.BeginPasskeyLogin(c.Request.Context())
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, options)
}

func (h *Handler) FinishPasskeyLogin(c *gin.Context) {
	// Safely and efficiently limit body size while using native JSON binding
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<16)

	var req struct {
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Challenge == "" || len(req.Credential) == 0 {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	access, refresh, err := h.svc.FinishPasskeyLogin(c.Request.Context(), req.Challenge, req.Credential)
	if httpkit.HandleError(c, err) {
		return
	}

	h.setRefreshCookie(c, refresh)
	httpkit.OK(c, transport.AuthResponse{AccessToken: access, RefreshToken: refresh})
}

// ---------------------------------------------------------------------------
// Management (protected – list, rename, delete passkeys)
// ---------------------------------------------------------------------------

func (h *Handler) ListPasskeys(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	passkeys, err := h.svc.ListPasskeys(c.Request.Context(), id.UserID())
	if httpkit.HandleError(c, err) {
		return
	}

	resp := make([]transport.PasskeyResponse, len(passkeys))
	for i, p := range passkeys {
		resp[i] = transport.PasskeyResponse{
			ID:         base64.URLEncoding.EncodeToString(p.ID),
			Nickname:   p.Nickname,
			CreatedAt:  p.CreatedAt,
			LastUsedAt: p.LastUsedAt,
		}
	}

	httpkit.OK(c, resp)
}

func (h *Handler) RenamePasskey(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	credID := getCredID(c)
	if credID == nil {
		return
	}

	var req transport.RenamePasskeyRequest
	if !h.bind(c, &req) {
		return
	}

	if httpkit.HandleError(c, h.svc.RenamePasskey(c.Request.Context(), id.UserID(), credID, req.Nickname)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "passkey renamed"})
}

func (h *Handler) DeletePasskey(c *gin.Context) {
	id := httpkit.MustGetIdentity(c)
	if id == nil {
		return
	}

	credID := getCredID(c)
	if credID == nil {
		return
	}

	if httpkit.HandleError(c, h.svc.DeletePasskey(c.Request.Context(), id.UserID(), credID)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "passkey deleted"})
}
