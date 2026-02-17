package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"portal_final_backend/internal/identity/smtpcrypto"
	"portal_final_backend/internal/quotes/repository"
	"portal_final_backend/internal/quotes/transport"
	"portal_final_backend/platform/apperr"

	"github.com/google/uuid"
)

const (
	moneybirdAPIBaseURL         = "https://moneybird.com/api/v2"
	defaultFrontendBaseURL      = "http://localhost:4200"
	moneybirdOAuthDefaultScope  = "sales_invoices"
	httpHeaderContentType       = "Content-Type"
	httpHeaderAccept            = "Accept"
	httpHeaderAuthorization     = "Authorization"
	mimeApplicationJSON         = "application/json"
	mimeFormURLEncoded          = "application/x-www-form-urlencoded"
	authorizationBearerPrefix   = "Bearer "
	moneybirdTokenRefreshLeeway = 30 * time.Second
)

type moneybirdOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type moneybirdAdministration struct {
	ID string `json:"id"`
}

type moneybirdContact struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type moneybirdTaxRate struct {
	ID         string `json:"id"`
	Percentage string `json:"percentage"`
}

type moneybirdExportLine struct {
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Amount      string  `json:"amount"`
	TaxRateID   int64   `json:"tax_rate_id"`
}

type moneybirdCreateInvoiceBody struct {
	SalesInvoice struct {
		ContactID         string                `json:"contact_id"`
		Reference         string                `json:"reference"`
		WorkflowID        *string               `json:"workflow_id,omitempty"`
		DetailsAttributes []moneybirdExportLine `json:"details_attributes"`
	} `json:"sales_invoice"`
}

type moneybirdInvoiceResponse struct {
	ID string `json:"id"`
}

type oauthStatePayload struct {
	OrganizationID string `json:"organizationId"`
	IssuedAt       int64  `json:"issuedAt"`
	Provider       string `json:"provider"`
}

type moneybirdConfig struct {
	ClientID      string
	ClientSecret  string
	RedirectURI   string
	FrontendURL   string
	EncryptionKey []byte
}

func (s *Service) SetMoneybirdConfig(clientID string, clientSecret string, redirectURI string, frontendURL string) {
	if s.moneybird == nil {
		s.moneybird = &moneybirdConfig{}
	}
	s.moneybird.ClientID = strings.TrimSpace(clientID)
	s.moneybird.ClientSecret = strings.TrimSpace(clientSecret)
	s.moneybird.RedirectURI = strings.TrimSpace(redirectURI)
	s.moneybird.FrontendURL = strings.TrimSpace(frontendURL)
}

func (s *Service) SetMoneybirdEncryptionKey(key []byte) {
	if s.moneybird == nil {
		s.moneybird = &moneybirdConfig{}
	}
	s.moneybird.EncryptionKey = key
}

func (s *Service) MoneybirdIntegrationRedirectURL(status string) string {
	baseURL := defaultFrontendBaseURL
	if s.moneybird != nil && strings.TrimSpace(s.moneybird.FrontendURL) != "" {
		baseURL = strings.TrimSpace(s.moneybird.FrontendURL)
	}

	return fmt.Sprintf(
		"%s/app/organization/integrations/moneybird?moneybird=%s",
		strings.TrimRight(baseURL, "/"),
		url.QueryEscape(status),
	)
}

func (s *Service) buildOAuthState(tenantID uuid.UUID) (string, error) {
	if s.moneybird == nil || len(s.moneybird.EncryptionKey) == 0 {
		return "", apperr.BadRequest("moneybird integration is not configured")
	}
	payload := oauthStatePayload{
		OrganizationID: tenantID.String(),
		IssuedAt:       time.Now().Unix(),
		Provider:       "moneybird",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal oauth state: %w", err)
	}

	mac := hmac.New(sha256.New, s.moneybird.EncryptionKey)
	_, _ = mac.Write(raw)
	sig := mac.Sum(nil)
	combined := append(raw, sig...)
	return base64.RawURLEncoding.EncodeToString(combined), nil
}

