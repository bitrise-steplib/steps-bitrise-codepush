package codepush

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// PushOptions holds user-provided parameters for a push operation.
type PushOptions struct {
	AppID        string
	DeploymentID string
	Token        string
	AppVersion   string
	Description  string
	Mandatory    bool
	Disabled     bool
	Rollout      float64
	BundlePath   string
}

// UploadURLRequest represents the query parameters for requesting an upload URL.
// Rollout uses a pointer to distinguish "not set" (nil, param omitted) from zero (0% rollout sent explicitly).
type UploadURLRequest struct {
	AppVersion    string
	FileName      string
	FileSizeBytes int64
	Description   string
	Mandatory     bool
	Disabled      bool
	Rollout       *float64
}

// HeaderMap is a map[string]string that can unmarshal from either a JSON object
// or a JSON array of {"key": "...", "value": "..."} objects, as returned by
// the upload-url API endpoint.
type HeaderMap map[string]string

func (h *HeaderMap) UnmarshalJSON(data []byte) error {
	// Try object format first: {"Content-Type": "application/zip"}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err == nil {
		*h = m
		return nil
	}

	// Try array-of-objects format: [{"key": "k", "value": "v"}] or [{"name": "k", "value": "v"}]
	var arr []struct {
		Key   string `json:"key"`
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("headers: expected object or array of {key, value}, got %s", string(data))
	}

	result := make(map[string]string, len(arr))
	for _, item := range arr {
		k := item.Key
		if k == "" {
			k = item.Name
		}
		if k == "" {
			continue
		}
		result[k] = item.Value
	}
	*h = result
	return nil
}

// UploadURLResponse is returned by the GET upload-url endpoint.
type UploadURLResponse struct {
	URL     string    `json:"url"`
	Method  string    `json:"method"`
	Headers HeaderMap `json:"headers"`
}

// UploadFileRequest holds all parameters needed to upload a file.
type UploadFileRequest struct {
	URL           string
	Method        string
	Headers       map[string]string
	Body          io.Reader
	ContentLength int64
}

// UpdateStatus is returned by the GET status endpoint.
type UpdateStatus struct {
	UpdateID     string `json:"update_id"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason"`
}

// Deployment represents a CodePush deployment.
type Deployment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DeploymentListResponse wraps the list deployments API response.
type DeploymentListResponse struct {
	Items []Deployment `json:"items"`
}

// PushResult is the output of a successful push.
type PushResult struct {
	UpdateID      string  `json:"package_id"`
	AppID         string  `json:"app_id"`
	DeploymentID  string  `json:"deployment_id"`
	AppVersion    string  `json:"app_version"`
	Status        string  `json:"status"`
	FileSizeBytes int64   `json:"file_size_bytes"`
	Rollout       float64 `json:"rollout"`
	// PackagePath is the local path to the uploaded zip package. Not part of the ported CLI
	// result; added so the step can also export the package under $BITRISE_DEPLOY_DIR.
	PackagePath string `json:"-"`
}

// PollConfig controls the polling behavior when waiting for update processing.
type PollConfig struct {
	MaxAttempts int
	Interval    time.Duration
}

// DefaultPollConfig is used in production.
var DefaultPollConfig = PollConfig{
	MaxAttempts: 60,
	Interval:    2 * time.Second,
}

// Status constants for update processing.
const (
	StatusCreated        = "created"
	StatusUploaded       = "uploaded"
	StatusProcessedValid = "processed_valid"
	StatusProcessedError = "processed_invalid"
)

// Client defines the CodePush API operations needed for the push workflow.
type Client interface {
	ListDeployments(ctx context.Context, appID string) ([]Deployment, error)
	GetUploadURL(ctx context.Context, deploymentID, updateID string, req UploadURLRequest) (*UploadURLResponse, error)
	UploadFile(ctx context.Context, req UploadFileRequest) error
	GetUpdateStatus(ctx context.Context, updateID string) (*UpdateStatus, error)
}
