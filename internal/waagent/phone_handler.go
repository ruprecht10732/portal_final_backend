package waagent

import (
	"net/http"
	"strings"

	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/platform/httpkit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const errOrganizationRequired = "organization required"

const errListAgentMembersFailed = "failed to list agent members"

// PhoneHandler manages agent phone-to-org registrations (org admin routes).
type PhoneHandler struct {
	queries            waagentdb.Querier
	partnerPhoneReader PartnerPhoneReader
}

type registerPhoneRequest struct {
	PhoneNumber string `json:"phoneNumber" binding:"required"`
	DisplayName string `json:"displayName"`
}

type phoneResponse struct {
	PhoneNumber string `json:"phoneNumber"`
	DisplayName string `json:"displayName"`
	UserType    string `json:"userType"`
	PartnerID   string `json:"partnerId,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

type registerPartnerPhoneRequest struct {
	PartnerID string `json:"partnerId" binding:"required"`
}

// RegisterAdminRoutes mounts phone management routes on the admin group.
func (h *PhoneHandler) RegisterAdminRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Register)
	rg.DELETE("/:phone", h.Remove)
	rg.POST("/partners", h.RegisterPartner)
	rg.DELETE("/partners/:partnerId", h.RemovePartner)
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
		httpkit.Error(c, http.StatusInternalServerError, errListAgentMembersFailed, nil)
		return
	}

	result := make([]phoneResponse, len(rows))
	for i, row := range rows {
		partnerID := ""
		if row.PartnerID.Valid {
			partnerID = uuid.UUID(row.PartnerID.Bytes).String()
		}
		result[i] = phoneResponse{
			PhoneNumber: row.PhoneNumber,
			DisplayName: row.DisplayName,
			UserType:    row.UserType,
			PartnerID:   partnerID,
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

	phone := normalizeAgentPhoneKey(req.PhoneNumber)
	if phone == "" {
		httpkit.Error(c, http.StatusBadRequest, "phoneNumber is required", nil)
		return
	}

	err := h.queries.CreateAgentUser(c.Request.Context(), waagentdb.CreateAgentUserParams{
		PhoneNumber:    phone,
		OrganizationID: pgtype.UUID{Bytes: *tenantID, Valid: true},
		DisplayName:    strings.TrimSpace(req.DisplayName),
		UserType:       "admin",
		PartnerID:      pgtype.UUID{},
	})
	if err != nil {
		httpkit.Error(c, http.StatusConflict, "phone number already registered", nil)
		return
	}

	httpkit.JSON(c, http.StatusCreated, gin.H{"phoneNumber": phone, "status": "registered"})
}

// RegisterPartner adds a partner contact phone to the agent for this organization.
func (h *PhoneHandler) RegisterPartner(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}
	if h.partnerPhoneReader == nil {
		httpkit.Error(c, http.StatusServiceUnavailable, "partner phone registration is not configured", nil)
		return
	}

	var req registerPartnerPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid request", nil)
		return
	}

	partnerID, err := uuid.Parse(strings.TrimSpace(req.PartnerID))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid partnerId", nil)
		return
	}

	partner, err := h.partnerPhoneReader.GetPartnerPhone(c.Request.Context(), *tenantID, partnerID)
	if err != nil {
		httpkit.Error(c, http.StatusNotFound, "partner not found or has no usable phone number", nil)
		return
	}

	rows, err := h.queries.ListAgentUsersByOrganization(c.Request.Context(), pgtype.UUID{Bytes: *tenantID, Valid: true})
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, errListAgentMembersFailed, nil)
		return
	}
	for _, row := range rows {
		if row.PartnerID.Valid && uuid.UUID(row.PartnerID.Bytes) == partnerID {
			httpkit.Error(c, http.StatusConflict, "partner already registered", nil)
			return
		}
	}

	err = h.queries.CreateAgentUser(c.Request.Context(), waagentdb.CreateAgentUserParams{
		PhoneNumber:    normalizeAgentPhoneKey(partner.PhoneNumber),
		OrganizationID: pgtype.UUID{Bytes: *tenantID, Valid: true},
		DisplayName:    strings.TrimSpace(partner.DisplayName),
		UserType:       "partner",
		PartnerID:      pgtype.UUID{Bytes: partnerID, Valid: true},
	})
	if err != nil {
		httpkit.Error(c, http.StatusConflict, "partner phone already registered", nil)
		return
	}

	httpkit.JSON(c, http.StatusCreated, gin.H{
		"partnerId":   partnerID.String(),
		"phoneNumber": normalizeAgentPhoneKey(partner.PhoneNumber),
		"status":      "registered",
	})
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

// RemovePartner deletes a partner registration for this organization.
func (h *PhoneHandler) RemovePartner(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}
	partnerID, err := uuid.Parse(strings.TrimSpace(c.Param("partnerId")))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid partnerId", nil)
		return
	}
	rows, err := h.queries.ListAgentUsersByOrganization(c.Request.Context(), pgtype.UUID{Bytes: *tenantID, Valid: true})
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, errListAgentMembersFailed, nil)
		return
	}
	for _, row := range rows {
		if !row.PartnerID.Valid || uuid.UUID(row.PartnerID.Bytes) != partnerID {
			continue
		}
		if err := h.queries.DeleteAgentUser(c.Request.Context(), waagentdb.DeleteAgentUserParams{
			PhoneNumber:    row.PhoneNumber,
			OrganizationID: pgtype.UUID{Bytes: *tenantID, Valid: true},
		}); err != nil {
			httpkit.Error(c, http.StatusInternalServerError, "failed to remove partner agent member", nil)
			return
		}
		c.Status(http.StatusNoContent)
		return
	}
	httpkit.Error(c, http.StatusNotFound, "partner registration not found", nil)
}
