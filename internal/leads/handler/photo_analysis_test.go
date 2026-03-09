package handler

import (
	"context"
	"io"
	"testing"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/leads/repository"
)

func TestPhotoAnalysisHandlerLoadImagesReturnsErrorWhenStorageMissing(t *testing.T) {
	h := &PhotoAnalysisHandler{bucket: "lead-service-attachments"}

	images, err := h.loadImages(context.Background(), []repository.Attachment{{
		FileKey:  "attachments/image-1.jpg",
		FileName: "image-1.jpg",
	}})
	if err == nil {
		t.Fatal("expected error when storage is not configured")
	}
	if len(images) != 0 {
		t.Fatalf("expected no images when storage is missing, got %d", len(images))
	}
}

func TestPhotoAnalysisHandlerLoadImagesSkipsNilReaders(t *testing.T) {
	h := &PhotoAnalysisHandler{
		storage: nilReaderStorage{},
		bucket:  "lead-service-attachments",
	}

	images, err := h.loadImages(context.Background(), []repository.Attachment{{
		FileKey:  "attachments/image-1.jpg",
		FileName: "image-1.jpg",
	}})
	if err != nil {
		t.Fatalf("expected nil error for nil reader download, got %v", err)
	}
	if len(images) != 0 {
		t.Fatalf("expected nil reader download to be skipped, got %d images", len(images))
	}
}

type nilReaderStorage struct{}

func (nilReaderStorage) GenerateUploadURL(context.Context, string, string, string, string, int64) (*storage.PresignedURL, error) {
	return nil, nil
}

func (nilReaderStorage) GenerateDownloadURL(context.Context, string, string) (*storage.PresignedURL, error) {
	return nil, nil
}

func (nilReaderStorage) DownloadFile(context.Context, string, string) (io.ReadCloser, error) {
	return nil, nil
}

func (nilReaderStorage) DeleteObject(context.Context, string, string) error {
	return nil
}

func (nilReaderStorage) UploadFile(context.Context, string, string, string, string, io.Reader, int64) (string, error) {
	return "", nil
}

func (nilReaderStorage) EnsureBucketExists(context.Context, string) error {
	return nil
}

func (nilReaderStorage) ValidateContentType(string) error {
	return nil
}

func (nilReaderStorage) ValidateFileSize(int64) error {
	return nil
}

func (nilReaderStorage) GetMaxFileSize() int64 {
	return 0
}