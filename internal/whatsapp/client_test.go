package whatsapp

import (
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portal_final_backend/platform/logger"
)

const testUnexpectedPathFmt = "unexpected path %q"
const testPhoneNumber = "+31612345678"

func TestGetDeviceInfoReturnsAccountJID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/devices/org_test" {
			t.Fatalf(testUnexpectedPathFmt, r.URL.Path)
		}
		if got := r.Header.Get("X-Device-Id"); got != "org_test" {
			t.Fatalf("expected X-Device-Id header, got %q", got)
		}
		w.Header().Set(headerContentType, "application/json")
		_, _ = w.Write([]byte(`{"status":200,"code":"SUCCESS","results":{"id":"org_test","display_name":"Robin","jid":"31619330634@s.whatsapp.net","state":"logged_in"}}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL:           server.URL,
		baseHost:          server.Listener.Addr().String(),
		apiKey:            "secret",
		apiKeyFingerprint: "fp",
		http:              &http.Client{Timeout: time.Second},
		log:               logger.New("development"),
	}

	info, err := client.GetDeviceInfo(context.Background(), "org_test")
	if err != nil {
		t.Fatalf("GetDeviceInfo returned error: %v", err)
	}
	if info.DeviceID != "org_test" {
		t.Fatalf("expected device id org_test, got %q", info.DeviceID)
	}
	if info.JID != "31619330634@s.whatsapp.net" {
		t.Fatalf("expected JID to be parsed, got %q", info.JID)
	}
	if info.State != "logged_in" {
		t.Fatalf("expected state logged_in, got %q", info.State)
	}
}

func TestFormatAuthHeaderUsesBasicPrefix(t *testing.T) {
	t.Parallel()

	got := formatAuthHeader("abc123")
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("abc123"))
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSendImageUsesMultipartFormAndParsesMessageID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertMultipartImageRequest(t, r)
		w.Header().Set(headerContentType, "application/json")
		_, _ = w.Write([]byte(`{"status":200,"code":"SUCCESS","results":{"message_id":"msg-123"}}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL:           server.URL,
		baseHost:          server.Listener.Addr().String(),
		apiKey:            "secret",
		apiKeyFingerprint: "fp",
		http:              &http.Client{Timeout: time.Second},
		log:               logger.New("development"),
	}

	result, err := client.SendImage(context.Background(), "org_test", SendImageInput{
		PhoneNumber: testPhoneNumber,
		Caption:     "Hello",
		ViewOnce:    true,
		Compress:    false,
		Attachment:  &MediaAttachment{Filename: "photo.jpg", Data: []byte("image-bytes")},
	})
	if err != nil {
		t.Fatalf("SendImage returned error: %v", err)
	}
	if result.MessageID != "msg-123" {
		t.Fatalf("expected message id msg-123, got %q", result.MessageID)
	}
}

func TestSendPollUsesJSONPayload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/send/poll" {
			t.Fatalf(testUnexpectedPathFmt, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Basic ") {
			t.Fatalf("expected basic auth header, got %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		payload := string(body)
		if !strings.Contains(payload, `"phone":"31612345678"`) {
			t.Fatalf("expected normalized phone in payload, got %s", payload)
		}
		if !strings.Contains(payload, `"question":"Best day?"`) {
			t.Fatalf("expected question in payload, got %s", payload)
		}
		if !strings.Contains(payload, `"max_answer":2`) {
			t.Fatalf("expected max_answer in payload, got %s", payload)
		}
		_, _ = w.Write([]byte(`{"status":200,"code":"SUCCESS","results":{"message_id":"poll-1"}}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL:           server.URL,
		baseHost:          server.Listener.Addr().String(),
		apiKey:            "secret",
		apiKeyFingerprint: "fp",
		http:              &http.Client{Timeout: time.Second},
		log:               logger.New("development"),
	}

	result, err := client.SendPoll(context.Background(), "org_test", SendPollInput{
		PhoneNumber: testPhoneNumber,
		Question:    "Best day?",
		Options:     []string{"Mon", "Tue"},
		MaxAnswer:   2,
	})
	if err != nil {
		t.Fatalf("SendPoll returned error: %v", err)
	}
	if result.MessageID != "poll-1" {
		t.Fatalf("expected message id poll-1, got %q", result.MessageID)
	}
}

func TestNormalizeRecipientPreservesJIDs(t *testing.T) {
	t.Parallel()

	if got := normalizeRecipient("120363025982934543@g.us"); got != "120363025982934543@g.us" {
		t.Fatalf("expected group jid to be preserved, got %q", got)
	}
	if got := normalizeRecipient(testPhoneNumber); got != "31612345678" {
		t.Fatalf("expected phone to be normalized, got %q", got)
	}
}

func TestMediaDownloadPhoneCandidatesIncludeJIDFallback(t *testing.T) {
	t.Parallel()

	got := mediaDownloadPhoneCandidates(testPhoneNumber)
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "31612345678") {
		t.Fatalf("expected bare phone candidate, got %v", got)
	}
	if !strings.Contains(joined, "31612345678@s.whatsapp.net") {
		t.Fatalf("expected JID phone candidate, got %v", got)
	}
	if !strings.Contains(joined, "+31612345678") {
		t.Fatalf("expected plus-prefixed phone candidate, got %v", got)
	}
	if len(got) < 3 {
		t.Fatalf("expected multiple phone candidates, got %v", got)
	}
}

func assertMultipartImageRequest(t *testing.T, r *http.Request) {
	t.Helper()

	if r.URL.Path != "/send/image" {
		t.Fatalf(testUnexpectedPathFmt, r.URL.Path)
	}
	if got := r.Header.Get("X-Device-Id"); got != "org_test" {
		t.Fatalf("expected X-Device-Id header, got %q", got)
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get(headerContentType))
	if err != nil {
		t.Fatalf("parse media type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("expected multipart/form-data, got %q", mediaType)
	}

	fields, filename, fileData := readMultipartRequest(t, r, params["boundary"])
	if filename != "photo.jpg" {
		t.Fatalf("unexpected filename %q", filename)
	}
	if fileData != "image-bytes" {
		t.Fatalf("unexpected file data %q", fileData)
	}
	if fields["phone"] != "31612345678" {
		t.Fatalf("unexpected phone field %q", fields["phone"])
	}
	if fields["caption"] != "Hello" {
		t.Fatalf("unexpected caption field %q", fields["caption"])
	}
	if fields["view_once"] != "true" {
		t.Fatalf("unexpected view_once field %q", fields["view_once"])
	}
	if fields["compress"] != "false" {
		t.Fatalf("unexpected compress field %q", fields["compress"])
	}
}

func readMultipartRequest(t *testing.T, r *http.Request, boundary string) (map[string]string, string, string) {
	t.Helper()

	reader := multipart.NewReader(r.Body, boundary)
	fields := map[string]string{}
	filename := ""
	fileData := ""
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		if part.FormName() == "image" {
			filename = part.FileName()
			fileData = string(data)
			continue
		}
		fields[part.FormName()] = string(data)
	}
	if filename == "" {
		t.Fatal("expected image file part")
	}
	return fields, filename, fileData
}
