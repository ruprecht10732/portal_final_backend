package exports

import (
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portal_final_backend/internal/auth/password"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	defaultCurrency = "EUR"
	defaultTimezone = "UTC"
	dateLayout      = "2006-01-02"
	noOrgContextMsg = "no organization context"
)

// Handler handles export requests and Google Ads HTTPS credential management.
type Handler struct {
	repo *Repository
	val  *validator.Validator
}

// NewHandler creates a new export handler.
func NewHandler(repo *Repository, val *validator.Validator) *Handler {
	return &Handler{repo: repo, val: val}
}

// ---- Admin Google Ads HTTPS Credentials (JWT authenticated) ----

type ExportCredentialResponse struct {
	Username   string     `json:"username"`
	CreatedAt  string     `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type UpsertExportCredentialResponse struct {
	ExportCredentialResponse
	Password string `json:"password"`
}

func (h *Handler) HandleUpsertCredential(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, noOrgContextMsg, nil)
		return
	}

	username, plaintextPassword, err := GenerateCredential()
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to generate export credentials", nil)
		return
	}

	passwordHash, err := password.Hash(plaintextPassword)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to secure export credentials", nil)
		return
	}

	createdBy := identity.UserID()
	credential, err := h.repo.UpsertCredential(c.Request.Context(), *tenantID, username, passwordHash, &createdBy)
	if httpkit.HandleError(c, err) {
		return
	}

	c.JSON(http.StatusOK, UpsertExportCredentialResponse{
		ExportCredentialResponse: toExportCredentialResponse(credential),
		Password:                 plaintextPassword,
	})
}

func (h *Handler) HandleGetCredential(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, noOrgContextMsg, nil)
		return
	}

	credential, err := h.repo.GetCredentialByOrganization(c.Request.Context(), *tenantID)
	if err == ErrCredentialNotFound {
		httpkit.Error(c, http.StatusNotFound, "google ads export credentials not configured", nil)
		return
	}
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, toExportCredentialResponse(credential))
}

func (h *Handler) HandleDeleteCredential(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, noOrgContextMsg, nil)
		return
	}

	err := h.repo.DeleteCredential(c.Request.Context(), *tenantID)
	if err == ErrCredentialNotFound {
		httpkit.Error(c, http.StatusNotFound, "google ads export credentials not configured", nil)
		return
	}
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "google ads export credentials removed"})
}

// ---- Google Ads CSV Export (HTTP Basic authenticated) ----

func (h *Handler) ExportGoogleAdsCSV(c *gin.Context) {
	orgID, ok := getExportOrgID(c)
	if !ok {
		return
	}

	credentialID, ok := getExportCredentialID(c)
	if ok {
		h.repo.TouchCredential(c.Request.Context(), credentialID)
	}

	fromDate, toDate, err := parseDateRange(c)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid date range", err.Error())
		return
	}

	limit := parseLimit(c, 5000, 50000)
	currency := strings.ToUpper(strings.TrimSpace(c.DefaultQuery("currency", defaultCurrency)))
	useEnhanced := parseEnhancedMode(c.Query("enhanced"))

	location, tzName, ok := parseTimezone(c)
	if !ok {
		return
	}

	events, err := h.repo.ListConversionEvents(c.Request.Context(), orgID, fromDate, toDate, limit)
	if httpkit.HandleError(c, err) {
		return
	}

	rows := buildConversionRows(events, location, currency, useEnhanced)
	if len(rows) == 0 {
		writeEmptyCsv(c, tzName, useEnhanced)
		return
	}

	orderIDs := collectOrderIDs(rows)
	exportedKeys, err := h.repo.ListExportedKeys(c.Request.Context(), orgID, orderIDs)
	if httpkit.HandleError(c, err) {
		return
	}

	writer, ok := startCsvResponse(c, tzName, useEnhanced)
	if !ok {
		return
	}

	records, ok := writeConversionRows(writer, rows, exportedKeys, useEnhanced)
	if !ok {
		return
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return
	}

	_ = h.repo.RecordExports(c.Request.Context(), orgID, records)
}

// ---- Helpers ----

type conversionRow struct {
	LeadID             uuid.UUID
	LeadServiceID      uuid.UUID
	ConversionName     string
	ConversionTime     time.Time
	ConversionValue    float64
	ConversionCurrency string
	GCLID              string
	OrderID            string
	HashedEmail        string
	HashedPhone        string
}

func (r conversionRow) CSV(useEnhanced bool) []string {
	fields := []string{
		r.GCLID,
		r.ConversionName,
		formatConversionTime(r.ConversionTime),
		formatConversionValue(r.ConversionValue),
		r.ConversionCurrency,
		r.OrderID,
	}
	if useEnhanced {
		fields = append(fields, r.HashedEmail, r.HashedPhone)
	}
	return fields
}

func csvHeaders(useEnhanced bool) []string {
	headers := []string{
		"Google Click ID",
		"Conversion Name",
		"Conversion Time",
		"Conversion Value",
		"Conversion Currency",
		"Order ID",
	}
	if useEnhanced {
		headers = append(headers, "Email", "Phone Number")
	}
	return headers
}

func getExportOrgID(c *gin.Context) (uuid.UUID, bool) {
	orgIDVal, ok := c.Get("exportOrgID")
	if !ok {
		httpkit.Error(c, http.StatusUnauthorized, "missing organization context", nil)
		return uuid.UUID{}, false
	}
	orgID, ok := orgIDVal.(uuid.UUID)
	if !ok {
		httpkit.Error(c, http.StatusUnauthorized, "missing organization context", nil)
		return uuid.UUID{}, false
	}
	return orgID, true
}

func getExportCredentialID(c *gin.Context) (uuid.UUID, bool) {
	credentialIDVal, _ := c.Get("exportCredentialID")
	credentialID, ok := credentialIDVal.(uuid.UUID)
	return credentialID, ok
}

func parseTimezone(c *gin.Context) (*time.Location, string, bool) {
	tzName := strings.TrimSpace(c.DefaultQuery("timezone", defaultTimezone))
	location, err := time.LoadLocation(tzName)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid timezone", nil)
		return nil, "", false
	}
	return location, tzName, true
}

func writeEmptyCsv(c *gin.Context, tzName string, useEnhanced bool) {
	writer, ok := startCsvResponse(c, tzName, useEnhanced)
	if !ok {
		return
	}

	writer.Flush()
	_ = writer.Error()
}

func collectOrderIDs(rows []conversionRow) []string {
	orderIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		orderIDs = append(orderIDs, row.OrderID)
	}
	return orderIDs
}

func startCsvResponse(c *gin.Context, _ string, useEnhanced bool) (*csv.Writer, bool) {
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=google-ads-conversions.csv")

	writer := csv.NewWriter(c.Writer)
	if err := writer.Write(csvHeaders(useEnhanced)); err != nil {
		return nil, false
	}
	return writer, true
}

func writeConversionRows(writer *csv.Writer, rows []conversionRow, exportedKeys map[string]struct{}, useEnhanced bool) ([]ExportRecord, bool) {
	records := make([]ExportRecord, 0, len(rows))
	for _, row := range rows {
		if _, exists := exportedKeys[row.OrderID+"::"+row.ConversionName]; exists {
			continue
		}
		if err := writer.Write(row.CSV(useEnhanced)); err != nil {
			return nil, false
		}
		records = append(records, ExportRecord{
			LeadID:          row.LeadID,
			LeadServiceID:   row.LeadServiceID,
			ConversionName:  row.ConversionName,
			ConversionTime:  row.ConversionTime,
			ConversionValue: row.ConversionValue,
			GCLID:           row.GCLID,
			OrderID:         row.OrderID,
		})
	}
	return records, true
}

func toExportCredentialResponse(credential ExportCredential) ExportCredentialResponse {
	return ExportCredentialResponse{
		Username:   credential.Username,
		CreatedAt:  credential.CreatedAt.Format(time.RFC3339),
		LastUsedAt: credential.LastUsedAt,
	}
}

func parseDateRange(c *gin.Context) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	defaultFrom := now.AddDate(0, 0, -90)
	fromStr := strings.TrimSpace(c.DefaultQuery("fromDate", ""))
	toStr := strings.TrimSpace(c.DefaultQuery("toDate", ""))

	from := defaultFrom
	to := now

	if fromStr != "" {
		parsed, err := time.Parse(dateLayout, fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		from = parsed
	}
	if toStr != "" {
		parsed, err := time.Parse(dateLayout, toStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to = parsed.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("toDate before fromDate")
	}
	return from, to, nil
}

func parseLimit(c *gin.Context, fallback int, max int) int {
	limit := fallback
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit > max {
		return max
	}
	if limit < 1 {
		return fallback
	}
	return limit
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

func parseEnhancedMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1", "true", "yes", "y":
		return true
	case "0", "false", "no", "n":
		return false
	default:
		return true
	}
}

func buildConversionRows(events []ConversionEvent, location *time.Location, currency string, includeEnhanced bool) []conversionRow {
	rows := make([]conversionRow, 0, len(events))
	for _, event := range events {
		conversionName := mapConversionName(event)
		if conversionName == "" {
			continue
		}
		conversionTime := event.OccurredAt.In(location)
		conversionValue := mapConversionValue(conversionName, event.ProjectedValueCents)

		hashedEmail := ""
		hashedPhone := ""
		if includeEnhanced {
			if event.ConsumerEmail != nil {
				hashedEmail = hashEmail(*event.ConsumerEmail)
			}
			hashedPhone = hashPhone(event.ConsumerPhone)
		}

		hasEnhancedIdentifier := hashedEmail != "" || hashedPhone != ""
		if event.GCLID == "" {
			if !includeEnhanced || !hasEnhancedIdentifier {
				continue
			}
		}

		rows = append(rows, conversionRow{
			LeadID:             event.LeadID,
			LeadServiceID:      event.LeadServiceID,
			ConversionName:     conversionName,
			ConversionTime:     conversionTime,
			ConversionValue:    conversionValue,
			ConversionCurrency: currency,
			GCLID:              event.GCLID,
			OrderID:            event.EventID.String(),
			HashedEmail:        hashedEmail,
			HashedPhone:        hashedPhone,
		})
	}
	return rows
}

func mapConversionName(event ConversionEvent) string {
	if event.EventType == "status_changed" && event.Status != nil {
		switch *event.Status {
		case "Scheduled":
			return "Appointment_Scheduled"
		case "Surveyed":
			return "Visit_Completed"
		case "Closed":
			return "Deal_Won"
		}
	}

	if event.EventType == "pipeline_stage_changed" && event.PipelineStage != nil {
		switch *event.PipelineStage {
		case "Nurturing":
			return "Lead_Qualified"
		case "Quote_Sent":
			return "Quote_Sent"
		case "Partner_Assigned":
			return "Partner_Assigned"
		case "Completed":
			return "Job_Completed"
		}
	}

	return ""
}

func mapConversionValue(conversionName string, projectedValueCents int64) float64 {
	if projectedValueCents <= 0 {
		return 0
	}

	switch conversionName {
	case "Deal_Won", "Job_Completed":
		return float64(projectedValueCents) / 100
	default:
		return 0
	}
}

func formatConversionTime(value time.Time) string {
	return value.Format("2006-01-02 15:04:05-0700")
}

func formatConversionValue(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func hashEmail(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	parts := strings.Split(value, "@")
	if len(parts) == 2 {
		domain := parts[1]
		user := parts[0]
		if domain == "gmail.com" || domain == "googlemail.com" {
			user = strings.ReplaceAll(user, ".", "")
			if plusIndex := strings.Index(user, "+"); plusIndex >= 0 {
				user = user[:plusIndex]
			}
			value = user + "@" + domain
		}
	}

	hash := sha256Sum(value)
	return hash
}

func hashPhone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	cleaned := strings.Builder{}
	for _, r := range value {
		if r >= '0' && r <= '9' {
			cleaned.WriteRune(r)
		}
	}

	normalized := cleaned.String()
	if normalized == "" {
		return ""
	}

	return sha256Sum("+" + normalized)
}

func sha256Sum(value string) string {
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", hash)
}
