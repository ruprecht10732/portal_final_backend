package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	storageadapter "portal_final_backend/internal/adapters/storage"
	identityservice "portal_final_backend/internal/identity/service"
	quotesvc "portal_final_backend/internal/quotes/service"
	"portal_final_backend/internal/waagent"
	waagentdb "portal_final_backend/internal/waagent/db"
	"portal_final_backend/internal/whatsapp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type waagentQuotePDFGenerator interface {
	RegeneratePDF(ctx context.Context, quoteID, organizationID uuid.UUID) (string, []byte, error)
}

type WAAgentQuoteWorkflowAdapter struct {
	svc       *quotesvc.Service
	pdfGen    waagentQuotePDFGenerator
	storage   storageadapter.StorageService
	pdfBucket string
}

func NewWAAgentQuoteWorkflowAdapter(svc *quotesvc.Service, pdfGen waagentQuotePDFGenerator, storage storageadapter.StorageService, pdfBucket string) *WAAgentQuoteWorkflowAdapter {
	return &WAAgentQuoteWorkflowAdapter{svc: svc, pdfGen: pdfGen, storage: storage, pdfBucket: strings.TrimSpace(pdfBucket)}
}

func (a *WAAgentQuoteWorkflowAdapter) DraftQuote(ctx context.Context, orgID uuid.UUID, input waagent.DraftQuoteInput) (waagent.DraftQuoteOutput, error) {
	if a == nil || a.svc == nil {
		return waagent.DraftQuoteOutput{Success: false, Message: "Offerte-opbouw is niet beschikbaar"}, nil
	}
	leadID, serviceID, err := parseLeadAndServiceIDs(input.LeadID, input.LeadServiceID)
	if err != nil {
		return waagent.DraftQuoteOutput{Success: false, Message: err.Error()}, err
	}
	if len(input.Items) == 0 {
		return waagent.DraftQuoteOutput{Success: false, Message: "Ik mis nog de offerte-regels", MissingFields: []string{"offerte_regels"}}, nil
	}
	items := make([]quotesvc.DraftQuoteItemParams, 0, len(input.Items))
	for _, item := range input.Items {
		mapped := quotesvc.DraftQuoteItemParams{
			Description:    strings.TrimSpace(item.Description),
			Quantity:       strings.TrimSpace(item.Quantity),
			UnitPriceCents: item.UnitPriceCents,
			TaxRateBps:     item.TaxRateBps,
			IsOptional:     item.IsOptional,
		}
		if item.CatalogProductID != nil && strings.TrimSpace(*item.CatalogProductID) != "" {
			catalogID, parseErr := uuid.Parse(strings.TrimSpace(*item.CatalogProductID))
			if parseErr != nil {
				return waagent.DraftQuoteOutput{Success: false, Message: "Ongeldige catalog_product_id"}, parseErr
			}
			mapped.CatalogProductID = &catalogID
		}
		items = append(items, mapped)
	}
	result, err := a.svc.DraftQuote(ctx, quotesvc.DraftQuoteParams{
		LeadID:         leadID,
		LeadServiceID:  serviceID,
		OrganizationID: orgID,
		CreatedByID:    uuid.Nil,
		Notes:          strings.TrimSpace(input.Notes),
		Items:          items,
	})
	if err != nil {
		return mapDraftQuoteError(err)
	}
	return waagent.DraftQuoteOutput{
		Success:     true,
		Message:     fmt.Sprintf("Conceptofferte %s gemaakt", result.QuoteNumber),
		QuoteID:     result.QuoteID.String(),
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}

func (a *WAAgentQuoteWorkflowAdapter) GenerateQuote(ctx context.Context, orgID uuid.UUID, input waagent.GenerateQuoteInput) (waagent.GenerateQuoteOutput, error) {
	if a == nil || a.svc == nil {
		return waagent.GenerateQuoteOutput{Success: false, Message: "AI-offertegeneratie is niet beschikbaar"}, nil
	}
	leadID, serviceID, err := parseLeadAndServiceIDs(input.LeadID, input.LeadServiceID)
	if err != nil {
		return waagent.GenerateQuoteOutput{Success: false, Message: err.Error()}, err
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return waagent.GenerateQuoteOutput{Success: false, Message: "Ik mis nog een duidelijke offertevraag", MissingInformation: []string{"offertevraag"}}, nil
	}
	force := false
	if input.Force != nil {
		force = *input.Force
	}
	result, err := a.svc.GenerateQuote(ctx, orgID, leadID, serviceID, prompt, nil, force)
	if err != nil {
		return mapGenerateQuoteError(err)
	}
	return waagent.GenerateQuoteOutput{
		Success:     true,
		Message:     fmt.Sprintf("Conceptofferte %s gemaakt", result.QuoteNumber),
		QuoteID:     result.QuoteID.String(),
		QuoteNumber: result.QuoteNumber,
		ItemCount:   result.ItemCount,
	}, nil
}

func (a *WAAgentQuoteWorkflowAdapter) GetQuotePDF(ctx context.Context, orgID uuid.UUID, input waagent.SendQuotePDFInput) (waagent.QuotePDFResult, error) {
	if a == nil || a.svc == nil {
		return waagent.QuotePDFResult{}, fmt.Errorf("offerte-pdf is niet beschikbaar")
	}
	quoteID, err := uuid.Parse(strings.TrimSpace(input.QuoteID))
	if err != nil {
		return waagent.QuotePDFResult{}, fmt.Errorf("ongeldige quote_id")
	}
	quote, err := a.svc.GetByID(ctx, quoteID, orgID)
	if err != nil {
		return waagent.QuotePDFResult{}, err
	}
	fileName := strings.TrimSpace(quote.QuoteNumber) + ".pdf"
	if quote.PDFFileKey != nil && strings.TrimSpace(*quote.PDFFileKey) != "" && a.storage != nil && a.pdfBucket != "" {
		reader, downloadErr := a.storage.DownloadFile(ctx, a.pdfBucket, *quote.PDFFileKey)
		if downloadErr == nil {
			defer func() { _ = reader.Close() }()
			data, readErr := io.ReadAll(reader)
			if readErr == nil {
				return waagent.QuotePDFResult{QuoteID: quote.ID.String(), QuoteNumber: quote.QuoteNumber, FileName: fileName, Data: data}, nil
			}
		}
	}
	if a.pdfGen == nil {
		return waagent.QuotePDFResult{}, fmt.Errorf("geen pdf beschikbaar voor deze offerte")
	}
	_, pdfBytes, err := a.pdfGen.RegeneratePDF(ctx, quoteID, orgID)
	if err != nil {
		return waagent.QuotePDFResult{}, fmt.Errorf("offerte-pdf kon niet worden gemaakt")
	}
	return waagent.QuotePDFResult{QuoteID: quote.ID.String(), QuoteNumber: quote.QuoteNumber, FileName: fileName, Data: pdfBytes}, nil
}

type waagentPhotoLeadActions interface {
	CreateAttachment(ctx context.Context, params identityservice.CreateLeadAttachmentParams) (identityservice.CreateLeadAttachmentResult, error)
}

type waagentMediaDownloader interface {
	DownloadMediaFile(ctx context.Context, deviceID string, messageID string, phoneNumber string) (whatsapp.DownloadMediaFileResult, error)
}

type waagentAgentMessageReader interface {
	GetRecentInboundAgentMessages(ctx context.Context, arg waagentdb.GetRecentInboundAgentMessagesParams) ([]waagentdb.GetRecentInboundAgentMessagesRow, error)
}

type WAAgentCurrentInboundPhotoAdapter struct {
	whatsapp          waagentMediaDownloader
	storage           storageadapter.StorageService
	attachmentsBucket string
	leadActions       waagentPhotoLeadActions
	historyReader     waagentAgentMessageReader
}

func NewWAAgentCurrentInboundPhotoAdapter(whatsappClient waagentMediaDownloader, storage storageadapter.StorageService, attachmentsBucket string, leadActions waagentPhotoLeadActions, historyReader waagentAgentMessageReader) *WAAgentCurrentInboundPhotoAdapter {
	return &WAAgentCurrentInboundPhotoAdapter{
		whatsapp:          whatsappClient,
		storage:           storage,
		attachmentsBucket: strings.TrimSpace(attachmentsBucket),
		leadActions:       leadActions,
		historyReader:     historyReader,
	}
}

func (a *WAAgentCurrentInboundPhotoAdapter) AttachCurrentWhatsAppPhoto(ctx context.Context, orgID uuid.UUID, input waagent.AttachCurrentWhatsAppPhotoInput, message waagent.CurrentInboundMessage) (waagent.AttachCurrentWhatsAppPhotoOutput, error) {
	if a == nil || a.whatsapp == nil || a.storage == nil || a.leadActions == nil || a.attachmentsBucket == "" {
		return waagent.AttachCurrentWhatsAppPhotoOutput{Success: false, Message: "Foto-opslag is niet beschikbaar"}, nil
	}
	leadID, serviceID, err := parseLeadAndServiceIDs(input.LeadID, input.LeadServiceID)
	if err != nil {
		return waagent.AttachCurrentWhatsAppPhotoOutput{Success: false, Message: err.Error()}, err
	}
	photoSource, err := a.resolvePhotoSource(ctx, orgID, message)
	if err != nil {
		return waagent.AttachCurrentWhatsAppPhotoOutput{Success: false, Message: err.Error(), MissingFields: []string{"foto als afbeelding"}}, nil
	}
	fileResult, err := a.whatsapp.DownloadMediaFile(ctx, photoSource.DeviceID, photoSource.ExternalMessageID, photoSource.PhoneNumber)
	if err != nil {
		return waagent.AttachCurrentWhatsAppPhotoOutput{Success: false, Message: "WhatsApp-afbeelding kon niet worden opgehaald"}, err
	}
	contentType := normalizeWhatsAppImportContentType(fileResult.ContentType, fileResult.MediaType)
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return waagent.AttachCurrentWhatsAppPhotoOutput{Success: false, Message: "Het huidige WhatsApp-bericht bevat geen ondersteunde afbeelding", MissingFields: []string{"foto als jpg of png"}}, nil
	}
	if err := a.storage.ValidateContentType(contentType); err != nil {
		return waagent.AttachCurrentWhatsAppPhotoOutput{Success: false, Message: "Dit afbeeldingstype wordt niet ondersteund", MissingFields: []string{"jpg of png"}}, nil
	}
	sizeBytes := int64(len(fileResult.Data))
	if err := a.storage.ValidateFileSize(sizeBytes); err != nil {
		return waagent.AttachCurrentWhatsAppPhotoOutput{Success: false, Message: err.Error(), MissingFields: []string{"kleinere afbeelding"}}, nil
	}
	fileName := chooseWAAgentImportFilename(fileResult.Filename, photoSource.FilenameHint, contentType)
	folder := fmt.Sprintf("%s/%s/%s", orgID.String(), leadID.String(), serviceID.String())
	fileKey, err := a.storage.UploadFile(ctx, a.attachmentsBucket, folder, fileName, contentType, bytes.NewReader(fileResult.Data), sizeBytes)
	if err != nil {
		return waagent.AttachCurrentWhatsAppPhotoOutput{Success: false, Message: "WhatsApp-afbeelding kon niet worden opgeslagen"}, err
	}
	attachment, err := a.leadActions.CreateAttachment(ctx, identityservice.CreateLeadAttachmentParams{
		LeadID:         leadID,
		ServiceID:      serviceID,
		OrganizationID: orgID,
		AuthorID:       uuid.Nil,
		FileKey:        fileKey,
		FileName:       fileName,
		ContentType:    contentType,
		SizeBytes:      sizeBytes,
	})
	if err != nil {
		return waagent.AttachCurrentWhatsAppPhotoOutput{Success: false, Message: "WhatsApp-afbeelding kon niet aan de lead worden gekoppeld"}, err
	}
	return waagent.AttachCurrentWhatsAppPhotoOutput{
		Success:       true,
		Message:       "Foto toegevoegd aan de lead",
		AttachmentID:  attachment.AttachmentID.String(),
		LeadID:        leadID.String(),
		LeadServiceID: serviceID.String(),
	}, nil
}

