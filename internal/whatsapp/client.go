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
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
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

const headerContentType = "Content-Type"
const errPhoneNumberRequired = "phone number is required"

type MediaAttachment struct {
	Filename string
	Data     []byte
}

type SendImageInput struct {
	PhoneNumber     string
	Caption         string
	ViewOnce        bool
	Compress        bool
	IsForwarded     bool
	DurationSeconds *int
	Attachment      *MediaAttachment
	RemoteURL       string
}

type SendVideoInput struct {
	PhoneNumber     string
	Caption         string
	ViewOnce        bool
	Compress        bool
	IsForwarded     bool
	DurationSeconds *int
	Attachment      *MediaAttachment
	RemoteURL       string
}

type SendAudioInput struct {
	PhoneNumber     string
	IsForwarded     bool
	PTT             bool
	DurationSeconds *int
	Attachment      *MediaAttachment
	RemoteURL       string
}

type SendFileInput struct {
	PhoneNumber     string
	Caption         string
	IsForwarded     bool
	DurationSeconds *int
	Attachment      *MediaAttachment
	RemoteURL       string
}

type SendStickerInput struct {
	PhoneNumber     string
	IsForwarded     bool
	DurationSeconds *int
	Attachment      *MediaAttachment
	RemoteURL       string
}

type SendContactInput struct {
	PhoneNumber     string `json:"phone"`
	ContactName     string `json:"contact_name"`
	ContactPhone    string `json:"contact_phone"`
	IsForwarded     bool   `json:"is_forwarded,omitempty"`
	DurationSeconds *int   `json:"duration,omitempty"`
}

type SendLinkInput struct {
	PhoneNumber     string `json:"phone"`
	Link            string `json:"link"`
	Caption         string `json:"caption,omitempty"`
	IsForwarded     bool   `json:"is_forwarded,omitempty"`
	DurationSeconds *int   `json:"duration,omitempty"`
}

type SendLocationInput struct {
	PhoneNumber     string `json:"phone"`
	Latitude        string `json:"latitude"`
	Longitude       string `json:"longitude"`
	IsForwarded     bool   `json:"is_forwarded,omitempty"`
	DurationSeconds *int   `json:"duration,omitempty"`
}

type SendPollInput struct {
	PhoneNumber     string   `json:"phone"`
	Question        string   `json:"question"`
	Options         []string `json:"options"`
	MaxAnswer       int      `json:"max_answer"`
	DurationSeconds *int     `json:"duration,omitempty"`
}

type ReactMessageInput struct {
	PhoneNumber string `json:"phone"`
	Emoji       string `json:"emoji"`
}

type UpdateMessageInput struct {
	PhoneNumber string `json:"phone"`
	Message     string `json:"message"`
}

type MessageTargetInput struct {
	PhoneNumber string `json:"phone"`
}

type MessageStarInput struct {
	PhoneNumber string `json:"phone"`
	Value       bool
}

type ToggleChatInput struct {
	Value bool
}

type SetDisappearingTimerInput struct {
	TimerSeconds int
}

type actionRequest struct {
	Type   string `json:"type,omitempty"`
	Phone  string `json:"phone,omitempty"`
	Action string `json:"action,omitempty"`
}

type ChatPresenceAction string

const (
	ChatPresenceComposing ChatPresenceAction = "composing"
	ChatPresenceRecording ChatPresenceAction = "recording"
	ChatPresencePaused    ChatPresenceAction = "paused"
	chatPresenceStartCompat                  = "start"
	chatPresenceStopCompat                   = "stop"
)

type providerActionResponse struct {
	Code    string `json:"code"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Results struct {
		MessageID string `json:"message_id"`
		Status    string `json:"status"`
	} `json:"results"`
}

type providerDownloadMediaResponse struct {
	Code    string `json:"code"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Results struct {
		MessageID string `json:"message_id"`
		Status    string `json:"status"`
		MediaType string `json:"media_type"`
		MimeType  string `json:"mime_type"`
		Filename  string `json:"filename"`
		FileName  string `json:"file_name"`
		FilePath  string `json:"file_path"`
		FileSize  int64  `json:"file_size"`
		Data      string `json:"data"`
	} `json:"results"`
}

