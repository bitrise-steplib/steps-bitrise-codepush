// Adapted from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/codepush/client_test.go). Trimmed to the four Client methods this port kept
// (ListDeployments, GetUploadURL, UploadFile, GetUpdateStatus); CRUD/rollback/promote/patch/list-
// updates test cases were dropped along with the methods they exercised (see client.go). The
// X-Bitrise-User-Agent expectation was updated from "codepush-cli/<version>" to
// "bitrise-codepush-step/<version>".
package codepush

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
			w.Write([]byte(`{"items":[{"id":"dep-1","name":"Staging"},{"id":"dep-2","name":"Production"}]}`))
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
			w.Write([]byte(`{"error":"invalid token"}`))
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
			w.Write([]byte(`{"items":[]}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-token", "test")
		deployments, err := client.ListDeployments(context.Background(), "app-123")
		require.NoError(t, err)
		assert.Empty(t, deployments)
	})
}

func TestHTTPClientGetUploadURL(t *testing.T) {
	t.Run("constructs correct request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/updates/pkg-789/upload-url", r.URL.Path)

			query := r.URL.Query()
			assert.Equal(t, "dep-456", query.Get("deployment_id"))
			assert.Equal(t, "1.0.0", query.Get("app_version"))
			assert.Equal(t, "bundle.zip", query.Get("file_name"))
			assert.Equal(t, "1024", query.Get("file_size_bytes"))
			assert.Equal(t, "true", query.Get("mandatory"))
			assert.Equal(t, "test update", query.Get("description"))
			assert.Empty(t, query.Get("rollout"))

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"url":"https://storage.example.com/upload","method":"PUT","headers":{"content_type":"application/zip"}}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-token", "test")
		resp, err := client.GetUploadURL(context.Background(), "dep-456", "pkg-789", UploadURLRequest{
			AppVersion:    "1.0.0",
			FileName:      "bundle.zip",
			FileSizeBytes: 1024,
			Description:   "test update",
			Mandatory:     true,
		})
		require.NoError(t, err)

		assert.Equal(t, "https://storage.example.com/upload", resp.URL)
		assert.Equal(t, "PUT", resp.Method)
	})

	t.Run("sends rollout=100 for full rollout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			query := r.URL.Query()
			assert.Empty(t, query.Get("description"))
			assert.Empty(t, query.Get("mandatory"))
			assert.Equal(t, "100", query.Get("rollout"))

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"url":"https://example.com/upload","method":"PUT","headers":{}}`))
		}))
		defer server.Close()

		rollout := 100.0
		client := NewHTTPClient(server.URL, "test-token", "test")
		_, err := client.GetUploadURL(context.Background(), "dep-456", "pkg-789", UploadURLRequest{
			AppVersion:    "1.0.0",
			FileName:      "bundle.zip",
			FileSizeBytes: 512,
			Rollout:       &rollout,
		})
		require.NoError(t, err)
	})

	t.Run("includes rollout in query params", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "25", r.URL.Query().Get("rollout"))

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"url":"https://example.com/upload","method":"PUT","headers":{}}`))
		}))
		defer server.Close()

		rollout := 25.0
		client := NewHTTPClient(server.URL, "test-token", "test")
		_, err := client.GetUploadURL(context.Background(), "dep-456", "pkg-789", UploadURLRequest{
			AppVersion:    "1.0.0",
			FileName:      "bundle.zip",
			FileSizeBytes: 512,
			Rollout:       &rollout,
		})
		require.NoError(t, err)
	})

	t.Run("sends rollout=0 for 0% rollout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "0", r.URL.Query().Get("rollout"))

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"url":"https://example.com/upload","method":"PUT","headers":{}}`))
		}))
		defer server.Close()

		rollout := 0.0
		client := NewHTTPClient(server.URL, "test-token", "test")
		_, err := client.GetUploadURL(context.Background(), "dep-456", "pkg-789", UploadURLRequest{
			AppVersion:    "1.0.0",
			FileName:      "bundle.zip",
			FileSizeBytes: 512,
			Rollout:       &rollout,
		})
		require.NoError(t, err)
	})

	t.Run("handles API error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"deployment not found"}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-token", "test")
		_, err := client.GetUploadURL(context.Background(), "dep-456", "pkg-789", UploadURLRequest{
			AppVersion:    "1.0.0",
			FileName:      "bundle.zip",
			FileSizeBytes: 512,
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "404")
	})
}

func TestHTTPClientUploadFile(t *testing.T) {
	t.Run("uploads file with correct headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPut, r.Method)
			assert.Equal(t, "application/zip", r.Header.Get("Content-Type"))
			assert.Equal(t, int64(11), r.ContentLength)

			body, _ := io.ReadAll(r.Body)
			assert.Equal(t, "zip content", string(body))

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient("", "test-token", "test")
		err := client.UploadFile(context.Background(), UploadFileRequest{
			URL:           server.URL,
			Method:        http.MethodPut,
			Headers:       map[string]string{"Content-Type": "application/zip"},
			Body:          strings.NewReader("zip content"),
			ContentLength: 11,
		})
		require.NoError(t, err)
	})

	t.Run("handles upload failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("URL expired"))
		}))
		defer server.Close()

		client := NewHTTPClient("", "test-token", "test")
		err := client.UploadFile(context.Background(), UploadFileRequest{
			URL:           server.URL,
			Method:        http.MethodPut,
			Body:          strings.NewReader("data"),
			ContentLength: 4,
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "403")
	})
}

func TestHTTPClientGetUpdateStatus(t *testing.T) {
	t.Run("returns status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/updates/pkg-789/status", r.URL.Path)

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"update_id":"pkg-789","status":"done","status_reason":""}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-token", "test")
		status, err := client.GetUpdateStatus(context.Background(), "pkg-789")
		require.NoError(t, err)

		assert.Equal(t, "pkg-789", status.UpdateID)
		assert.Equal(t, "done", status.Status)
	})

	t.Run("returns failed status with reason", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"update_id":"pkg-789","status":"failed","status_reason":"invalid bundle format"}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-token", "test")
		status, err := client.GetUpdateStatus(context.Background(), "pkg-789")
		require.NoError(t, err)

		assert.Equal(t, "failed", status.Status)
		assert.Equal(t, "invalid bundle format", status.StatusReason)
	})

	t.Run("handles HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-token", "test")
		_, err := client.GetUpdateStatus(context.Background(), "pkg-789")
		require.Error(t, err)
		assert.ErrorContains(t, err, "500")
	})
}

func TestHTTPClientSetsUserAgent(t *testing.T) {
	const expectedHeader = "bitrise-codepush-step/1.2.3"

	t.Run("doRequest sets X-Bitrise-User-Agent", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, expectedHeader, r.Header.Get("X-Bitrise-User-Agent"))
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[]}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "token", "1.2.3")
		_, err := client.ListDeployments(context.Background(), "app-1")
		require.NoError(t, err)
	})

	t.Run("UploadFile sets X-Bitrise-User-Agent", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, expectedHeader, r.Header.Get("X-Bitrise-User-Agent"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient("", "token", "1.2.3")
		err := client.UploadFile(context.Background(), UploadFileRequest{
			URL:           server.URL,
			Method:        http.MethodPut,
			Body:          strings.NewReader("data"),
			ContentLength: 4,
		})
		require.NoError(t, err)
	})

	t.Run("empty version falls back to unknown", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "bitrise-codepush-step/unknown", r.Header.Get("X-Bitrise-User-Agent"))
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[]}`))
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "token", "")
		_, err := client.ListDeployments(context.Background(), "app-1")
		require.NoError(t, err)
	})
}
