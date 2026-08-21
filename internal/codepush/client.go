package codepush

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type HTTPClient struct {
	BaseURL string
	Token   string
	version string
	client  *http.Client
}

func NewHTTPClient(baseURL, token, version string) *HTTPClient {
	if version == "" {
		version = "unknown"
	}
	return &HTTPClient{
		BaseURL: baseURL,
		Token:   token,
		version: version,
		client:  &http.Client{},
	}
}

// Path parameters must already be escaped with url.PathEscape by the caller.
func buildURL(path string, params url.Values) string {
	if len(params) == 0 {
		return path
	}
	return path + "?" + params.Encode()
}

func (c *HTTPClient) ListDeployments(ctx context.Context, appID string) ([]Deployment, error) {
	params := url.Values{}
	params.Set("app_id", appID)
	path := buildURL("/deployments", params)

	resp, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var result DeploymentListResponse
	if err := decodeResponse(resp, &result); err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	return result.Items, nil
}

func (c *HTTPClient) GetUploadURL(ctx context.Context, deploymentID, updateID string, req UploadURLRequest) (*UploadURLResponse, error) {
	path := fmt.Sprintf("/updates/%s/upload-url", url.PathEscape(updateID))

	params := url.Values{}
	params.Set("deployment_id", deploymentID)
	params.Set("app_version", req.AppVersion)
	params.Set("file_name", req.FileName)
	params.Set("file_size_bytes", strconv.FormatInt(req.FileSizeBytes, 10))
	if req.Description != "" {
		params.Set("description", req.Description)
	}
	if req.Mandatory {
		params.Set("mandatory", "true")
	}
	if req.Disabled {
		params.Set("disabled", "true")
	}
	if req.Rollout != nil {
		params.Set("rollout", strconv.FormatFloat(*req.Rollout, 'f', -1, 64))
	}

	resp, err := c.doRequest(ctx, http.MethodGet, buildURL(path, params))
	if err != nil {
		return nil, err
	}

	var result UploadURLResponse
	if err := decodeResponse(resp, &result); err != nil {
		return nil, fmt.Errorf("getting upload URL: %w", err)
	}

	return &result, nil
}

func (c *HTTPClient) UploadFile(ctx context.Context, ufr UploadFileRequest) error {
	req, err := http.NewRequestWithContext(ctx, ufr.Method, ufr.URL, ufr.Body)
	if err != nil {
		return fmt.Errorf("creating upload request: %w", err)
	}

	req.ContentLength = ufr.ContentLength
	for k, v := range ufr.Headers {
		req.Header.Set(k, v)
	}
	// Set after upload headers so the step's identity is always authoritative.
	req.Header.Set("X-Bitrise-User-Agent", "bitrise-codepush-step/"+c.version)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("uploading file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *HTTPClient) GetUpdateStatus(ctx context.Context, updateID string) (*UpdateStatus, error) {
	path := fmt.Sprintf("/updates/%s/status", url.PathEscape(updateID))

	resp, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var result UpdateStatus
	if err := decodeResponse(resp, &result); err != nil {
		return nil, fmt.Errorf("getting update status: %w", err)
	}

	return &result, nil
}

func (c *HTTPClient) doRequest(ctx context.Context, method, path string) (*http.Response, error) {
	reqURL := c.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Bitrise-User-Agent", "bitrise-codepush-step/"+c.version)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request to %s: %w", path, err)
	}

	return resp, nil
}

func decodeResponse(resp *http.Response, v any) error {
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}