type DownloadMediaResult struct {
	MessageID   string
	MediaType   string
	Filename    string
	FilePath    string
	FileSize    int64
	DownloadURL string
}

type DownloadMediaFileResult struct {
	DownloadMediaResult
	ContentType string
	Data        []byte
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

func (c *Client) SendImage(ctx context.Context, deviceID string, input SendImageInput) (SendResult, error) {
	fields := buildMediaFields(input.PhoneNumber)
	fields["caption"] = strings.TrimSpace(input.Caption)
	fields["view_once"] = boolString(input.ViewOnce)
	fields["compress"] = boolString(input.Compress)
	fields["is_forwarded"] = boolString(input.IsForwarded)
	addOptionalIntField(fields, "duration", input.DurationSeconds)
	addOptionalStringField(fields, "image_url", input.RemoteURL)
	return c.sendMultipartMedia(ctx, deviceID, "/send/image", fields, "image", input.Attachment)
}

func (c *Client) SendVideo(ctx context.Context, deviceID string, input SendVideoInput) (SendResult, error) {
	fields := buildMediaFields(input.PhoneNumber)
	fields["caption"] = strings.TrimSpace(input.Caption)
	fields["view_once"] = boolString(input.ViewOnce)
	fields["compress"] = boolString(input.Compress)
	fields["is_forwarded"] = boolString(input.IsForwarded)
	addOptionalIntField(fields, "duration", input.DurationSeconds)
	addOptionalStringField(fields, "video_url", input.RemoteURL)
	return c.sendMultipartMedia(ctx, deviceID, "/send/video", fields, "video", input.Attachment)
}

func (c *Client) SendAudio(ctx context.Context, deviceID string, input SendAudioInput) (SendResult, error) {
	fields := buildMediaFields(input.PhoneNumber)
	fields["is_forwarded"] = boolString(input.IsForwarded)
	fields["ptt"] = boolString(input.PTT)
	addOptionalIntField(fields, "duration", input.DurationSeconds)
	addOptionalStringField(fields, "audio_url", input.RemoteURL)
	return c.sendMultipartMedia(ctx, deviceID, "/send/audio", fields, "audio", input.Attachment)
}

func (c *Client) SendFile(ctx context.Context, deviceID string, input SendFileInput) (SendResult, error) {
	fields := buildMediaFields(input.PhoneNumber)
	fields["caption"] = strings.TrimSpace(input.Caption)
	fields["is_forwarded"] = boolString(input.IsForwarded)
	addOptionalIntField(fields, "duration", input.DurationSeconds)
	addOptionalStringField(fields, "file_url", input.RemoteURL)
	return c.sendMultipartMedia(ctx, deviceID, "/send/file", fields, "file", input.Attachment)
}

func (c *Client) SendSticker(ctx context.Context, deviceID string, input SendStickerInput) (SendResult, error) {
	fields := buildMediaFields(input.PhoneNumber)
	fields["is_forwarded"] = boolString(input.IsForwarded)
	addOptionalIntField(fields, "duration", input.DurationSeconds)
	addOptionalStringField(fields, "sticker_url", input.RemoteURL)
	return c.sendMultipartMedia(ctx, deviceID, "/send/sticker", fields, "sticker", input.Attachment)
}

func (c *Client) SendContact(ctx context.Context, deviceID string, input SendContactInput) (SendResult, error) {
	input.PhoneNumber = normalizeRecipient(input.PhoneNumber)
	input.ContactPhone = normalizeRecipient(input.ContactPhone)
	return c.sendJSONMessage(ctx, deviceID, "/send/contact", input)
}

func (c *Client) SendLink(ctx context.Context, deviceID string, input SendLinkInput) (SendResult, error) {
	input.PhoneNumber = normalizeRecipient(input.PhoneNumber)
	return c.sendJSONMessage(ctx, deviceID, "/send/link", input)
}

func (c *Client) SendLocation(ctx context.Context, deviceID string, input SendLocationInput) (SendResult, error) {
	input.PhoneNumber = normalizeRecipient(input.PhoneNumber)
	return c.sendJSONMessage(ctx, deviceID, "/send/location", input)
}

func (c *Client) SendPoll(ctx context.Context, deviceID string, input SendPollInput) (SendResult, error) {
	input.PhoneNumber = normalizeRecipient(input.PhoneNumber)
	return c.sendJSONMessage(ctx, deviceID, "/send/poll", input)
}

func (c *Client) ReactMessage(ctx context.Context, deviceID string, messageID string, input ReactMessageInput) error {
	input.PhoneNumber = normalizeRecipient(input.PhoneNumber)
	_, err := c.postJSONAction(ctx, fmt.Sprintf("%s/message/%s/reaction", c.baseURL, url.PathEscape(strings.TrimSpace(messageID))), deviceID, map[string]any{
		"phone": input.PhoneNumber,
		"emoji": strings.TrimSpace(input.Emoji),
	})
	return err
}

func (c *Client) UpdateMessage(ctx context.Context, deviceID string, messageID string, input UpdateMessageInput) error {
	input.PhoneNumber = normalizeRecipient(input.PhoneNumber)
	_, err := c.postJSONAction(ctx, fmt.Sprintf("%s/message/%s/update", c.baseURL, url.PathEscape(strings.TrimSpace(messageID))), deviceID, map[string]any{
		"phone":   input.PhoneNumber,
		"message": strings.TrimSpace(input.Message),
	})
	return err
}

func (c *Client) DeleteMessage(ctx context.Context, deviceID string, messageID string, input MessageTargetInput) error {
	input.PhoneNumber = normalizeRecipient(input.PhoneNumber)
	_, err := c.postJSONAction(ctx, fmt.Sprintf("%s/message/%s/delete", c.baseURL, url.PathEscape(strings.TrimSpace(messageID))), deviceID, input)
	return err
}

func (c *Client) RevokeMessage(ctx context.Context, deviceID string, messageID string, input MessageTargetInput) error {
	input.PhoneNumber = normalizeRecipient(input.PhoneNumber)
	_, err := c.postJSONAction(ctx, fmt.Sprintf("%s/message/%s/revoke", c.baseURL, url.PathEscape(strings.TrimSpace(messageID))), deviceID, input)
	return err
}

func (c *Client) StarMessage(ctx context.Context, deviceID string, messageID string, input MessageStarInput) error {
	input.PhoneNumber = normalizeRecipient(input.PhoneNumber)
	endpoint := "%s/message/%s/star"
	if !input.Value {
		endpoint = "%s/message/%s/unstar"
	}
	_, err := c.postJSONAction(ctx, fmt.Sprintf(endpoint, c.baseURL, url.PathEscape(strings.TrimSpace(messageID))), deviceID, map[string]any{
		"phone": input.PhoneNumber,
	})
	return err
}

func (c *Client) DownloadMedia(ctx context.Context, deviceID string, messageID string, phoneNumber string) (DownloadMediaResult, error) {
	phone := normalizeRecipient(phoneNumber)
	if phone == "" {
		return DownloadMediaResult{}, errors.New(errPhoneNumberRequired)
	}

	endpoint := fmt.Sprintf("%s/message/%s/download?phone=%s", c.baseURL, url.PathEscape(strings.TrimSpace(messageID)), url.QueryEscape(phone))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return DownloadMediaResult{}, err
	}
	c.addHeaders(req, deviceID)
	data, err := c.doRequest(req)
	if err != nil {
		return DownloadMediaResult{}, err
	}

	var parsed providerDownloadMediaResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return DownloadMediaResult{}, fmt.Errorf("parse download media response: %w", err)
	}

	// API v8 returns file_name; older versions return filename.
	fileName := coalesceStr(parsed.Results.FileName, parsed.Results.Filename)
	// API v8 returns mime_type; older versions return media_type.
	mediaType := coalesceStr(parsed.Results.MimeType, parsed.Results.MediaType)

	return DownloadMediaResult{
		MessageID:   strings.TrimSpace(parsed.Results.MessageID),
		MediaType:   strings.TrimSpace(mediaType),
		Filename:    strings.TrimSpace(fileName),
		FilePath:    strings.TrimSpace(parsed.Results.FilePath),
		FileSize:    parsed.Results.FileSize,
		DownloadURL: c.resolveProviderAssetURL(strings.TrimSpace(parsed.Results.FilePath)),
	}, nil
}

