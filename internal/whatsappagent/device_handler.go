package whatsappagent

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	apphttp "portal_final_backend/internal/http"
	"portal_final_backend/internal/whatsapp"
	whatsappagentdb "portal_final_backend/internal/whatsappagent/db"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const errNoAgentDeviceConfigured = "no agent device configured"

type agentConfigStore interface {
	GetAgentConfig(ctx context.Context) (whatsappagentdb.RacWhatsappAgentConfig, error)
	UpsertAgentConfig(ctx context.Context, arg whatsappagentdb.UpsertAgentConfigParams) (whatsappagentdb.RacWhatsappAgentConfig, error)
	DeleteAgentConfig(ctx context.Context) error
}

type agentDeviceTransport interface {
	CreateDevice(ctx context.Context, deviceID string) error
	GetLoginQR(ctx context.Context, deviceID string) ([]byte, error)
	GetDeviceStatus(ctx context.Context, deviceID string) (*whatsapp.DeviceStatusResponse, error)
	GetDeviceInfo(ctx context.Context, deviceID string) (*whatsapp.DeviceInfoResponse, error)
	ReconnectDevice(ctx context.Context, deviceID string) error
	DeleteDevice(ctx context.Context, deviceID string) error
}

// DeviceHandler manages the global WhatsApp agent device for superadmin routes.
type DeviceHandler struct {
	queries       agentConfigStore
	waClient      agentDeviceTransport
	webhookSecret string
}

func newDeviceHandler(queries agentConfigStore, waClient agentDeviceTransport, webhookSecret string) *DeviceHandler {
	if queries == nil || waClient == nil {
		return nil
	}
	return &DeviceHandler{queries: queries, waClient: waClient, webhookSecret: webhookSecret}
}

func (h *DeviceHandler) RegisterSuperAdminRoutes(rg *gin.RouterGroup) {
	rg.POST("/register", h.Register)
	rg.GET("/webhook-config", h.GetWebhookConfig)
	rg.GET("/qr", h.GetQR)
	rg.GET("/status", h.GetStatus)
	rg.POST("/reconnect", h.Reconnect)
	rg.DELETE("", h.Disconnect)
}

func (h *DeviceHandler) RegisterRoutes(ctx *apphttp.RouterContext) {
	if h == nil || ctx == nil || ctx.SuperAdmin == nil {
		return
	}
	agentDevice := ctx.SuperAdmin.Group("/whatsapp-agent")
	h.RegisterSuperAdminRoutes(agentDevice)
}

func (h *DeviceHandler) GetWebhookConfig(c *gin.Context) {
	httpkit.OK(c, gin.H{
		"secretHeaderName": "X-Webhook-Secret",
		"queryParamName":   "webhook_secret",
		"sharedSecret":     strings.TrimSpace(h.webhookSecret),
	})
}

func (h *DeviceHandler) Register(c *gin.Context) {
	deviceID := fmt.Sprintf("agent_%s", uuid.NewString()[:8])

	if err := h.waClient.CreateDevice(c.Request.Context(), deviceID); err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to create device with provider", nil)
		return
	}

	cfg, err := h.queries.UpsertAgentConfig(c.Request.Context(), whatsappagentdb.UpsertAgentConfigParams{
		DeviceID:   deviceID,
		AccountJid: pgtype.Text{},
	})
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to save agent config", nil)
		return
	}

	httpkit.JSON(c, http.StatusCreated, gin.H{
		"deviceId":  cfg.DeviceID,
		"createdAt": cfg.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
	})
}

func (h *DeviceHandler) GetQR(c *gin.Context) {
	cfg, err := h.queries.GetAgentConfig(c.Request.Context())
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, errNoAgentDeviceConfigured, nil)
		return
	}

	qrBytes, err := h.waClient.GetLoginQR(c.Request.Context(), cfg.DeviceID)
	if err != nil {
		httpkit.Error(c, http.StatusBadGateway, "failed to get QR from provider", nil)
		return
	}

	c.Data(http.StatusOK, "image/png", qrBytes)
}

func (h *DeviceHandler) GetStatus(c *gin.Context) {
	cfg, err := h.queries.GetAgentConfig(c.Request.Context())
	if err != nil {
		httpkit.OK(c, gin.H{"state": "UNREGISTERED"})
		return
	}

	status, err := h.waClient.GetDeviceStatus(c.Request.Context(), cfg.DeviceID)
	if err != nil {
		httpkit.OK(c, gin.H{
			"state":    "ERROR",
			"deviceId": cfg.DeviceID,
			"error":    err.Error(),
		})
		return
	}

	resp := gin.H{
		"state":    "DISCONNECTED",
		"deviceId": cfg.DeviceID,
	}
	if status.IsLoggedIn {
		resp["state"] = "CONNECTED"

		info, infoErr := h.waClient.GetDeviceInfo(c.Request.Context(), cfg.DeviceID)
		if infoErr == nil && info != nil {
			jid := strings.TrimSpace(info.JID)
			if jid != "" {
				resp["accountJid"] = jid
				_, _ = h.queries.UpsertAgentConfig(c.Request.Context(), whatsappagentdb.UpsertAgentConfigParams{
					DeviceID:   cfg.DeviceID,
					AccountJid: pgtype.Text{String: jid, Valid: true},
				})
			}
		}
	}

	httpkit.OK(c, resp)
}

func (h *DeviceHandler) Reconnect(c *gin.Context) {
	cfg, err := h.queries.GetAgentConfig(c.Request.Context())
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, errNoAgentDeviceConfigured, nil)
		return
	}

	if err := h.waClient.ReconnectDevice(c.Request.Context(), cfg.DeviceID); err != nil {
		httpkit.Error(c, http.StatusBadGateway, "failed to reconnect device", nil)
		return
	}

	httpkit.OK(c, gin.H{"status": "reconnecting", "deviceId": cfg.DeviceID})
}

func (h *DeviceHandler) Disconnect(c *gin.Context) {
	cfg, err := h.queries.GetAgentConfig(c.Request.Context())
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, errNoAgentDeviceConfigured, nil)
		return
	}

	_ = h.waClient.DeleteDevice(c.Request.Context(), cfg.DeviceID)

	if err := h.queries.DeleteAgentConfig(c.Request.Context()); err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to clear agent config", nil)
		return
	}

	c.Status(http.StatusNoContent)
}
