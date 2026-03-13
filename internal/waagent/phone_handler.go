package waagent

import (
	"net/http"
	"strings"

	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

const errOrganizationRequired = "organization required"

// PhoneHandler manages agent phone-to-org registrations (org admin routes).
type PhoneHandler struct {
	queries waagentdb.Querier
}

type registerPhoneRequest struct {
	PhoneNumber string `json:"phoneNumber" binding:"required"`
	DisplayName string `json:"displayName"`
}

type phoneResponse struct {
	PhoneNumber string `json:"phoneNumber"`
	DisplayName string `json:"displayName"`
	CreatedAt   string `json:"createdAt"`
}

// RegisterAdminRoutes mounts phone management routes on the admin group.
func (h *PhoneHandler) RegisterAdminRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Register)
	rg.DELETE("/:phone", h.Remove)
}

// List returns all registered agent phone numbers for the organization.
func (h *PhoneHandler) List(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}

	rows, err := h.queries.ListAgentUsersByOrganization(c.Request.Context(), pgtype.UUID{Bytes: *tenantID, Valid: true})
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to list agent members", nil)
		return
	}

	result := make([]phoneResponse, len(rows))
	for i, row := range rows {
		result[i] = phoneResponse{
			PhoneNumber: row.PhoneNumber,
			DisplayName: row.DisplayName,
			CreatedAt:   row.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		}
	}
	httpkit.OK(c, gin.H{"members": result})
}

// Register adds a phone number to the agent for this organization.
func (h *PhoneHandler) Register(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}

	var req registerPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid request", nil)
		return
	}

	phone := strings.TrimSpace(req.PhoneNumber)
	if phone == "" {
		httpkit.Error(c, http.StatusBadRequest, "phoneNumber is required", nil)
		return
	}

	err := h.queries.CreateAgentUser(c.Request.Context(), waagentdb.CreateAgentUserParams{
		PhoneNumber:    phone,
		OrganizationID: pgtype.UUID{Bytes: *tenantID, Valid: true},
		DisplayName:    strings.TrimSpace(req.DisplayName),
	})
	if err != nil {
		httpkit.Error(c, http.StatusConflict, "phone number already registered", nil)
		return
	}

	httpkit.JSON(c, http.StatusCreated, gin.H{"phoneNumber": phone, "status": "registered"})
}

// Remove deletes a phone number registration for this organization.
func (h *PhoneHandler) Remove(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}

	phone := strings.TrimSpace(c.Param("phone"))
	if phone == "" {
		httpkit.Error(c, http.StatusBadRequest, "phone parameter required", nil)
		return
	}

	err := h.queries.DeleteAgentUser(c.Request.Context(), waagentdb.DeleteAgentUserParams{
		PhoneNumber:    phone,
		OrganizationID: pgtype.UUID{Bytes: *tenantID, Valid: true},
	})
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to remove agent member", nil)
		return
	}

	c.Status(http.StatusNoContent)
}