type waagentPhotoSource struct {
	ExternalMessageID string
	PhoneNumber       string
	DeviceID          string
	FilenameHint      string
}

func (a *WAAgentCurrentInboundPhotoAdapter) resolvePhotoSource(ctx context.Context, orgID uuid.UUID, current waagent.CurrentInboundMessage) (waagentPhotoSource, error) {
	if source, ok := photoSourceFromCurrentMessage(current); ok {
		return source, nil
	}
	if a.historyReader == nil {
		return waagentPhotoSource{}, fmt.Errorf("Ik zie geen recente foto in dit gesprek")
	}
	recent, err := a.historyReader.GetRecentInboundAgentMessages(ctx, waagentdb.GetRecentInboundAgentMessagesParams{
		OrganizationID: pgtype.UUID{Bytes: orgID, Valid: true},
		PhoneNumber:    strings.TrimSpace(current.PhoneNumber),
		Limit:          10,
	})
	if err != nil {
		return waagentPhotoSource{}, err
	}
	for _, message := range recent {
		if source, ok := photoSourceFromStoredMessage(message); ok {
			return source, nil
		}
	}
	return waagentPhotoSource{}, fmt.Errorf("Ik zie geen recente foto in dit gesprek")
}

func photoSourceFromCurrentMessage(message waagent.CurrentInboundMessage) (waagentPhotoSource, bool) {
	deviceID, filenameHint, isImage := parseCurrentInboundMediaMetadata(message.Metadata)
	if !isImage || strings.TrimSpace(deviceID) == "" || strings.TrimSpace(message.ExternalMessageID) == "" {
		return waagentPhotoSource{}, false
	}
	return waagentPhotoSource{
		ExternalMessageID: strings.TrimSpace(message.ExternalMessageID),
		PhoneNumber:       strings.TrimSpace(message.PhoneNumber),
		DeviceID:          deviceID,
		FilenameHint:      filenameHint,
	}, true
}

