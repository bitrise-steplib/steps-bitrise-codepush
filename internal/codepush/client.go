package codepush

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
