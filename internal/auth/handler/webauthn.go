package handler

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"

	"portal_final_backend/internal/auth/transport"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
)

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
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	credJSON, err := json.Marshal(req.Credential)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if err := h.svc.FinishPasskeyRegistration(c.Request.Context(), id.UserID(), req.Nickname, credJSON); err != nil {
		httpkit.HandleError(c, err)
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
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<16))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var envelope struct {
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Challenge == "" || len(envelope.Credential) == 0 {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	accessToken, refreshToken, err := h.svc.FinishPasskeyLogin(c.Request.Context(), envelope.Challenge, envelope.Credential)
	if httpkit.HandleError(c, err) {
		return
	}

	h.setRefreshCookie(c, refreshToken)
	httpkit.OK(c, transport.AuthResponse{AccessToken: accessToken, RefreshToken: refreshToken})
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

	credID, err := base64.URLEncoding.DecodeString(c.Param("credentialId"))
	if err != nil || len(credID) == 0 {
		httpkit.Error(c, http.StatusBadRequest, "invalid credential id", nil)
		return
	}

	var req transport.RenamePasskeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
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

	credID, err := base64.URLEncoding.DecodeString(c.Param("credentialId"))
	if err != nil || len(credID) == 0 {
		httpkit.Error(c, http.StatusBadRequest, "invalid credential id", nil)
		return
	}

	if httpkit.HandleError(c, h.svc.DeletePasskey(c.Request.Context(), id.UserID(), credID)) {
		return
	}

	httpkit.OK(c, gin.H{"message": "passkey deleted"})
}