func photoSourceFromStoredMessage(message waagentdb.GetRecentInboundAgentMessagesRow) (waagentPhotoSource, bool) {
	if !message.ExternalMessageID.Valid || strings.TrimSpace(message.ExternalMessageID.String) == "" {
		return waagentPhotoSource{}, false
	}
	deviceID, filenameHint, isImage := parseCurrentInboundMediaMetadata(message.Metadata)
	if !isImage || strings.TrimSpace(deviceID) == "" {
		return waagentPhotoSource{}, false
	}
	return waagentPhotoSource{
		ExternalMessageID: strings.TrimSpace(message.ExternalMessageID.String),
		PhoneNumber:       strings.TrimSpace(message.PhoneNumber),
		DeviceID:          deviceID,
		FilenameHint:      filenameHint,
	}, true
}

func parseLeadAndServiceIDs(leadIDRaw, serviceIDRaw string) (uuid.UUID, uuid.UUID, error) {
	leadID, err := uuid.Parse(strings.TrimSpace(leadIDRaw))
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("ongeldige lead_id")
	}
	serviceID, err := uuid.Parse(strings.TrimSpace(serviceIDRaw))
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("ongeldige lead_service_id")
	}
	return leadID, serviceID, nil
}

func mapDraftQuoteError(err error) (waagent.DraftQuoteOutput, error) {
	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	output := waagent.DraftQuoteOutput{Success: false, Message: message}
	if strings.Contains(lower, "onvoldoende intake") || strings.Contains(lower, "reliable conceptofferte") {
		output.MissingFields = []string{"werkbeschrijving", "hoeveelheden of extra foto's"}
	}
	if strings.Contains(lower, "quantity") {
		output.MissingFields = []string{"concrete hoeveelheden per regel"}
	}
	return output, err
}