func (s *Service) parseOAuthState(state string) (uuid.UUID, error) {
	if s.moneybird == nil || len(s.moneybird.EncryptionKey) == 0 {
		return uuid.Nil, apperr.BadRequest("moneybird integration is not configured")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		return uuid.Nil, apperr.BadRequest("invalid oauth state")
	}
	if len(decoded) < sha256.Size {
		return uuid.Nil, apperr.BadRequest("invalid oauth state")
	}
	raw := decoded[:len(decoded)-sha256.Size]
	receivedSig := decoded[len(decoded)-sha256.Size:]

	mac := hmac.New(sha256.New, s.moneybird.EncryptionKey)
	_, _ = mac.Write(raw)
	expectedSig := mac.Sum(nil)
	if !hmac.Equal(receivedSig, expectedSig) {
		return uuid.Nil, apperr.BadRequest("invalid oauth state signature")
	}

	var payload oauthStatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return uuid.Nil, apperr.BadRequest("invalid oauth state payload")
	}
	if payload.Provider != "moneybird" {
		return uuid.Nil, apperr.BadRequest("invalid oauth provider")
	}
	if time.Since(time.Unix(payload.IssuedAt, 0)) > 15*time.Minute {
		return uuid.Nil, apperr.BadRequest("oauth state expired")
	}
	tenantID, err := uuid.Parse(payload.OrganizationID)
	if err != nil {
		return uuid.Nil, apperr.BadRequest("invalid organization in oauth state")
	}
	return tenantID, nil
}

func (s *Service) GetMoneybirdAuthorizeURL(ctx context.Context, tenantID uuid.UUID) (*transport.MoneybirdAuthorizeURLResponse, error) {
	_, _ = ctx, tenantID
	if s.moneybird == nil || s.moneybird.ClientID == "" || s.moneybird.ClientSecret == "" || s.moneybird.RedirectURI == "" {
		return nil, apperr.BadRequest("moneybird oauth is not configured")
	}
	state, err := s.buildOAuthState(tenantID)
	if err != nil {
		return nil, err
	}
	authorizeURL := fmt.Sprintf(
		"https://moneybird.com/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
		url.QueryEscape(s.moneybird.ClientID),
		url.QueryEscape(s.moneybird.RedirectURI),
		url.QueryEscape(moneybirdOAuthDefaultScope),
		url.QueryEscape(state),
	)
	return &transport.MoneybirdAuthorizeURLResponse{
		Provider:     "moneybird",
		AuthorizeURL: authorizeURL,
	}, nil
}

func (s *Service) HandleMoneybirdOAuthCallback(ctx context.Context, code string, state string) (*transport.MoneybirdCallbackResponse, string, error) {
	if s.moneybird == nil || s.moneybird.ClientID == "" || s.moneybird.ClientSecret == "" || s.moneybird.RedirectURI == "" {
		return nil, "", apperr.BadRequest("moneybird oauth is not configured")
	}
	tenantID, err := s.parseOAuthState(state)
	if err != nil {
		return nil, "", err
	}

	tokens, err := s.moneybirdExchangeCode(ctx, code)
	if err != nil {
		return nil, "", err
	}

	administrationID, err := s.moneybirdResolveAdministrationID(ctx, tokens.AccessToken)
	if err != nil {
		return nil, "", err
	}

	encryptedAccess, err := smtpcrypto.Encrypt(tokens.AccessToken, s.moneybird.EncryptionKey)
	if err != nil {
		return nil, "", fmt.Errorf("encrypt access token: %w", err)
	}
	encryptedRefresh, err := smtpcrypto.Encrypt(tokens.RefreshToken, s.moneybird.EncryptionKey)
	if err != nil {
		return nil, "", fmt.Errorf("encrypt refresh token: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	if err := s.repo.UpsertProviderIntegration(ctx, repository.ProviderIntegration{
		OrganizationID:   tenantID,
		Provider:         "moneybird",
		IsConnected:      true,
		AccessToken:      &encryptedAccess,
		RefreshToken:     &encryptedRefresh,
		TokenExpiresAt:   &expiresAt,
		AdministrationID: &administrationID,
		DisconnectedAt:   nil,
	}); err != nil {
		return nil, "", err
	}

	resp := &transport.MoneybirdCallbackResponse{
		Provider:         "moneybird",
		IsConnected:      true,
		AdministrationID: administrationID,
	}
	return resp, tenantID.String(), nil
}