func (c *Client) DownloadMediaFile(ctx context.Context, deviceID string, messageID string, phoneNumber string) (DownloadMediaFileResult, error) {
	normPhone := normalizeRecipient(phoneNumber)
	if normPhone == "" {
		return DownloadMediaFileResult{}, errors.New(errPhoneNumberRequired)
	}

	endpoint := fmt.Sprintf("%s/message/%s/download?phone=%s", c.baseURL, url.PathEscape(strings.TrimSpace(messageID)), url.QueryEscape(normPhone))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return DownloadMediaFileResult{}, err
	}
	c.addHeaders(req, deviceID)
	raw, err := c.doRequest(req)
	if err != nil {
		return DownloadMediaFileResult{}, err
	}

	var parsed providerDownloadMediaResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return DownloadMediaFileResult{}, fmt.Errorf("parse download media response: %w", err)
	}

	// Normalise field names across API versions.
	fileName := coalesceStr(parsed.Results.FileName, parsed.Results.Filename)
	mediaType := coalesceStr(parsed.Results.MimeType, parsed.Results.MediaType)

	result := DownloadMediaResult{
		MessageID:   strings.TrimSpace(parsed.Results.MessageID),
		MediaType:   strings.TrimSpace(mediaType),
		Filename:    strings.TrimSpace(fileName),
		FilePath:    strings.TrimSpace(parsed.Results.FilePath),
		FileSize:    parsed.Results.FileSize,
		DownloadURL: c.resolveProviderAssetURL(strings.TrimSpace(parsed.Results.FilePath)),
	}

	// API v8+ returns media data inline as base64.
	if b64 := strings.TrimSpace(parsed.Results.Data); b64 != "" {
		decoded, decErr := base64.StdEncoding.DecodeString(b64)
		if decErr != nil {
			decoded, decErr = base64.RawStdEncoding.DecodeString(b64)
		}
		if decErr == nil && len(decoded) > 0 {
			contentType := strings.TrimSpace(mediaType)
			if contentType == "" {
				contentType = inferContentType(result.Filename, result.FilePath, result.MediaType)
			}
			return DownloadMediaFileResult{
				DownloadMediaResult: result,
				ContentType:         contentType,
				Data:                decoded,
			}, nil
		}
	}

	// Fallback: fetch binary from the file_path URL (older provider versions).
	if strings.TrimSpace(result.DownloadURL) == "" {
		return DownloadMediaFileResult{}, fmt.Errorf("download url is missing for media message")
	}

	data, contentType, err := c.fetchBinaryFromURL(ctx, result.DownloadURL)
	if err != nil {
		return DownloadMediaFileResult{}, err
	}
	if contentType == "" {
		contentType = inferContentType(result.Filename, result.FilePath, result.MediaType)
	}

	return DownloadMediaFileResult{
		DownloadMediaResult: result,
		ContentType:         contentType,
		Data:                data,
	}, nil
}

