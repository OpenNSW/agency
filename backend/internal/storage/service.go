// Package storage handles file storage HTTP endpoints for the frontend,
// delegating the actual calls to the NSW backend to internal/nswclient.
package storage

import (
	"context"

	"github.com/OpenNSW/nsw-agency/backend/internal/nswclient"
)

// Wire DTOs and sentinel errors are owned by nswclient (they mirror the NSW backend contract).
// They are aliased here so the handler and its callers keep a stable,
// storage-local vocabulary.
type (
	// UploadRequest is the payload sent by the frontend to initiate an upload.
	UploadRequest = nswclient.UploadRequest
	// FileMetadata is the full metadata of an uploaded file.
	FileMetadata = nswclient.FileMetadata
	// DownloadMetadata is the response returned when a download URL is fetched.
	DownloadMetadata = nswclient.DownloadMetadata
)

var (
	ErrInvalidUploadRequest = nswclient.ErrInvalidUploadRequest
	ErrProhibitedFileType   = nswclient.ErrProhibitedFileType
	ErrDisallowedMimeType   = nswclient.ErrDisallowedMimeType
	ErrInvalidFilename      = nswclient.ErrInvalidFilename
	ErrFileSizeExceeded     = nswclient.ErrFileSizeExceeded
)

// Service is the subset of the NSW client that the storage handler depends on.
// It is satisfied directly by *nswclient.Client.
type Service interface {
	// GetDownloadURL fetches a download URL for a key from the main backend.
	GetDownloadURL(ctx context.Context, key string) (*DownloadMetadata, error)

	// CreateUploadURL proxies an upload initialization request to the main backend.
	CreateUploadURL(ctx context.Context, req UploadRequest) (*FileMetadata, error)
}
