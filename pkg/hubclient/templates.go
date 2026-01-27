package hubclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ptone/scion-agent/pkg/apiclient"
)

// TemplateService handles template operations.
type TemplateService interface {
	// List returns templates matching the filter criteria.
	List(ctx context.Context, opts *ListTemplatesOptions) (*ListTemplatesResponse, error)

	// Get returns a single template by ID.
	Get(ctx context.Context, templateID string) (*Template, error)

	// Create creates a new template.
	Create(ctx context.Context, req *CreateTemplateRequest) (*CreateTemplateResponse, error)

	// Update updates a template.
	Update(ctx context.Context, templateID string, req *UpdateTemplateRequest) (*Template, error)

	// Delete removes a template.
	Delete(ctx context.Context, templateID string) error

	// Clone creates a copy of a template.
	Clone(ctx context.Context, templateID string, req *CloneTemplateRequest) (*Template, error)

	// RequestUploadURLs requests signed URLs for uploading template files.
	RequestUploadURLs(ctx context.Context, templateID string, files []FileUploadRequest) (*UploadResponse, error)

	// Finalize finalizes a template after file upload.
	Finalize(ctx context.Context, templateID string, manifest *TemplateManifest) (*Template, error)

	// RequestDownloadURLs requests signed URLs for downloading template files.
	RequestDownloadURLs(ctx context.Context, templateID string) (*DownloadResponse, error)

	// UploadFile uploads a file to the given signed URL.
	UploadFile(ctx context.Context, url string, method string, headers map[string]string, content io.Reader) error

	// DownloadFile downloads a file from the given signed URL.
	DownloadFile(ctx context.Context, url string) ([]byte, error)
}

// templateService is the implementation of TemplateService.
type templateService struct {
	c *client
}

// ListTemplatesOptions configures template list filtering.
type ListTemplatesOptions struct {
	Scope   string // Filter by scope (global, grove, user)
	GroveID string // Filter by grove
	Harness string // Filter by harness type
	Page    apiclient.PageOptions
}

// ListTemplatesResponse is the response from listing templates.
type ListTemplatesResponse struct {
	Templates []Template
	Page      apiclient.PageResult
}

// CreateTemplateRequest is the request for creating a template.
type CreateTemplateRequest struct {
	Name       string          `json:"name"`
	Harness    string          `json:"harness"`
	Scope      string          `json:"scope"`
	GroveID    string          `json:"groveId,omitempty"`
	Config     *TemplateConfig `json:"config,omitempty"`
	Visibility string          `json:"visibility,omitempty"`
}

// UpdateTemplateRequest is the request for updating a template.
type UpdateTemplateRequest struct {
	Name       string          `json:"name,omitempty"`
	Config     *TemplateConfig `json:"config,omitempty"`
	Visibility string          `json:"visibility,omitempty"`
}

// CloneTemplateRequest is the request for cloning a template.
type CloneTemplateRequest struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	GroveID string `json:"groveId,omitempty"`
}

// FileUploadRequest describes a file to upload.
type FileUploadRequest struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// CreateTemplateResponse is the response from creating a template.
type CreateTemplateResponse struct {
	Template    *Template       `json:"template"`
	UploadURLs  []UploadURLInfo `json:"uploadUrls,omitempty"`
	ManifestURL string          `json:"manifestUrl,omitempty"`
}

// UploadURLInfo contains a signed URL for uploading a file.
type UploadURLInfo struct {
	Path    string            `json:"path"`
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Expires time.Time         `json:"expires"`
}

// UploadResponse is the response containing signed upload URLs.
type UploadResponse struct {
	UploadURLs  []UploadURLInfo `json:"uploadUrls"`
	ManifestURL string          `json:"manifestUrl,omitempty"`
}

// FinalizeRequest is the request body for finalizing a template upload.
type FinalizeRequest struct {
	Manifest *TemplateManifest `json:"manifest"`
}

// TemplateManifest is the manifest of uploaded template files.
type TemplateManifest struct {
	Version string         `json:"version"`
	Harness string         `json:"harness,omitempty"`
	Files   []TemplateFile `json:"files"`
}

// DownloadResponse contains signed URLs for downloading template files.
type DownloadResponse struct {
	ManifestURL string            `json:"manifestUrl,omitempty"`
	Files       []DownloadURLInfo `json:"files"`
	Expires     time.Time         `json:"expires"`
}

// DownloadURLInfo contains info for downloading a file.
type DownloadURLInfo struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
	Hash string `json:"hash,omitempty"`
}