func (c *Client) ArchiveChat(ctx context.Context, deviceID string, chatJID string, archived bool) error {
	_, err := c.postJSONAction(ctx, fmt.Sprintf("%s/chat/%s/archive", c.baseURL, url.PathEscape(strings.TrimSpace(chatJID))), deviceID, map[string]any{"archived": archived})
	return err
}

func (c *Client) PinChat(ctx context.Context, deviceID string, chatJID string, pinned bool) error {
	_, err := c.postJSONAction(ctx, fmt.Sprintf("%s/chat/%s/pin", c.baseURL, url.PathEscape(strings.TrimSpace(chatJID))), deviceID, map[string]any{"pinned": pinned})
	return err
}

func (c *Client) SetDisappearingTimer(ctx context.Context, deviceID string, chatJID string, timerSeconds int) error {
	_, err := c.postJSONAction(ctx, fmt.Sprintf("%s/chat/%s/disappearing", c.baseURL, url.PathEscape(strings.TrimSpace(chatJID))), deviceID, map[string]any{"timer_seconds": timerSeconds})
	return err
}

func coalesceStr(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
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
		return nil, errors.New(errWhatsAppClientNotInitialized)
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
	ct := resp.Header.Get(headerContentType)
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

func (c *Client) resolveProviderAssetURL(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return c.resolveGoWAURL(trimmed)
	}
	baseParsed, err := url.Parse(c.baseURL)
	if err != nil {
		return trimmed
	}
	resolved := baseParsed.ResolveReference(&url.URL{Path: "/" + strings.TrimLeft(trimmed, "/")})
	return resolved.String()
}

