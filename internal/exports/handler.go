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
	"portal_final_backend/internal/identity/smtpcrypto"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	defaultCurrency            = "EUR"
	defaultTimezone            = "UTC"
	dateLayout                 = "2006-01-02"
	noOrgContextMsg            = "no organization context"
	credentialNotConfiguredMsg = "google ads export credentials not configured"
)

// ─── DATA STRUCTURES ─────────────────────────────────────────────────────────

// conversionRow represents a single line in the Google Ads CSV.
// Reordered fields to optimize memory alignment (8-byte boundaries).
type conversionRow struct {
	ConversionTime     time.Time
	LeadID             uuid.UUID
	LeadServiceID      uuid.UUID
	ConversionName     string
	ConversionCurrency string
	GCLID              string
	OrderID            string
	HashedEmail        string
	HashedPhone        string
	HashedFirstName    string
	HashedLastName     string
	HashedStreet       string
	City               string
	ZipCode            string
	CountryCode        string
	ConversionValue    float64
}

type ExportCredentialResponse struct {
	Username   string     `json:"username"`
	CreatedAt  string     `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

// ─── HANDLER DEFINITION ──────────────────────────────────────────────────────

type Handler struct {
	val           *validator.Validator
	repo          *Repository
	encryptionKey []byte
}

func NewHandler(repo *Repository, val *validator.Validator) *Handler {
	return &Handler{repo: repo, val: val}
}

func (h *Handler) SetEncryptionKey(key []byte) { h.encryptionKey = key }

func (h *Handler) Wait() {
	// No background tasks to wait for.
}

// ─── CREDENTIAL METHODS ──────────────────────────────────────────────────────

func (h *Handler) HandleUpsertCredential(c *gin.Context) {
	idnt := httpkit.MustGetIdentity(c)
	tid := idnt.TenantID()
	if tid == nil {
		httpkit.Error(c, http.StatusForbidden, noOrgContextMsg, nil)
		return
	}

	user, plain, err := GenerateCredential()
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "generation failed", nil)
		return
	}

	hash, _ := password.Hash(plain)
	var enc *string
	if len(h.encryptionKey) == 32 {
		if cipher, err := smtpcrypto.Encrypt(plain, h.encryptionKey); err == nil {
			enc = &cipher
		}
	}

	uid := idnt.UserID()
	cred, err := h.repo.UpsertCredential(c.Request.Context(), *tid, user, hash, enc, &uid)
	if httpkit.HandleError(c, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"username":  cred.Username,
		"password":  plain,
		"createdAt": cred.CreatedAt.Format(time.RFC3339),
	})
}

func (h *Handler) HandleGetCredential(c *gin.Context) {
	tid := httpkit.MustGetIdentity(c).TenantID()
	if tid == nil {
		httpkit.Error(c, http.StatusForbidden, noOrgContextMsg, nil)
		return
	}

	cred, err := h.repo.GetCredentialByOrganization(c.Request.Context(), *tid)
	if err == ErrCredentialNotFound {
		httpkit.Error(c, http.StatusNotFound, credentialNotConfiguredMsg, nil)
		return
	}
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, toExportCredentialResponse(cred))
}

func (h *Handler) HandleDeleteCredential(c *gin.Context) {
	tid := httpkit.MustGetIdentity(c).TenantID()
	if tid == nil {
		httpkit.Error(c, http.StatusForbidden, noOrgContextMsg, nil)
		return
	}

	err := h.repo.DeleteCredential(c.Request.Context(), *tid)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "credentials removed"})
}

func (h *Handler) HandleRevealPassword(c *gin.Context) {
	tid := httpkit.MustGetIdentity(c).TenantID()
	if tid == nil {
		httpkit.Error(c, http.StatusForbidden, noOrgContextMsg, nil)
		return
	}

	if len(h.encryptionKey) != 32 {
		httpkit.Error(c, http.StatusConflict, "reveal not configured", nil)
		return
	}

	cred, err := h.repo.GetCredentialByOrganization(c.Request.Context(), *tid)
	if httpkit.HandleError(c, err) || cred.PasswordEncrypted == nil {
		return
	}

	plain, err := smtpcrypto.Decrypt(*cred.PasswordEncrypted, h.encryptionKey)
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "decryption failed", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"password": plain})
}

// ─── EXPORT METHODS ──────────────────────────────────────────────────────────

func (h *Handler) ExportGoogleAdsCSV(c *gin.Context) {
	orgID, ok := getExportOrgID(c)
	if !ok {
		return
	}

	if credID, ok := getExportCredentialID(c); ok {
		h.repo.TouchCredential(c.Request.Context(), credID)
	}

	from, to, err := parseDateRange(c)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid range", nil)
		return
	}

	limit := parseLimit(c, 5000, 50000)
	currency := strings.ToUpper(strings.TrimSpace(c.DefaultQuery("currency", defaultCurrency)))
	enhanced := parseEnhancedMode(c.Query("enhanced"))
	schema := parseSchemaRowMode(c.Query("schemaRow"))
	loc, tzName, ok := parseTimezone(c)
	if !ok {
		return
	}

	events, err := h.repo.ListConversionEvents(c.Request.Context(), orgID, from, to, limit)
	if httpkit.HandleError(c, err) {
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=google-conversions-%s.csv", tzName))

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	_ = writer.Write(csvHeaders(enhanced))

	if len(events) == 0 {
		if schema {
			_ = writer.Write(sampleSchemaRow(loc, currency, enhanced))
		}
		return
	}

	rows := buildConversionRows(events, loc, currency, enhanced)
	for _, row := range rows {
		_ = writer.Write(row.CSV(enhanced))
	}
}

// ─── HELPERS ─────────────────────────────────────────────────────────────────

func (r conversionRow) CSV(enhanced bool) []string {
	res := []string{
		r.GCLID,
		r.ConversionName,
		r.ConversionTime.Format("2006-01-02 15:04:05-0700"),
		strconv.FormatFloat(r.ConversionValue, 'f', 2, 64),
		r.ConversionCurrency,
		r.OrderID,
	}
	if enhanced {
		res = append(res, r.HashedEmail, r.HashedPhone, r.HashedFirstName, r.HashedLastName, r.HashedStreet, r.City, r.ZipCode, r.CountryCode)
	}
	return res
}

func buildConversionRows(events []ConversionEvent, loc *time.Location, cur string, enh bool) []conversionRow {
	rows := make([]conversionRow, 0, len(events))
	for _, e := range events {
		name := "Lead_Generated"
		if e.Status != nil && *e.Status == "won" {
			name = "Deal_Won"
		}

		row := conversionRow{
			ConversionTime:     e.OccurredAt.In(loc),
			LeadID:             e.LeadID,
			LeadServiceID:      e.LeadServiceID,
			ConversionName:     name,
			ConversionCurrency: cur,
			GCLID:              e.GCLID,
			OrderID:            e.EventID.String(),
			ConversionValue:    float64(e.ProjectedValueCents) / 100,
			CountryCode:        "NL",
		}
		if enh {
			if e.ConsumerEmail != nil {
				row.HashedEmail = hashEmail(*e.ConsumerEmail)
			}
			row.HashedPhone = hashPhone(e.ConsumerPhone)
			row.HashedFirstName = hashName(e.ConsumerFirstName)
			row.HashedLastName = hashName(e.ConsumerLastName)
			row.HashedStreet = hashAddress(e.AddressStreet + " " + e.AddressHouseNumber)
			row.City = e.AddressCity
			row.ZipCode = e.AddressZipCode
		}
		rows = append(rows, row)
	}
	return rows
}

func hashEmail(val string) string {
	val = strings.TrimSpace(strings.ToLower(val))
	if parts := strings.Split(val, "@"); len(parts) == 2 {
		user, domain := parts[0], parts[1]
		if domain == "gmail.com" {
			user = strings.ReplaceAll(user, ".", "")
			if idx := strings.Index(user, "+"); idx >= 0 {
				user = user[:idx]
			}
			val = user + "@" + domain
		}
	}
	return sha256Sum(val)
}

func hashPhone(val string) string {
	var sb strings.Builder
	for _, r := range val {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	norm := sb.String()
	if strings.HasPrefix(norm, "0") && !strings.HasPrefix(norm, "00") {
		norm = "31" + norm[1:]
	}
	return sha256Sum("+" + norm)
}

func hashName(v string) string { return sha256Sum(strings.TrimSpace(strings.ToLower(v))) }
func hashAddress(v string) string {
	return sha256Sum(strings.Join(strings.Fields(strings.ToLower(v)), " "))
}
func sha256Sum(v string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(v))) }

func toExportCredentialResponse(c ExportCredential) ExportCredentialResponse {
	return ExportCredentialResponse{Username: c.Username, CreatedAt: c.CreatedAt.Format(time.RFC3339), LastUsedAt: c.LastUsedAt}
}

func csvHeaders(enh bool) []string {
	h := []string{"Google Click ID", "Conversion Name", "Conversion Time", "Conversion Value", "Conversion Currency", "Order ID"}
	if enh {
		h = append(h, "Email", "Phone Number", "First Name", "Last Name", "Street Address", "City", "Zip Code", "Country Code")
	}
	return h
}

func getExportOrgID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get("exportOrgID")
	if !ok {
		return uuid.Nil, false
	}
	return v.(uuid.UUID), true
}

func getExportCredentialID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get("exportCredentialID")
	if !ok {
		return uuid.Nil, false
	}
	return v.(uuid.UUID), true
}

func parseTimezone(c *gin.Context) (*time.Location, string, bool) {
	tz := c.DefaultQuery("timezone", defaultTimezone)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, "", false
	}
	return loc, tz, true
}

func parseDateRange(c *gin.Context) (time.Time, time.Time, error) {
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -90)
	if f := c.Query("fromDate"); f != "" {
		if p, err := time.Parse(dateLayout, f); err == nil {
			from = p
		}
	}
	if t := c.Query("toDate"); t != "" {
		if p, err := time.Parse(dateLayout, t); err == nil {
			to = p.Add(24 * time.Hour)
		}
	}
	return from, to, nil
}

func parseLimit(c *gin.Context, def, max int) int {
	l, err := strconv.Atoi(c.Query("limit"))
	if err != nil || l < 1 {
		return def
	}
	if l > max {
		return max
	}
	return l
}

func parseEnhancedMode(v string) bool  { return v != "0" && v != "false" }
func parseSchemaRowMode(v string) bool { return v == "1" || v == "true" }

func sampleSchemaRow(loc *time.Location, cur string, enh bool) []string {
	r := conversionRow{
		GCLID: "TEST_GCLID", ConversionName: "Sample",
		ConversionTime: time.Now().In(loc), ConversionCurrency: cur,
		OrderID: "123", ConversionValue: 0,
	}
	return r.CSV(enh)
}
