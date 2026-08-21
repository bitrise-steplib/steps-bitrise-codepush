// Adapted from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/codepush/client_test.go). Trimmed to the one Client method this layer of the stack
// keeps (ListDeployments); the upload-path test cases land with GetUploadURL/UploadFile/
// GetUpdateStatus in a later PR. The X-Bitrise-User-Agent expectation was updated from
// "codepush-cli/<version>" to "bitrise-codepush-step/<version>".
package codepush

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		params url.Values
		want   string
	}{
		{"no params nil", "/deployments", nil, "/deployments"},
		{"no params empty", "/deployments", url.Values{}, "/deployments"},
		{"single param", "/deployments", url.Values{"app_id": {"abc-123"}}, "/deployments?app_id=abc-123"},
		{"multiple params sorted", "/updates", url.Values{"deployment_id": {"dep-1"}, "limit": {"10"}}, "/updates?deployment_id=dep-1&limit=10"},
		{"value needs encoding", "/updates", url.Values{"q": {"hello world"}}, "/updates?q=hello+world"},
		{"path with escaped segment", "/deployments/" + url.PathEscape("id/with/slashes"), nil, "/deployments/id%2Fwith%2Fslashes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildURL(tt.path, tt.params))
		})
	}
}

func TestPathEscapePreventsTraveral(t *testing.T) {
	// url.PathEscape encodes "/" as "%2F", so a traversal input like "../../../etc/passwd"
	// becomes a single literal path segment — the unescaped slashes that would allow
	// directory traversal are removed, confining the value to one segment.
	malicious := "../../../etc/passwd"
	path := "/deployments/" + url.PathEscape(malicious)
	assert.Equal(t, "/deployments/..%2F..%2F..%2Fetc%2Fpasswd", path)
	assert.NotContains(t, path, "../")
}

func TestHTTPClientListDeployments(t *testing.T) {
	t.Run("returns deployments", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/deployments", r.URL.Path)
			assert.Equal(t, "app-123", r.URL.Query().Get("app_id"))
			assert.Equal(t, "test-token", r.Header.Get("Authorization"))

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"dep-1","name":"Staging"},{"id":"dep-2","name":"Production"}]}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-token", "test")
		deployments, err := client.ListDeployments(context.Background(), "app-123")
		require.NoError(t, err)

		require.Len(t, deployments, 2)
		assert.Equal(t, "dep-1", deployments[0].ID)
		assert.Equal(t, "Staging", deployments[0].Name)
		assert.Equal(t, "dep-2", deployments[1].ID)
		assert.Equal(t, "Production", deployments[1].Name)
	})

	t.Run("handles HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid token"}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "bad-token", "test")
		_, err := client.ListDeployments(context.Background(), "app-123")
		require.Error(t, err)
		assert.ErrorContains(t, err, "401")
	})

	t.Run("handles empty list", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[]}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-token", "test")
		deployments, err := client.ListDeployments(context.Background(), "app-123")
		require.NoError(t, err)
		assert.Empty(t, deployments)
	})
}