// fetchImageFromURL downloads an image from the given URL with auth headers.
func (c *Client) fetchImageFromURL(ctx context.Context, imageURL string) ([]byte, error) {
	data, _, err := c.fetchBinaryFromURL(ctx, imageURL)
	return data, err
}

func (c *Client) fetchBinaryFromURL(ctx context.Context, targetURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "*/*")
	if c.apiKey != "" {
		req.Header.Set("Authorization", formatAuthHeader(c.apiKey))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch provider asset: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("provider asset fetch returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get(headerContentType), ";")[0])
	return data, contentType, nil
}

func inferContentType(fileName string, filePath string, mediaType string) string {
	for _, candidate := range []string{fileName, filePath} {
		ext := strings.ToLower(strings.TrimSpace(path.Ext(candidate)))
		if ext == "" {
			continue
		}
		if contentType := strings.TrimSpace(mime.TypeByExtension(ext)); contentType != "" {
			return contentType
		}
	}

	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image":
		return "image/jpeg"
	case "video":
		return "video/mp4"
	case "audio":
		return "audio/mpeg"
	default:
		return ""
	}
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
		return errors.New(errPhoneNumberRequired)
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

	phone := normalizeActionPhone(phoneNumber)
	if phone == "" {
		return errors.New(errPhoneNumberRequired)
	}
	normalized, compat, err := normalizeChatPresenceAction(action)
	if err != nil {
		return err
	}

	primaryPayload := actionRequest{
		Phone: phone,
		Type:  normalized,
	}
	if primaryPayload.Type == "" {
		return fmt.Errorf("action is required")
	}
	endpoint := fmt.Sprintf("%s/send/chat-presence", c.baseURL)
	_, err = c.postJSONAction(ctx, endpoint, deviceID, primaryPayload)
	if err == nil {
		return nil
	}

	fallbackPayload := actionRequest{
		Phone:  phone,
		Action: compat,
	}
	_, fallbackErr := c.postJSONAction(ctx, endpoint, deviceID, fallbackPayload)
	if fallbackErr == nil {
		return nil
	}
	return err
}

func normalizeChatPresenceAction(raw string) (string, string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case string(ChatPresenceComposing), chatPresenceStartCompat:
		return string(ChatPresenceComposing), chatPresenceStartCompat, nil
	case string(ChatPresencePaused), chatPresenceStopCompat:
		return string(ChatPresencePaused), chatPresenceStopCompat, nil
	case string(ChatPresenceRecording):
		return string(ChatPresenceRecording), chatPresenceStartCompat, nil
	default:
		return "", "", fmt.Errorf("action is required")
	}
}

