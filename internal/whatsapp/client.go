package whatsapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"portal_final_backend/platform/config"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/phone"
)

type Client struct {
	baseURL  string
	apiKey   string
	deviceID string
	http     *http.Client
	log      *logger.Logger
}

type gowaRequest struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

func NewClient(cfg config.WhatsAppConfig, log *logger.Logger) *Client {
	if cfg.GetWhatsAppURL() == "" {
		return nil
	}

	return &Client{
		baseURL:  strings.TrimRight(cfg.GetWhatsAppURL(), "/"),
		apiKey:   cfg.GetWhatsAppKey(),
		deviceID: cfg.GetWhatsAppDeviceID(),
		http:     &http.Client{Timeout: 10 * time.Second},
		log:      log,
	}
}

func (c *Client) SendMessage(ctx context.Context, phoneNumber string, message string) error {
	if c == nil {
		return nil
	}

	normalized := strings.TrimPrefix(phone.NormalizeE164(phoneNumber), "+")

	payload := gowaRequest{
		Phone:   normalized,
		Message: message,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal whatsapp payload: %w", err)
	}

	url := fmt.Sprintf("%s/send/message", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", formatAuthHeader(c.apiKey))
	}
	if c.deviceID != "" {
		req.Header.Set("X-Device-Id", c.deviceID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	c.log.Info("whatsapp sent via gowa", "phone", normalized)
	return nil
}

func formatAuthHeader(apiKey string) string {
	if strings.HasPrefix(strings.ToLower(apiKey), "basic ") {
		return apiKey
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(apiKey))
	return "Basic " + encoded
}
