package transport

// PartnerLogoPresignRequest is the request for a presigned logo upload URL.
type PartnerLogoPresignRequest struct {
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=100"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
}

// PartnerLogoPresignResponse returns a presigned logo upload URL.
type PartnerLogoPresignResponse struct {
	UploadURL string `json:"uploadUrl"`
	FileKey   string `json:"fileKey"`
	ExpiresAt int64  `json:"expiresAt"`
}

// SetPartnerLogoRequest stores logo metadata after upload.
type SetPartnerLogoRequest struct {
	FileKey     string `json:"fileKey" validate:"required,min=1,max=500"`
	FileName    string `json:"fileName" validate:"required,min=1,max=255"`
	ContentType string `json:"contentType" validate:"required,min=1,max=100"`
	SizeBytes   int64  `json:"sizeBytes" validate:"required,min=1"`
}

// PartnerLogoDownloadResponse returns a presigned download URL.
type PartnerLogoDownloadResponse struct {
	DownloadURL string `json:"downloadUrl"`
	ExpiresAt   int64  `json:"expiresAt"`
}