func (c *Client) GetDeviceStatus(ctx context.Context, deviceID string) (*DeviceStatusResponse, error) {
	if c == nil {
		return nil, errors.New(errWhatsAppClientNotInitialized)
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
		return nil, errors.New(errWhatsAppClientNotInitialized)
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
	data, err := c.doJSONRequest(ctx, http.MethodPost, endpoint, deviceID, payload)
	if err != nil {
		return "", err
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

func (c *Client) sendJSONMessage(ctx context.Context, deviceID string, path string, payload any) (SendResult, error) {
	if c == nil {
		return SendResult{}, nil
	}
	data, err := c.doJSONRequest(ctx, http.MethodPost, c.baseURL+path, deviceID, payload)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{MessageID: parseSendMessageID(data)}, nil
}

func (c *Client) sendMultipartMedia(ctx context.Context, deviceID string, path string, fields map[string]string, fileField string, attachment *MediaAttachment) (SendResult, error) {
	if c == nil {
		return SendResult{}, nil
	}
	data, err := c.doMultipartRequest(ctx, c.baseURL+path, deviceID, fields, fileField, attachment)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{MessageID: parseSendMessageID(data)}, nil
}

func (c *Client) doJSONRequest(ctx context.Context, method string, endpoint string, deviceID string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal whatsapp payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	c.addHeaders(req, deviceID)
	return c.doRequest(req)
}

func (c *Client) doMultipartRequest(ctx context.Context, endpoint string, deviceID string, fields map[string]string, fileField string, attachment *MediaAttachment) ([]byte, error) {
	if attachment == nil && strings.TrimSpace(fields[fileURLFieldName(fileField)]) == "" {
		return nil, fmt.Errorf("attachment or remote url is required")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writeMultipartFields(writer, fields); err != nil {
		return nil, err
	}
	if err := writeMultipartAttachment(writer, fileField, attachment); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", formatAuthHeader(c.apiKey))
	}
	if deviceID != "" {
		req.Header.Set("X-Device-Id", deviceID)
	}
	req.Header.Set(headerContentType, writer.FormDataContentType())
	return c.doRequest(req)
}

func (c *Client) doRequest(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("whatsapp service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func normalizeActionPhone(value string) string {
	return normalizeRecipient(value)
}

func normalizeRecipient(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "@") {
		return trimmed
	}
	return strings.TrimPrefix(phone.NormalizeE164(trimmed), "+")
}

func buildMediaFields(phoneNumber string) map[string]string {
	return map[string]string{"phone": normalizeRecipient(phoneNumber)}
}

func addOptionalIntField(fields map[string]string, key string, value *int) {
	if value == nil {
		return
	}
	fields[key] = fmt.Sprintf("%d", *value)
}

func addOptionalStringField(fields map[string]string, key string, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	fields[key] = trimmed
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func attachmentFilename(attachment *MediaAttachment, fallback string) string {
	if attachment == nil {
		return fallback
	}
	trimmed := strings.TrimSpace(attachment.Filename)
	if trimmed != "" {
		return trimmed
	}
	return fallback
}

func fileURLFieldName(fileField string) string {
	switch fileField {
	case "image":
		return "image_url"
	case "video":
		return "video_url"
	case "audio":
		return "audio_url"
	case "sticker":
		return "sticker_url"
	default:
		return fileField + "_url"
	}
}

func writeMultipartFields(writer *multipart.Writer, fields map[string]string) error {
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return fmt.Errorf("write multipart field %s: %w", key, err)
		}
	}
	return nil
}

func writeMultipartAttachment(writer *multipart.Writer, fileField string, attachment *MediaAttachment) error {
	if attachment == nil {
		return nil
	}
	part, err := writer.CreateFormFile(fileField, attachmentFilename(attachment, fileField))
	if err != nil {
		return fmt.Errorf("create multipart file %s: %w", fileField, err)
	}
	if _, err := part.Write(attachment.Data); err != nil {
		return fmt.Errorf("write multipart file %s: %w", fileField, err)
	}
	return nil
}

func (c *Client) addHeaders(req *http.Request, deviceID string) {
	req.Header.Set(headerContentType, "application/json")
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