func (s *Service) DisconnectProvider(ctx context.Context, tenantID uuid.UUID, provider string) error {
	normalizedProvider, err := normalizeProvider(provider)
	if err != nil {
		return err
	}
	return s.repo.DisconnectProviderIntegration(ctx, tenantID, normalizedProvider)
}

func (s *Service) moneybirdExchangeCode(ctx context.Context, code string) (*moneybirdOAuthTokenResponse, error) {
	endpoint := "https://moneybird.com/oauth/token"
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.moneybird.RedirectURI)
	form.Set("client_id", s.moneybird.ClientID)
	form.Set("client_secret", s.moneybird.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build oauth token request: %w", err)
	}
	req.Header.Set(httpHeaderContentType, mimeFormURLEncoded)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apperr.BadRequest("moneybird token exchange failed")
	}

	var tokens moneybirdOAuthTokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, fmt.Errorf("decode oauth token response: %w", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return nil, apperr.BadRequest("moneybird returned incomplete token response")
	}
	return &tokens, nil
}

func (s *Service) moneybirdRefreshToken(ctx context.Context, refreshToken string) (*moneybirdOAuthTokenResponse, error) {
	endpoint := "https://moneybird.com/oauth/token"
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", s.moneybird.ClientID)
	form.Set("client_secret", s.moneybird.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build refresh token request: %w", err)
	}
	req.Header.Set(httpHeaderContentType, mimeFormURLEncoded)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apperr.BadRequest("moneybird refresh token failed")
	}

	var tokens moneybirdOAuthTokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, fmt.Errorf("decode refresh token response: %w", err)
	}
	if tokens.AccessToken == "" {
		return nil, apperr.BadRequest("moneybird returned empty refreshed access token")
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = refreshToken
	}
	return &tokens, nil
}

func (s *Service) moneybirdResolveAdministrationID(ctx context.Context, accessToken string) (string, error) {
	endpoint := moneybirdAPIBaseURL + "/administrations"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build administrations request: %w", err)
	}
	req.Header.Set(httpHeaderAuthorization, authorizationBearerPrefix+accessToken)
	req.Header.Set(httpHeaderAccept, mimeApplicationJSON)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("administrations request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", apperr.BadRequest("moneybird administrations lookup failed")
	}

	var administrations []moneybirdAdministration
	if err := json.Unmarshal(body, &administrations); err != nil {
		return "", fmt.Errorf("decode administrations response: %w", err)
	}
	if len(administrations) == 0 || administrations[0].ID == "" {
		return "", apperr.BadRequest("no moneybird administration available")
	}
	return administrations[0].ID, nil
}

func normalizeProvider(provider string) (string, error) {
	switch provider {
	case "moneybird", "rompslomp", "stripe":
		return provider, nil
	default:
		return "", apperr.BadRequest("unsupported provider")
	}
}

func (s *Service) GetProviderIntegrationStatus(ctx context.Context, tenantID uuid.UUID, provider string) (*transport.ProviderIntegrationStatusResponse, error) {
	normalizedProvider, err := normalizeProvider(provider)
	if err != nil {
		return nil, err
	}

	integration, err := s.repo.GetProviderIntegration(ctx, tenantID, normalizedProvider)
	if err != nil {
		return nil, err
	}

	if integration == nil {
		return &transport.ProviderIntegrationStatusResponse{
			Provider:    normalizedProvider,
			IsConnected: false,
		}, nil
	}

	return &transport.ProviderIntegrationStatusResponse{
		Provider:    normalizedProvider,
		IsConnected: integration.IsConnected,
		ConnectedAt: &integration.UpdatedAt,
	}, nil
}

