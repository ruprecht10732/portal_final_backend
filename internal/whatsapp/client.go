package whatsapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/phone"
)

// gowaLoginResponse is the JSON envelope GoWA returns for /devices/:id/login.
// Results.QRLink is a URL pointing to a static QR image on the GoWA server.
type gowaLoginResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Results struct {
		QRLink     string `json:"qr_link"`
		QRDuration int    `json:"qr_duration"`
	} `json:"results"`
}

type Client struct {
	baseURL           string
	baseHost          string
	apiKey            string
	apiKeyFingerprint string
	http              *http.Client
	log               *logger.Logger
}

type gowaRequest struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

type DeviceInput struct {
	DeviceID string `json:"device_id"`
}

// gowaStatusResponse is the raw JSON envelope from GoWA's /devices/:id/status.
type gowaStatusResponse struct {
	Code    string `json:"code"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Results struct {
		DeviceID    string `json:"device_id"`
		IsConnected bool   `json:"is_connected"`
		IsLoggedIn  bool   `json:"is_logged_in"`
	} `json:"results"`
}

type gowaDeviceInfoResponse struct {
	Code    string `json:"code"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Results struct {
		ID          string `json:"id"`
		DeviceID    string `json:"device_id"`
		Device      string `json:"device"`
		DisplayName string `json:"display_name"`
		PhoneNumber string `json:"phone_number"`
		State       string `json:"state"`
		JID         string `json:"jid"`
	} `json:"results"`
}

// DeviceStatusResponse is the normalised device status exposed to callers.
type DeviceStatusResponse struct {
	DeviceID    string
	IsConnected bool
	IsLoggedIn  bool
}

type DeviceInfoResponse struct {
	DeviceID    string
	DisplayName string
	PhoneNumber string
	State       string
	JID         string
}

type SendResult struct {
	MessageID string
}

type actionRequest struct {
	Type   string `json:"type,omitempty"`
	Phone  string `json:"phone,omitempty"`
	Action string `json:"action,omitempty"`
}