func mapGenerateQuoteError(err error) (waagent.GenerateQuoteOutput, error) {
	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	output := waagent.GenerateQuoteOutput{Success: false, Message: message}
	if strings.Contains(lower, "onvoldoende") || strings.Contains(lower, "missing") || strings.Contains(lower, "intake") {
		output.MissingInformation = []string{"omschrijving van het werk", "relevante maten of foto's"}
	}
	return output, err
}

func parseCurrentInboundMediaMetadata(raw []byte) (deviceID string, filename string, isImage bool) {
	if len(raw) == 0 {
		return "", "", false
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", "", false
	}
	deviceID = strings.TrimSpace(stringValue(envelope["device_id"]))
	portal, _ := envelope["portal"].(map[string]any)
	if portal == nil {
		return deviceID, "", false
	}
	messageType := strings.TrimSpace(stringValue(portal["messageType"]))
	attachment, _ := portal["attachment"].(map[string]any)
	if attachment != nil && strings.TrimSpace(stringValue(attachment["filename"])) != "" {
		filename = strings.TrimSpace(stringValue(attachment["filename"]))
	}
	mediaType := ""
	if attachment != nil {
		mediaType = strings.TrimSpace(stringValue(attachment["mediaType"]))
	}
	isImage = messageType == "image" || mediaType == "image"
	return deviceID, filename, isImage
}

func chooseWAAgentImportFilename(downloadFilename string, filenameHint string, contentType string) string {
	for _, candidate := range []string{strings.TrimSpace(downloadFilename), strings.TrimSpace(filenameHint)} {
		if candidate != "" {
			return candidate
		}
	}
	ext := ".jpg"
	if extensions, err := mime.ExtensionsByType(contentType); err == nil && len(extensions) > 0 && strings.TrimSpace(extensions[0]) != "" {
		ext = extensions[0]
	}
	return fmt.Sprintf("whatsapp-image-%s%s", time.Now().UTC().Format("20060102-150405"), ext)
}

func normalizeWhatsAppImportContentType(contentType string, mediaType string) string {
	trimmed := strings.TrimSpace(contentType)
	if trimmed != "" {
		return trimmed
	}
	switch strings.TrimSpace(strings.ToLower(mediaType)) {
	case "image":
		return "image/jpeg"
	case "video":
		return "video/mp4"
	case "audio":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", value)
}