func (s *Service) GetQuoteExportStatus(ctx context.Context, quoteID, tenantID uuid.UUID, provider string) (*transport.QuoteExportStatusResponse, error) {
	normalizedProvider, err := normalizeProvider(provider)
	if err != nil {
		return nil, err
	}

	if _, err := s.repo.GetByID(ctx, quoteID, tenantID); err != nil {
		return nil, err
	}

	export, err := s.repo.GetQuoteExport(ctx, quoteID, tenantID, normalizedProvider)
	if err != nil {
		return nil, err
	}

	if export == nil {
		return &transport.QuoteExportStatusResponse{
			QuoteID:    quoteID,
			Provider:   normalizedProvider,
			IsExported: false,
		}, nil
	}

	externalID := export.ExternalID
	state := export.State
	exportedAt := export.CreatedAt

	return &transport.QuoteExportStatusResponse{
		QuoteID:     quoteID,
		Provider:    normalizedProvider,
		IsExported:  true,
		ExternalID:  &externalID,
		ExternalURL: export.ExternalURL,
		State:       &state,
		ExportedAt:  &exportedAt,
	}, nil
}

func (s *Service) ExportQuoteToProvider(ctx context.Context, quoteID, tenantID uuid.UUID, provider string) (*transport.QuoteExportResponse, error) {
	normalizedProvider, err := normalizeProvider(provider)
	if err != nil {
		return nil, err
	}

	integration, err := s.repo.GetProviderIntegration(ctx, tenantID, normalizedProvider)
	if err != nil {
		return nil, err
	}
	if integration == nil || !integration.IsConnected {
		return nil, apperr.BadRequest("provider is not connected")
	}

	if normalizedProvider != "moneybird" {
		return nil, apperr.BadRequest("provider not yet implemented")
	}

	if err := s.repo.MustBeAcceptedQuote(ctx, quoteID, tenantID); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetQuoteExport(ctx, quoteID, tenantID, normalizedProvider)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &transport.QuoteExportResponse{
			QuoteID:     quoteID,
			Provider:    normalizedProvider,
			ExternalID:  existing.ExternalID,
			ExternalURL: ptrToString(existing.ExternalURL),
			State:       existing.State,
			ExportedAt:  existing.CreatedAt,
		}, nil
	}

	now := time.Now()
	state := "draft"
	moneybirdInvoiceID, invoiceURL, mbErr := s.exportQuoteToMoneybird(ctx, quoteID, tenantID, integration)
	if mbErr != nil {
		return nil, mbErr
	}
	externalID := moneybirdInvoiceID
	externalURL := optionalString(invoiceURL)

	err = s.repo.CreateQuoteExport(ctx, repository.QuoteExport{
		ID:             uuid.New(),
		QuoteID:        quoteID,
		OrganizationID: tenantID,
		Provider:       normalizedProvider,
		ExternalID:     externalID,
		ExternalURL:    externalURL,
		State:          state,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		return nil, err
	}

	return &transport.QuoteExportResponse{
		QuoteID:     quoteID,
		Provider:    normalizedProvider,
		ExternalID:  externalID,
		ExternalURL: ptrToString(externalURL),
		State:       state,
		ExportedAt:  now,
	}, nil
}

