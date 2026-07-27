package nswclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
)

// UploadRequest represents the payload sent to initiate a file upload.
type UploadRequest struct {
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// FileMetadata mirrors the NSW backend's uploads.FileMetadata struct.
// It represents the full metadata of an uploaded file as returned by the backend.
type FileMetadata struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	URL       string `json:"url,omitempty"`
	UploadURL string `json:"upload_url,omitempty"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mime_type"`
}

// DownloadMetadata represents the response returned when a download URL is fetched.
type DownloadMetadata struct {
	DownloadURL string `json:"download_url"`
	ExpiresAt   int64  `json:"expires_at"`
}

const storageBasePath = "storage"

// GetDownloadURL fetches a (possibly presigned) download URL for a key from the
// NSW backend's storage metadata endpoint.
func (c *Client) GetDownloadURL(ctx context.Context, key string) (*DownloadMetadata, error) {
	// JoinPath treats its arguments as already-escaped path elements, so escape
	// key first to keep a slash-containing key within one segment.
	apiURL, err := url.JoinPath(storageBasePath, url.PathEscape(key))
	if err != nil {
		return nil, fmt.Errorf("failed to build storage metadata URL: %w", err)
	}

	resp, err := c.http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch upload metadata: %w", err)
	}
	defer closeBody(ctx, resp.Body)

	if resp.StatusCode != http.StatusOK {
		slog.WarnContext(ctx, "failed to fetch upload metadata",
			"key", key, "status", resp.Status)
		return nil, fmt.Errorf("failed to fetch upload metadata, status code: %d", resp.StatusCode)
	}

	var metadata DownloadMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode upload metadata: %w", err)
	}

	if metadata.DownloadURL == "" {
		return nil, fmt.Errorf("metadata response missing download_url")
	}

	// The download URL is presigned and may embed credentials, so it is not logged.
	slog.InfoContext(ctx, "resolved download URL from metadata", "key", key)
	return &metadata, nil
}

// CreateUploadURL proxies an upload initialization request to the NSW backend.
func (c *Client) CreateUploadURL(ctx context.Context, req UploadRequest) (*FileMetadata, error) {
	if req.Filename == "" || req.MimeType == "" || req.Size <= 0 {
		return nil, fmt.Errorf("invalid upload request: missing required fields")
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal upload request: %w", err)
	}

	resp, err := c.http.Post(storageBasePath, "application/json", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to POST upload metadata: %w", err)
	}
	defer closeBody(ctx, resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		decodeErr := json.NewDecoder(resp.Body).Decode(&errResp)
		errMsg := errResp["error"]
		if decodeErr != nil || errMsg == "" {
			errMsg = "unknown upstream error or invalid JSON response"
		}
		slog.WarnContext(ctx, "failed to fetch upload metadata from backend", "status", resp.Status, "error", errMsg)
		return nil, fmt.Errorf("backend error (status %d): %s", resp.StatusCode, errMsg)
	}

	var metadata FileMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode upload metadata: %w", err)
	}

	return &metadata, nil
}