type providerActionResponse struct {
	Code    string `json:"code"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Results struct {
		MessageID string `json:"message_id"`
		Status    string `json:"status"`
	} `json:"results"`
}

var ErrNoDevice = errors.New("no whatsapp device configured")

const errWhatsAppClientNotInitialized = "whatsapp client not initialized"
const errProviderDeviceNotFound = "device not found in provider"

func NewClient(cfg config.WhatsAppConfig, log *logger.Logger) *Client {
	if cfg.GetWhatsAppURL() == "" {
		return nil
	}

	return &Client{
		baseURL:           strings.TrimRight(cfg.GetWhatsAppURL(), "/"),
		baseHost:          hostFromURL(cfg.GetWhatsAppURL()),
		apiKey:            cfg.GetWhatsAppKey(),
		apiKeyFingerprint: fingerprintKey(cfg.GetWhatsAppKey()),
		http:              &http.Client{Timeout: 10 * time.Second},
		log:               log,
	}
}

func (c *Client) SendMessage(ctx context.Context, deviceID string, phoneNumber string, message string) (SendResult, error) {
	if c == nil {
		return SendResult{}, nil
	}

	targetDevice := strings.TrimSpace(deviceID)
	if targetDevice == "" {
		return SendResult{}, ErrNoDevice
	}

	normalized := strings.TrimPrefix(phone.NormalizeE164(phoneNumber), "+")
	payload := gowaRequest{
		Phone:   normalized,
		Message: message,
	}

	c.log.Info("whatsapp send attempt", "deviceId", targetDevice, "providerHost", c.baseHost, "apiKeyFp", c.apiKeyFingerprint, "phone", normalized)

	result, err := c.doSendMessage(ctx, targetDevice, payload)
	if err != nil && isConnectionError(err) {
		c.log.Warn("whatsapp connection lost, attempting reconnect", "deviceId", targetDevice)
		if reconErr := c.ReconnectDevice(ctx, targetDevice); reconErr == nil {
			time.Sleep(2 * time.Second)
			return c.doSendMessage(ctx, targetDevice, payload)
		}
	}

	if err != nil {
		c.log.Warn("whatsapp send failed", "deviceId", targetDevice, "providerHost", c.baseHost, "apiKeyFp", c.apiKeyFingerprint, "error", err)
	}

	if err == nil {
		c.log.Info("whatsapp sent via gowa", "phone", normalized, "deviceId", targetDevice, "messageId", result.MessageID)
	}
	return result, err
}

func hostFromURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func fingerprintKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])[:8]
}

func (c *Client) doSendMessage(ctx context.Context, deviceID string, payload gowaRequest) (SendResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return SendResult{}, fmt.Errorf("marshal whatsapp payload: %w", err)
	}

	url := fmt.Sprintf("%s/send/message", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return SendResult{}, fmt.Errorf("failed to create request: %w", err)
	}

	c.addHeaders(req, deviceID)

	resp, err := c.http.Do(req)
	if err != nil {
		return SendResult{}, fmt.Errorf("whatsapp request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if readErr != nil {
		return SendResult{}, fmt.Errorf("read whatsapp response: %w", readErr)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return SendResult{}, fmt.Errorf("whatsapp service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return SendResult{MessageID: parseSendMessageID(data)}, nil
}

func parseSendMessageID(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}

	for _, candidate := range []any{
		payload["message_id"],
		payload["id"],
		nestedMapValue(payload["results"], "message_id"),
		nestedMapValue(payload["results"], "id"),
		nestedMapValue(nestedMapValue(payload["results"], "message"), "id"),
	} {
		if id, ok := candidate.(string); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}

	return ""
}

func nestedMapValue(value any, key string) any {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return obj[key]
}

func (c *Client) CreateDevice(ctx context.Context, deviceID string) error {
	if c == nil {
		return nil
	}

	payload := DeviceInput{
		DeviceID: deviceID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal device payload: %w", err)
	}

	url := fmt.Sprintf("%s/devices", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	c.addHeaders(req, "")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusConflict {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		return fmt.Errorf("failed to create device, status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func (c *Client) GetLoginQR(ctx context.Context, deviceID string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf(errWhatsAppClientNotInitialized)
	}

	// Use a generous timeout for QR generation (WhatsApp handshake can be slow).
	qrCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1st attempt: per-device login endpoint (GoWA v8 multi-device)
	primaryURL := fmt.Sprintf("%s/devices/%s/login?output=image", c.baseURL, deviceID)
	qrBytes, fallback, err := c.fetchLoginQR(qrCtx, primaryURL, deviceID)
	if err == nil {
		return qrBytes, nil
	}
	if !fallback {
		return nil, err
	}

	// 2nd attempt: legacy endpoint with device_id query param
	fallbackURL := fmt.Sprintf("%s/app/login?output=image&device_id=%s", c.baseURL, deviceID)
	qrBytes, fallback, err = c.fetchLoginQR(qrCtx, fallbackURL, "")
	if err == nil {
		return qrBytes, nil
	}
	if !fallback {
		return nil, err
	}

	// 3rd attempt: plain legacy endpoint (single-device GoWA builds)
	finalFallbackURL := fmt.Sprintf("%s/app/login?output=image", c.baseURL)
	qrBytes, _, err = c.fetchLoginQR(qrCtx, finalFallbackURL, "")
	if err == nil {
		return qrBytes, nil
	}

	return nil, err
}

func (c *Client) fetchLoginQR(ctx context.Context, url string, deviceID string) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}

	c.addHeaders(req, deviceID)
	req.Header.Set("Accept", "image/png, image/*, application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		body := strings.TrimSpace(string(data))
		msgLower := strings.ToLower(body)
		// If the endpoint says "not implemented", signal the caller to try the next fallback.
		if resp.StatusCode >= http.StatusInternalServerError && strings.Contains(msgLower, "not implemented") {
			return nil, true, fmt.Errorf("failed to get QR, status %d: %s", resp.StatusCode, body)
		}
		// Also treat 404 as "try next" – some GoWA builds don't expose the endpoint.
		if resp.StatusCode == http.StatusNotFound {
			return nil, true, fmt.Errorf("QR endpoint not found: %d: %s", resp.StatusCode, body)
		}
		return nil, false, fmt.Errorf("failed to get QR, status %d: %s", resp.StatusCode, body)
	}

	qrBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, false, err
	}

	// If GoWA returned JSON instead of an image, extract the QR URL and fetch it.
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") || (len(qrBytes) > 0 && qrBytes[0] == '{') {
		if img, err := c.extractQRFromJSON(ctx, qrBytes); err == nil && img != nil {
			return img, false, nil
		}
		// JSON but no extractable image — treat as fallback-worthy.
		return nil, true, fmt.Errorf("QR endpoint returned JSON without image data")
	}

	return qrBytes, false, nil
}

// extractQRFromJSON parses the GoWA login JSON response and returns the QR
// image bytes. If qr_link is a URL (the normal case), the image is fetched
// from the GoWA server. Falls back to base64 data-URI decoding.
func (c *Client) extractQRFromJSON(ctx context.Context, data []byte) ([]byte, error) {
	var resp gowaLoginResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	qr := resp.Results.QRLink
	if qr == "" {
		return nil, fmt.Errorf("no qr_link in response")
	}

	// GoWA returns qr_link as a URL to a static PNG on the server.
	if strings.HasPrefix(qr, "http://") || strings.HasPrefix(qr, "https://") {
		resolved := c.resolveGoWAURL(qr)
		c.log.Info("fetching QR image from URL", "url", resolved)
		return c.fetchImageFromURL(ctx, resolved)
	}

	// Fallback: try to decode as base64 data URI.
	if idx := strings.Index(qr, ","); idx >= 0 {
		qr = qr[idx+1:]
	}
	decoded, err := base64.StdEncoding.DecodeString(qr)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(qr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode QR data: %w", err)
		}
	}
	return decoded, nil
}

// resolveGoWAURL rewrites a URL returned by GoWA so it uses the configured
// base URL's scheme and host (GoWA often returns http://localhost:3000/…).
func (c *Client) resolveGoWAURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	baseParsed, err := url.Parse(c.baseURL)
	if err != nil {
		return rawURL
	}
	parsed.Scheme = baseParsed.Scheme
	parsed.Host = baseParsed.Host
	return parsed.String()
}

// fetchImageFromURL downloads an image from the given URL with auth headers.
func (c *Client) fetchImageFromURL(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/png, image/*")
	if c.apiKey != "" {
		req.Header.Set("Authorization", formatAuthHeader(c.apiKey))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch QR image: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("QR image fetch returned %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

func (c *Client) DeleteDevice(ctx context.Context, deviceID string) error {
	if c == nil {
		return nil
	}

	url := fmt.Sprintf("%s/devices/%s", c.baseURL, deviceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	c.addHeaders(req, deviceID)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		return fmt.Errorf("failed to delete device, status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func (c *Client) MarkMessageRead(ctx context.Context, deviceID string, phoneNumber string, messageID string) error {
	if c == nil {
		return nil
	}

	normalizedMessageID := strings.TrimSpace(messageID)
	if normalizedMessageID == "" {
		return fmt.Errorf("message id is required")
	}

	payload := actionRequest{Phone: normalizeActionPhone(phoneNumber)}
	if payload.Phone == "" {
		return fmt.Errorf("phone number is required")
	}

	url := fmt.Sprintf("%s/message/%s/read", c.baseURL, url.PathEscape(normalizedMessageID))
	_, err := c.postJSONAction(ctx, url, deviceID, payload)
	return err
}

func (c *Client) SendPresence(ctx context.Context, deviceID string, presenceType string) error {
	if c == nil {
		return nil
	}

	payload := actionRequest{Type: strings.ToLower(strings.TrimSpace(presenceType))}
	if payload.Type == "" {
		return fmt.Errorf("presence type is required")
	}

	_, err := c.postJSONAction(ctx, fmt.Sprintf("%s/send/presence", c.baseURL), deviceID, payload)
	return err
}

func (c *Client) SendChatPresence(ctx context.Context, deviceID string, phoneNumber string, action string) error {
	if c == nil {
		return nil
	}

	payload := actionRequest{
		Phone:  normalizeActionPhone(phoneNumber),
		Action: strings.ToLower(strings.TrimSpace(action)),
	}
	if payload.Phone == "" {
		return fmt.Errorf("phone number is required")
	}
	if payload.Action == "" {
		return fmt.Errorf("action is required")
	}

	_, err := c.postJSONAction(ctx, fmt.Sprintf("%s/send/chat-presence", c.baseURL), deviceID, payload)
	return err
}

func (c *Client) GetDeviceStatus(ctx context.Context, deviceID string) (*DeviceStatusResponse, error) {
	if c == nil {
		return nil, fmt.Errorf(errWhatsAppClientNotInitialized)
	}

	url := fmt.Sprintf("%s/devices/%s/status", c.baseURL, deviceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	c.addHeaders(req, deviceID)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, apperr.NotFound(errProviderDeviceNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		body := strings.TrimSpace(string(data))
		if resp.StatusCode >= http.StatusInternalServerError {
			msgLower := strings.ToLower(body)
			if strings.Contains(msgLower, "device") && strings.Contains(msgLower, "not found") {
				return nil, apperr.NotFound(errProviderDeviceNotFound)
			}
		}
		return nil, fmt.Errorf("provider error: %d: %s", resp.StatusCode, body)
	}

	var raw gowaStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	return &DeviceStatusResponse{
		DeviceID:    raw.Results.DeviceID,
		IsConnected: raw.Results.IsConnected,
		IsLoggedIn:  raw.Results.IsLoggedIn,
	}, nil
}

func (c *Client) GetDeviceInfo(ctx context.Context, deviceID string) (*DeviceInfoResponse, error) {
	if c == nil {
		return nil, fmt.Errorf(errWhatsAppClientNotInitialized)
	}

	endpoint := fmt.Sprintf("%s/devices/%s", c.baseURL, url.PathEscape(deviceID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	c.addHeaders(req, deviceID)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, apperr.NotFound(errProviderDeviceNotFound)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider error: %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var raw gowaDeviceInfoResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	return &DeviceInfoResponse{
		DeviceID:    firstNonEmpty(raw.Results.ID, raw.Results.DeviceID, raw.Results.Device, deviceID),
		DisplayName: strings.TrimSpace(raw.Results.DisplayName),
		PhoneNumber: strings.TrimSpace(raw.Results.PhoneNumber),
		State:       strings.TrimSpace(raw.Results.State),
		JID:         strings.TrimSpace(raw.Results.JID),
	}, nil
}

func (c *Client) ReconnectDevice(ctx context.Context, deviceID string) error {
	if c == nil {
		return nil
	}

	url := fmt.Sprintf("%s/devices/%s/reconnect", c.baseURL, deviceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	c.addHeaders(req, deviceID)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		return fmt.Errorf("reconnect failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return nil
}

func (c *Client) postJSONAction(ctx context.Context, endpoint string, deviceID string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal whatsapp action payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	c.addHeaders(req, deviceID)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("whatsapp service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var parsed providerActionResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", nil
	}
	if strings.TrimSpace(parsed.Message) != "" {
		return strings.TrimSpace(parsed.Message), nil
	}
	if strings.TrimSpace(parsed.Results.Status) != "" {
		return strings.TrimSpace(parsed.Results.Status), nil
	}
	return "", nil
}

func normalizeActionPhone(value string) string {
	normalized := strings.TrimPrefix(phone.NormalizeE164(value), "+")
	return strings.TrimSpace(normalized)
}

func (c *Client) addHeaders(req *http.Request, deviceID string) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", formatAuthHeader(c.apiKey))
	}
	if deviceID != "" {
		req.Header.Set("X-Device-Id", deviceID)
	}
}

func isConnectionError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "client is not connected") || strings.Contains(msg, "context deadline exceeded")
}

func formatAuthHeader(apiKey string) string {
	if strings.HasPrefix(strings.ToLower(apiKey), "basic ") {
		return apiKey
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(apiKey))
	return "Basic " + encoded
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
