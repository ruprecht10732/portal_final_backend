package handler

import (
	"errors"
	"net/http"

	"portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/scheduling"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for leads.
// Uses focused services following vertical slicing pattern.
type Handler struct {
	mgmt         *management.Service
	scheduling   *scheduling.Service
	notesHandler *NotesHandler
}

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

// New creates a new leads handler with focused services.
func New(mgmt *management.Service, scheduling *scheduling.Service, notesHandler *NotesHandler) *Handler {
	return &Handler{mgmt: mgmt, scheduling: scheduling, notesHandler: notesHandler}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.GET("/check-duplicate", h.CheckDuplicate)
	rg.GET("/:id", h.GetByID)
	rg.PUT("/:id", h.Update)
	rg.DELETE("/:id", h.Delete)
	rg.POST("/bulk-delete", h.BulkDelete)
	rg.PATCH("/:id/status", h.UpdateStatus)
	rg.PUT(":id/assign", h.Assign)
	rg.POST("/:id/schedule", h.ScheduleVisit)
	rg.POST("/:id/reschedule", h.RescheduleVisit)
	rg.POST("/:id/survey", h.CompleteSurvey)
	rg.POST("/:id/no-show", h.MarkNoShow)
	rg.POST("/:id/view", h.MarkViewed)
	rg.GET("/:id/notes", h.notesHandler.ListNotes)
	rg.POST("/:id/notes", h.notesHandler.AddNote)
	rg.GET("/:id/visit-history", h.ListVisitHistory)
	// Service-specific routes
	rg.POST("/:id/services", h.AddService)
	rg.PATCH("/:id/services/:serviceId/status", h.UpdateServiceStatus)
}

func (h *Handler) Create(c *gin.Context) {
	var req transport.CreateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.mgmt.Create(c.Request.Context(), req)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.JSON(c, http.StatusCreated, lead)
}

func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	lead, err := h.mgmt.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, management.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, lead)
}

func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	actorIDValue, ok := c.Get(httpkit.ContextUserIDKey)
	if !ok {
		httpkit.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	rolesValue, ok := c.Get(httpkit.ContextRolesKey)
	if !ok {
		httpkit.Error(c, http.StatusForbidden, "forbidden", nil)
		return
	}

	actorID := actorIDValue.(uuid.UUID)
	roles := rolesValue.([]string)

	lead, err := h.mgmt.Update(c.Request.Context(), id, req, actorID, roles)
	if err != nil {
		switch {
		case errors.Is(err, management.ErrLeadNotFound):
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		case errors.Is(err, management.ErrForbidden):
			httpkit.Error(c, http.StatusForbidden, err.Error(), nil)
			return
		default:
			httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
	}

	httpkit.OK(c, lead)
}

func (h *Handler) Assign(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.AssignLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	actorIDValue, ok := c.Get(httpkit.ContextUserIDKey)
	if !ok {
		httpkit.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}
	rolesValue, ok := c.Get(httpkit.ContextRolesKey)
	if !ok {
		httpkit.Error(c, http.StatusForbidden, "forbidden", nil)
		return
	}

	actorID := actorIDValue.(uuid.UUID)
	roles := rolesValue.([]string)

	lead, err := h.mgmt.Assign(c.Request.Context(), id, req.AssigneeID, actorID, roles)
	if err != nil {
		switch {
		case errors.Is(err, management.ErrLeadNotFound):
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		case errors.Is(err, management.ErrForbidden):
			httpkit.Error(c, http.StatusForbidden, err.Error(), nil)
			return
		default:
			httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
			return
		}
	}

	httpkit.OK(c, lead)
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if err := h.mgmt.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, management.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, gin.H{"message": "lead deleted"})
}

func (h *Handler) BulkDelete(c *gin.Context) {
	var req transport.BulkDeleteLeadsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	deletedCount, err := h.mgmt.BulkDelete(c.Request.Context(), req.IDs)
	if err != nil {
		if errors.Is(err, management.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, transport.BulkDeleteLeadsResponse{DeletedCount: deletedCount})
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateLeadStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.mgmt.UpdateStatus(c.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, management.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, lead)
}

func (h *Handler) ScheduleVisit(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.ScheduleVisitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.scheduling.ScheduleVisit(c.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, scheduling.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, lead)
}

func (h *Handler) RescheduleVisit(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.RescheduleVisitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	actorID, ok := c.Get(httpkit.ContextUserIDKey)
	if !ok {
		httpkit.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	lead, err := h.scheduling.RescheduleVisit(c.Request.Context(), id, req, actorID.(uuid.UUID))
	if err != nil {
		if errors.Is(err, scheduling.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, lead)
}

func (h *Handler) CompleteSurvey(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.CompleteSurveyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.scheduling.CompleteSurvey(c.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, scheduling.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, lead)
}

func (h *Handler) MarkNoShow(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.MarkNoShowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	lead, err := h.scheduling.MarkNoShow(c.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, scheduling.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, lead)
}

func (h *Handler) MarkViewed(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	userID, exists := c.Get(httpkit.ContextUserIDKey)
	if !exists {
		httpkit.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	if err := h.mgmt.SetViewedBy(c.Request.Context(), id, userID.(uuid.UUID)); err != nil {
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, gin.H{"message": "lead marked as viewed"})
}

func (h *Handler) CheckDuplicate(c *gin.Context) {
	phone := c.Query("phone")
	if phone == "" {
		httpkit.Error(c, http.StatusBadRequest, "phone parameter required", nil)
		return
	}

	result, err := h.mgmt.CheckDuplicate(c.Request.Context(), phone)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	var req transport.ListLeadsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	result, err := h.mgmt.List(c.Request.Context(), req)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) ListVisitHistory(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	result, err := h.scheduling.ListVisitHistory(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, scheduling.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) AddService(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.AddServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.mgmt.AddService(c.Request.Context(), id, req)
	if err != nil {
		if errors.Is(err, management.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.JSON(c, http.StatusCreated, lead)
}

func (h *Handler) UpdateServiceStatus(c *gin.Context) {
	leadID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateServiceStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := validator.Validate.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.mgmt.UpdateServiceStatus(c.Request.Context(), leadID, serviceID, req)
	if err != nil {
		if errors.Is(err, management.ErrLeadNotFound) {
			httpkit.Error(c, http.StatusNotFound, err.Error(), nil)
			return
		}
		httpkit.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	httpkit.OK(c, lead)
}