func (s *Service) BulkExportQuotesToProvider(ctx context.Context, quoteIDs []uuid.UUID, tenantID uuid.UUID, provider string) (*transport.BulkQuoteExportResponse, error) {
	normalizedProvider, err := normalizeProvider(provider)
	if err != nil {
		return nil, err
	}

	items := make([]transport.BulkQuoteExportItem, 0, len(quoteIDs))
	for _, quoteID := range quoteIDs {
		export, exportErr := s.ExportQuoteToProvider(ctx, quoteID, tenantID, normalizedProvider)
		if exportErr != nil {
			errText := exportErr.Error()
			items = append(items, transport.BulkQuoteExportItem{
				QuoteID:  quoteID,
				Provider: normalizedProvider,
				Status:   "failed",
				Error:    &errText,
			})
			continue
		}

		items = append(items, transport.BulkQuoteExportItem{
			QuoteID:     quoteID,
			Provider:    normalizedProvider,
			Status:      "exported",
			ExternalID:  &export.ExternalID,
			ExternalURL: optionalString(export.ExternalURL),
			State:       optionalString(export.State),
			ExportedAt:  &export.ExportedAt,
		})
	}

	return &transport.BulkQuoteExportResponse{Items: items}, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (s *Service) exportQuoteToMoneybird(ctx context.Context, quoteID, tenantID uuid.UUID, integration *repository.ProviderIntegration) (string, string, error) {
	if err := s.validateMoneybirdExportIntegration(integration); err != nil {
		return "", "", err
	}

	accessToken, err := s.resolveMoneybirdAccessToken(ctx, tenantID, integration)
	if err != nil {
		return "", "", err
	}

	quote, items, err := s.loadMoneybirdQuoteData(ctx, quoteID, tenantID)
	if err != nil {
		return "", "", err
	}

	administrationID := *integration.AdministrationID
	contactID, err := s.moneybirdResolveContactID(ctx, administrationID, accessToken, quote.CustomerEmail, quote.CustomerFirstName, quote.CustomerLastName)
	if err != nil {
		return "", "", err
	}

	taxRateIDByBPS, err := s.moneybirdResolveTaxRateIDByBPS(ctx, administrationID, accessToken)
	if err != nil {
		return "", "", err
	}

	lines, err := s.buildMoneybirdExportLines(items, taxRateIDByBPS)
	if err != nil {
		return "", "", err
	}

	return s.createMoneybirdInvoice(ctx, administrationID, accessToken, quote.QuoteNumber, contactID, lines)
}

func (s *Service) moneybirdResolveContactID(ctx context.Context, administrationID string, accessToken string, email *string, firstName *string, lastName *string) (string, error) {
	foundContactID, err := s.moneybirdFindContactIDByEmail(ctx, administrationID, accessToken, email)
	if err != nil {
		return "", err
	}
	if foundContactID != "" {
		return foundContactID, nil
	}

	return s.moneybirdCreateContact(ctx, administrationID, accessToken, email, firstName, lastName)
}

func (s *Service) validateMoneybirdExportIntegration(integration *repository.ProviderIntegration) error {
	if s.moneybird == nil || len(s.moneybird.EncryptionKey) == 0 {
		return apperr.BadRequest("moneybird encryption key is not configured")
	}
	if integration.AccessToken == nil || integration.RefreshToken == nil || integration.AdministrationID == nil {
		return apperr.BadRequest("moneybird credentials are incomplete")
	}
	return nil
}

func (s *Service) resolveMoneybirdAccessToken(ctx context.Context, tenantID uuid.UUID, integration *repository.ProviderIntegration) (string, error) {
	accessToken, refreshToken, err := s.decryptMoneybirdTokens(integration)
	if err != nil {
		return "", err
	}

	if integration.TokenExpiresAt == nil || integration.TokenExpiresAt.After(time.Now().Add(moneybirdTokenRefreshLeeway)) {
		return accessToken, nil
	}

	refreshed, refreshErr := s.moneybirdRefreshToken(ctx, refreshToken)
	if refreshErr != nil {
		_ = s.repo.DisconnectProviderIntegration(ctx, tenantID, "moneybird")
		return "", refreshErr
	}

	if err := s.persistRefreshedMoneybirdTokens(ctx, tenantID, integration, refreshed); err != nil {
		return "", err
	}

	return refreshed.AccessToken, nil
}

func (s *Service) decryptMoneybirdTokens(integration *repository.ProviderIntegration) (string, string, error) {
	accessToken, err := smtpcrypto.Decrypt(*integration.AccessToken, s.moneybird.EncryptionKey)
	if err != nil {
		return "", "", fmt.Errorf("decrypt moneybird access token: %w", err)
	}

	refreshToken, err := smtpcrypto.Decrypt(*integration.RefreshToken, s.moneybird.EncryptionKey)
	if err != nil {
		return "", "", fmt.Errorf("decrypt moneybird refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

func (s *Service) persistRefreshedMoneybirdTokens(
	ctx context.Context,
	tenantID uuid.UUID,
	integration *repository.ProviderIntegration,
	refreshed *moneybirdOAuthTokenResponse,
) error {
	encAccess, err := smtpcrypto.Encrypt(refreshed.AccessToken, s.moneybird.EncryptionKey)
	if err != nil {
		return fmt.Errorf("encrypt refreshed access token: %w", err)
	}

	encRefresh, err := smtpcrypto.Encrypt(refreshed.RefreshToken, s.moneybird.EncryptionKey)
	if err != nil {
		return fmt.Errorf("encrypt refreshed refresh token: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(refreshed.ExpiresIn) * time.Second)
	return s.repo.UpsertProviderIntegration(ctx, repository.ProviderIntegration{
		OrganizationID:   tenantID,
		Provider:         "moneybird",
		IsConnected:      true,
		AccessToken:      &encAccess,
		RefreshToken:     &encRefresh,
		TokenExpiresAt:   &expiresAt,
		AdministrationID: integration.AdministrationID,
		ConnectedBy:      integration.ConnectedBy,
		DisconnectedAt:   nil,
	})
}

func (s *Service) loadMoneybirdQuoteData(ctx context.Context, quoteID, tenantID uuid.UUID) (*repository.Quote, []repository.QuoteItem, error) {
	quote, err := s.repo.GetByID(ctx, quoteID, tenantID)
	if err != nil {
		return nil, nil, err
	}

	items, err := s.repo.GetItemsByQuoteID(ctx, quoteID, tenantID)
	if err != nil {
		return nil, nil, err
	}

	return quote, items, nil
}

func (s *Service) buildMoneybirdExportLines(items []repository.QuoteItem, taxRateIDByBPS map[int]int64) ([]moneybirdExportLine, error) {
	lines := make([]moneybirdExportLine, 0, len(items))
	for _, item := range items {
		taxID, ok := taxRateIDByBPS[item.TaxRateBps]
		if !ok {
			return nil, apperr.BadRequest("moneybird export failed: tax rate not found")
		}

		lines = append(lines, moneybirdExportLine{
			Description: item.Description,
			Price:       float64(item.UnitPriceCents) / 100,
			Amount:      moneybirdExportAmount(item.Quantity),
			TaxRateID:   taxID,
		})
	}

	return lines, nil
}

func moneybirdExportAmount(quantity string) string {
	parsed := parseQuantityNumber(quantity)
	return strconv.FormatFloat(parsed, 'f', -1, 64)
}

func (s *Service) createMoneybirdInvoice(
	ctx context.Context,
	administrationID string,
	accessToken string,
	reference string,
	contactID string,
	lines []moneybirdExportLine,
) (string, string, error) {
	body := moneybirdCreateInvoiceBody{}
	body.SalesInvoice.ContactID = contactID
	body.SalesInvoice.Reference = reference
	body.SalesInvoice.DetailsAttributes = lines

	rawBody, err := json.Marshal(body)
	if err != nil {
		return "", "", fmt.Errorf("marshal moneybird invoice body: %w", err)
	}

	invoiceEndpoint := fmt.Sprintf("%s/%s/sales_invoices", moneybirdAPIBaseURL, administrationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, invoiceEndpoint, bytes.NewReader(rawBody))
	if err != nil {
		return "", "", fmt.Errorf("build moneybird invoice request: %w", err)
	}
	req.Header.Set(httpHeaderAuthorization, authorizationBearerPrefix+accessToken)
	req.Header.Set(httpHeaderAccept, mimeApplicationJSON)
	req.Header.Set(httpHeaderContentType, mimeApplicationJSON)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("moneybird invoice request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		details := strings.TrimSpace(string(respBody))
		if details == "" {
			return "", "", apperr.BadRequest("moneybird invoice creation failed")
		}
		return "", "", apperr.BadRequest(fmt.Sprintf("moneybird invoice creation failed: %s", details))
	}

	var invoice moneybirdInvoiceResponse
	if err := json.Unmarshal(respBody, &invoice); err != nil {
		return "", "", fmt.Errorf("decode moneybird invoice response: %w", err)
	}
	if invoice.ID == "" {
		return "", "", apperr.BadRequest("moneybird invoice id missing in response")
	}

	invoiceURL := fmt.Sprintf("https://moneybird.com/app/%s/sales_invoices/%s", administrationID, invoice.ID)
	return invoice.ID, invoiceURL, nil
}

func (s *Service) moneybirdFindContactIDByEmail(ctx context.Context, administrationID string, accessToken string, email *string) (string, error) {
	if email == nil || strings.TrimSpace(*email) == "" {
		return "", nil
	}
	targetEmail := strings.TrimSpace(*email)

	queryURL := fmt.Sprintf("%s/%s/contacts?query=%s", moneybirdAPIBaseURL, administrationID, url.QueryEscape(targetEmail))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return "", fmt.Errorf("build moneybird contact search request: %w", err)
	}
	req.Header.Set(httpHeaderAuthorization, authorizationBearerPrefix+accessToken)
	req.Header.Set(httpHeaderAccept, mimeApplicationJSON)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil
	}

	var contacts []moneybirdContact
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if json.Unmarshal(body, &contacts) != nil || len(contacts) == 0 {
		return "", nil
	}

	for _, contact := range contacts {
		if contact.ID == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(contact.Email), targetEmail) {
			return contact.ID, nil
		}
	}

	return "", nil
}

func (s *Service) moneybirdCreateContact(
	ctx context.Context,
	administrationID string,
	accessToken string,
	email *string,
	firstName *string,
	lastName *string,
) (string, error) {
	createEndpoint := fmt.Sprintf("%s/%s/contacts", moneybirdAPIBaseURL, administrationID)
	firstNameValue := strings.TrimSpace(ptrToString(firstName))
	lastNameValue := strings.TrimSpace(ptrToString(lastName))
	emailValue := strings.TrimSpace(ptrToString(email))

	contactPayload := map[string]any{
		"firstname": firstNameValue,
		"lastname":  lastNameValue,
	}
	if emailValue != "" {
		contactPayload["email"] = emailValue
	}
	if firstNameValue == "" || lastNameValue == "" {
		contactPayload["company_name"] = moneybirdContactCompanyNameFallback(firstNameValue, lastNameValue, emailValue)
	}

	payload := map[string]any{"contact": contactPayload}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, createEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build moneybird create contact request: %w", err)
	}
	req.Header.Set(httpHeaderAuthorization, authorizationBearerPrefix+accessToken)
	req.Header.Set(httpHeaderAccept, mimeApplicationJSON)
	req.Header.Set(httpHeaderContentType, mimeApplicationJSON)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("moneybird create contact request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		details := strings.TrimSpace(string(respBody))
		if details == "" {
			return "", apperr.BadRequest("moneybird contact create failed")
		}
		return "", apperr.BadRequest(fmt.Sprintf("moneybird contact create failed: %s", details))
	}

	var contact moneybirdContact
	if err := json.Unmarshal(respBody, &contact); err != nil {
		return "", fmt.Errorf("decode moneybird contact response: %w", err)
	}
	if contact.ID == "" {
		return "", apperr.BadRequest("moneybird contact id missing in response")
	}

	return contact.ID, nil
}

func moneybirdContactCompanyNameFallback(firstName string, lastName string, email string) string {
	fullName := strings.TrimSpace(strings.TrimSpace(firstName) + " " + strings.TrimSpace(lastName))
	if fullName != "" {
		return fullName
	}

	trimmedEmail := strings.TrimSpace(email)
	if trimmedEmail != "" {
		if idx := strings.Index(trimmedEmail, "@"); idx > 0 {
			localPart := strings.TrimSpace(trimmedEmail[:idx])
			if localPart != "" {
				return localPart
			}
		}
		return trimmedEmail
	}

	return "Unknown contact"
}

func (s *Service) moneybirdResolveTaxRateIDByBPS(ctx context.Context, administrationID string, accessToken string) (map[int]int64, error) {
	endpoint := fmt.Sprintf("%s/%s/tax_rates", moneybirdAPIBaseURL, administrationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build moneybird tax rates request: %w", err)
	}
	req.Header.Set(httpHeaderAuthorization, authorizationBearerPrefix+accessToken)
	req.Header.Set(httpHeaderAccept, mimeApplicationJSON)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("moneybird tax rates request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apperr.BadRequest("moneybird tax rates lookup failed")
	}

	var taxRates []moneybirdTaxRate
	if err := json.Unmarshal(body, &taxRates); err != nil {
		return nil, fmt.Errorf("decode moneybird tax rates response: %w", err)
	}

	result := map[int]int64{}
	for _, rate := range taxRates {
		rateID, err := strconv.ParseInt(strings.TrimSpace(rate.ID), 10, 64)
		if err != nil {
			continue
		}
		value := strings.TrimSpace(strings.ReplaceAll(rate.Percentage, ",", "."))
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		result[int(parsed*100)] = rateID
	}

	return result, nil
}