// List returns templates matching the filter criteria.
func (s *templateService) List(ctx context.Context, opts *ListTemplatesOptions) (*ListTemplatesResponse, error) {
	query := url.Values{}
	if opts != nil {
		if opts.Scope != "" {
			query.Set("scope", opts.Scope)
		}
		if opts.GroveID != "" {
			query.Set("groveId", opts.GroveID)
		}
		if opts.Harness != "" {
			query.Set("harness", opts.Harness)
		}
		opts.Page.ToQuery(query)
	}

	resp, err := s.c.transport.GetWithQuery(ctx, "/api/v1/templates", query, nil)
	if err != nil {
		return nil, err
	}

	type listResponse struct {
		Templates  []Template `json:"templates"`
		NextCursor string     `json:"nextCursor,omitempty"`
		TotalCount int        `json:"totalCount,omitempty"`
	}

	result, err := apiclient.DecodeResponse[listResponse](resp)
	if err != nil {
		return nil, err
	}

	return &ListTemplatesResponse{
		Templates: result.Templates,
		Page: apiclient.PageResult{
			NextCursor: result.NextCursor,
			TotalCount: result.TotalCount,
		},
	}, nil
}

// Get returns a single template by ID.
func (s *templateService) Get(ctx context.Context, templateID string) (*Template, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/templates/"+templateID, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Template](resp)
}

// Create creates a new template.
func (s *templateService) Create(ctx context.Context, req *CreateTemplateRequest) (*CreateTemplateResponse, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/templates", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[CreateTemplateResponse](resp)
}

// Update updates a template.
func (s *templateService) Update(ctx context.Context, templateID string, req *UpdateTemplateRequest) (*Template, error) {
	resp, err := s.c.transport.Patch(ctx, "/api/v1/templates/"+templateID, req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Template](resp)
}

// Delete removes a template.
func (s *templateService) Delete(ctx context.Context, templateID string) error {
	resp, err := s.c.transport.Delete(ctx, "/api/v1/templates/"+templateID, nil)
	if err != nil {
		return err
	}
	return apiclient.CheckResponse(resp)
}

// Clone creates a copy of a template.
func (s *templateService) Clone(ctx context.Context, templateID string, req *CloneTemplateRequest) (*Template, error) {
	resp, err := s.c.transport.Post(ctx, "/api/v1/templates/"+templateID+"/clone", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Template](resp)
}

// RequestUploadURLs requests signed URLs for uploading template files.
func (s *templateService) RequestUploadURLs(ctx context.Context, templateID string, files []FileUploadRequest) (*UploadResponse, error) {
	req := struct {
		Files []FileUploadRequest `json:"files"`
	}{
		Files: files,
	}
	resp, err := s.c.transport.Post(ctx, "/api/v1/templates/"+templateID+"/upload", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[UploadResponse](resp)
}

// Finalize finalizes a template after file upload.
func (s *templateService) Finalize(ctx context.Context, templateID string, manifest *TemplateManifest) (*Template, error) {
	req := FinalizeRequest{
		Manifest: manifest,
	}
	resp, err := s.c.transport.Post(ctx, "/api/v1/templates/"+templateID+"/finalize", req, nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[Template](resp)
}

// RequestDownloadURLs requests signed URLs for downloading template files.
func (s *templateService) RequestDownloadURLs(ctx context.Context, templateID string) (*DownloadResponse, error) {
	resp, err := s.c.transport.Get(ctx, "/api/v1/templates/"+templateID+"/download", nil)
	if err != nil {
		return nil, err
	}
	return apiclient.DecodeResponse[DownloadResponse](resp)
}

// UploadFile uploads a file to the given signed URL.
// For local storage (file:// URLs), this writes directly to the filesystem.
func (s *templateService) UploadFile(ctx context.Context, signedURL string, method string, headers map[string]string, content io.Reader) error {
	// Handle file:// URLs for local storage
	if strings.HasPrefix(signedURL, "file://") {
		return s.uploadToFile(signedURL, content)
	}

	if method == "" {
		method = http.MethodPut
	}

	// Read content into buffer for Content-Length
	var body bytes.Buffer
	if _, err := io.Copy(&body, content); err != nil {
		return fmt.Errorf("failed to read content: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, signedURL, &body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.ContentLength = int64(body.Len())

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Use a plain HTTP client for direct storage uploads
	httpClient := s.c.transport.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// uploadToFile writes content directly to a file path from a file:// URL.
func (s *templateService) uploadToFile(fileURL string, content io.Reader) error {
	// Parse file:// URL to get path
	path := strings.TrimPrefix(fileURL, "file://")

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create and write file
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, content); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}

// DownloadFile downloads a file from the given signed URL.
// For local storage (file:// URLs), this reads directly from the filesystem.
func (s *templateService) DownloadFile(ctx context.Context, signedURL string) ([]byte, error) {
	// Handle file:// URLs for local storage
	if strings.HasPrefix(signedURL, "file://") {
		path := strings.TrimPrefix(signedURL, "file://")
		return os.ReadFile(path)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use a plain HTTP client for direct storage downloads
	httpClient := s.c.transport.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return io.ReadAll(resp.Body)
}
