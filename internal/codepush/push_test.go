// Adapted from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/codepush/push_test.go). testOut now backs the local Logger interface instead of
// output.NewTest. The "does not export bitrise summary" case is unchanged (still true for this
// port). Zip cleanup is no longer performed after Push (see push.go's doc comment); the
// "successful end to end" case below asserts the zip persists at result.PackagePath instead of
// asserting it was removed, and a new "keeps package file after successful push" case makes the
// PackagePath contract explicit.
package codepush

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPush(t *testing.T) {
	t.Run("successful end to end", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)
		var capturedReq UploadURLRequest
		var capturedUploadBody []byte

		client := &mockClient{
			getUploadURLFunc: func(deploymentID, updateID string, req UploadURLRequest) (*UploadURLResponse, error) {
				capturedReq = req
				return &UploadURLResponse{
					URL:     "https://storage.example.com/upload",
					Method:  "PUT",
					Headers: map[string]string{"Content-Type": "application/zip"},
				}, nil
			},
			uploadFileFunc: func(req UploadFileRequest) error {
				assert.Equal(t, "https://storage.example.com/upload", req.URL)
				assert.Equal(t, "PUT", req.Method)
				capturedUploadBody, _ = io.ReadAll(req.Body)
				return nil
			},
			getUpdateStatusFunc: func(updateID string) (*UpdateStatus, error) {
				return &UpdateStatus{UpdateID: updateID, Status: StatusProcessedValid}, nil
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "test-token",
			AppVersion:   "1.0.0",
			Description:  "test update",
			Mandatory:    true,
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		result, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.NoError(t, err)

		assert.Equal(t, "1.0.0", result.AppVersion)
		assert.Equal(t, StatusProcessedValid, result.Status)
		assert.NotEmpty(t, result.UpdateID)
		assert.NotZero(t, result.FileSizeBytes)
		assert.InDelta(t, 100.0, result.Rollout, 0.001)

		assert.Equal(t, "1.0.0", capturedReq.AppVersion)
		assert.True(t, capturedReq.Mandatory)
		require.NotNil(t, capturedReq.Rollout)
		assert.InDelta(t, 100.0, *capturedReq.Rollout, 0.001)
		assert.NotEmpty(t, capturedUploadBody)

		// Unlike the ported CLI (which deletes the zip via defer os.Remove(zipPath)), this
		// step does not delete the built package: the step layer still needs it to export
		// under $BITRISE_DEPLOY_DIR. PackagePath must point at a file that still exists.
		require.NotEmpty(t, result.PackagePath)
		info, statErr := os.Stat(result.PackagePath)
		require.NoError(t, statErr, "zip package should still exist after push")
		assert.False(t, info.IsDir())
	})

	t.Run("keeps package file after successful push", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)

		client := &mockClient{}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "test-token",
			AppVersion:   "1.0.0",
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		result, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.NoError(t, err)

		require.NotEmpty(t, result.PackagePath)
		_, statErr := os.Stat(result.PackagePath)
		assert.NoError(t, statErr, "PackagePath must still exist so the step can export it to $BITRISE_DEPLOY_DIR")
	})

	t.Run("deployment name resolution", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)
		var resolvedDeploymentID string

		client := &mockClient{
			listDeploymentsFunc: func(appID string) ([]Deployment, error) {
				return []Deployment{
					{ID: "dep-aaa", Name: "Staging"},
					{ID: "dep-bbb", Name: "Production"},
				}, nil
			},
			getUploadURLFunc: func(deploymentID, updateID string, req UploadURLRequest) (*UploadURLResponse, error) {
				resolvedDeploymentID = deploymentID
				return &UploadURLResponse{URL: "https://example.com/upload", Method: "PUT"}, nil
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "Production",
			Token:        "test-token",
			AppVersion:   "1.0.0",
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		_, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.NoError(t, err)

		assert.Equal(t, "dep-bbb", resolvedDeploymentID)
	})

	t.Run("deployment UUID passthrough", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)
		listCalled := false

		client := &mockClient{
			listDeploymentsFunc: func(appID string) ([]Deployment, error) {
				listCalled = true
				return nil, nil
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "test-token",
			AppVersion:   "1.0.0",
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		_, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.NoError(t, err)

		assert.False(t, listCalled, "ListDeployments should not be called when UUID is provided")
	})

	t.Run("deployment name not found", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)

		client := &mockClient{
			listDeploymentsFunc: func(appID string) ([]Deployment, error) {
				return []Deployment{
					{ID: "dep-aaa", Name: "Staging"},
				}, nil
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "NonExistent",
			Token:        "test-token",
			AppVersion:   "1.0.0",
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		_, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.Error(t, err)
		assert.ErrorContains(t, err, "NonExistent")
	})

	t.Run("upload URL failure", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)

		client := &mockClient{
			getUploadURLFunc: func(deploymentID, updateID string, req UploadURLRequest) (*UploadURLResponse, error) {
				return nil, errors.New("API returned HTTP 500: internal error")
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "test-token",
			AppVersion:   "1.0.0",
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		_, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.Error(t, err)
		assert.ErrorContains(t, err, "upload URL")
	})

	t.Run("upload failure", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)

		client := &mockClient{
			uploadFileFunc: func(req UploadFileRequest) error {
				return errors.New("upload failed with HTTP 403: URL expired")
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "test-token",
			AppVersion:   "1.0.0",
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		_, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.Error(t, err)
		assert.ErrorContains(t, err, "uploading update")
	})

	t.Run("poll returns failed", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)

		client := &mockClient{
			getUpdateStatusFunc: func(updateID string) (*UpdateStatus, error) {
				return &UpdateStatus{
					UpdateID:     updateID,
					Status:       StatusProcessedError,
					StatusReason: "invalid bundle format",
				}, nil
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "test-token",
			AppVersion:   "1.0.0",
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		_, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.Error(t, err)
		assert.ErrorContains(t, err, "invalid bundle format")
	})

	t.Run("poll timeout", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)

		client := &mockClient{
			getUpdateStatusFunc: func(updateID string) (*UpdateStatus, error) {
				return &UpdateStatus{UpdateID: updateID, Status: StatusUploaded}, nil
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "test-token",
			AppVersion:   "1.0.0",
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		_, err := PushWithConfig(context.Background(), client, opts, PollConfig{MaxAttempts: 2, Interval: 1 * time.Millisecond}, testOut)
		require.Error(t, err)
		assert.ErrorContains(t, err, "timed out")
	})

	t.Run("rollout is sent and captured in result", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)
		var capturedReq UploadURLRequest

		client := &mockClient{
			getUploadURLFunc: func(deploymentID, updateID string, req UploadURLRequest) (*UploadURLResponse, error) {
				capturedReq = req
				return &UploadURLResponse{URL: "https://example.com/upload", Method: "PUT"}, nil
			},
			uploadFileFunc: func(req UploadFileRequest) error { return nil },
			getUpdateStatusFunc: func(updateID string) (*UpdateStatus, error) {
				return &UpdateStatus{UpdateID: updateID, Status: StatusProcessedValid}, nil
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "tok",
			AppVersion:   "1.0.0",
			Rollout:      50,
			BundlePath:   bundleDir,
		}

		result, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.NoError(t, err)

		assert.InDelta(t, 50.0, result.Rollout, 0.001)
		require.NotNil(t, capturedReq.Rollout)
		assert.InDelta(t, 50.0, *capturedReq.Rollout, 0.001)
	})

	t.Run("rollout zero is sent to server not dropped", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)
		var capturedReq UploadURLRequest

		client := &mockClient{
			getUploadURLFunc: func(deploymentID, updateID string, req UploadURLRequest) (*UploadURLResponse, error) {
				capturedReq = req
				return &UploadURLResponse{URL: "https://example.com/upload", Method: "PUT"}, nil
			},
			uploadFileFunc: func(req UploadFileRequest) error { return nil },
			getUpdateStatusFunc: func(updateID string) (*UpdateStatus, error) {
				return &UpdateStatus{UpdateID: updateID, Status: StatusProcessedValid}, nil
			},
		}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "tok",
			AppVersion:   "1.0.0",
			Rollout:      0,
			BundlePath:   bundleDir,
		}

		result, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.NoError(t, err)

		assert.InDelta(t, 0.0, result.Rollout, 0.001)
		require.NotNil(t, capturedReq.Rollout)
		assert.InDelta(t, 0.0, *capturedReq.Rollout, 0.001)
	})

	t.Run("does not export bitrise summary", func(t *testing.T) {
		bundleDir := createTestBundleDir(t)
		deployDir := t.TempDir()
		t.Setenv("BITRISE_DEPLOY_DIR", deployDir)
		t.Setenv("BITRISE_BUILD_NUMBER", "42")

		client := &mockClient{}

		opts := &PushOptions{
			AppID:        "app-123",
			DeploymentID: "00000000-0000-0000-0000-000000000001",
			Token:        "test-token",
			AppVersion:   "2.0.0",
			Rollout:      100,
			BundlePath:   bundleDir,
		}

		_, err := PushWithConfig(context.Background(), client, opts, fastPollConfig, testOut)
		require.NoError(t, err)

		// Push does not export to Bitrise deploy dir; the step layer handles that.
		summaryPath := filepath.Join(deployDir, "codepush-push-summary.json")
		_, err = os.Stat(summaryPath)
		assert.Error(t, err, "push should not export summary; that responsibility lives in the step layer")
	})
}

func TestValidatePushOptions(t *testing.T) {
	bundleDir := createTestBundleDir(t)

	t.Run("rollout zero is valid", func(t *testing.T) {
		opts := PushOptions{AppID: "app", DeploymentID: "dep", Token: "tok", AppVersion: "1.0", Rollout: 0, BundlePath: bundleDir}
		require.NoError(t, validatePushOptions(&opts))
	})

	tests := []struct {
		name    string
		opts    PushOptions
		wantErr string
	}{
		{
			name:    "missing app ID",
			opts:    PushOptions{DeploymentID: "dep", Token: "tok", AppVersion: "1.0", Rollout: 100, BundlePath: bundleDir},
			wantErr: "app ID is required",
		},
		{
			name:    "missing deployment",
			opts:    PushOptions{AppID: "app", Token: "tok", AppVersion: "1.0", Rollout: 100, BundlePath: bundleDir},
			wantErr: "deployment is required",
		},
		{
			name:    "missing token",
			opts:    PushOptions{AppID: "app", DeploymentID: "dep", AppVersion: "1.0", Rollout: 100, BundlePath: bundleDir},
			wantErr: "API token is required",
		},
		{
			name:    "missing app version",
			opts:    PushOptions{AppID: "app", DeploymentID: "dep", Token: "tok", Rollout: 100, BundlePath: bundleDir},
			wantErr: "app version is required",
		},
		{
			name:    "missing bundle path",
			opts:    PushOptions{AppID: "app", DeploymentID: "dep", Token: "tok", AppVersion: "1.0", Rollout: 100},
			wantErr: "bundle path is required",
		},
		{
			name:    "rollout too low",
			opts:    PushOptions{AppID: "app", DeploymentID: "dep", Token: "tok", AppVersion: "1.0", Rollout: -1, BundlePath: bundleDir},
			wantErr: "rollout must be between 0 and 100",
		},
		{
			name:    "rollout too high",
			opts:    PushOptions{AppID: "app", DeploymentID: "dep", Token: "tok", AppVersion: "1.0", Rollout: 101, BundlePath: bundleDir},
			wantErr: "rollout must be between 0 and 100",
		},
		{
			name:    "bundle path does not exist",
			opts:    PushOptions{AppID: "app", DeploymentID: "dep", Token: "tok", AppVersion: "1.0", Rollout: 100, BundlePath: "/nonexistent"},
			wantErr: "bundle path does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePushOptions(&tt.opts)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestResolveDeployment(t *testing.T) {
	t.Run("UUID passthrough", func(t *testing.T) {
		client := &mockClient{}
		id, err := ResolveDeployment(context.Background(), client, "app-123", "00000000-0000-0000-0000-000000000001", testOut)
		require.NoError(t, err)
		assert.Equal(t, "00000000-0000-0000-0000-000000000001", id)
	})

	t.Run("name resolution", func(t *testing.T) {
		client := &mockClient{
			listDeploymentsFunc: func(appID string) ([]Deployment, error) {
				return []Deployment{
					{ID: "dep-aaa", Name: "Staging"},
					{ID: "dep-bbb", Name: "Production"},
				}, nil
			},
		}

		id, err := ResolveDeployment(context.Background(), client, "app-123", "Production", testOut)
		require.NoError(t, err)
		assert.Equal(t, "dep-bbb", id)
	})

	t.Run("name not found", func(t *testing.T) {
		client := &mockClient{
			listDeploymentsFunc: func(appID string) ([]Deployment, error) {
				return []Deployment{{ID: "dep-aaa", Name: "Staging"}}, nil
			},
		}

		_, err := ResolveDeployment(context.Background(), client, "app-123", "Production", testOut)
		require.Error(t, err)
		assert.ErrorContains(t, err, "Production")
	})

	t.Run("list deployments error", func(t *testing.T) {
		client := &mockClient{
			listDeploymentsFunc: func(appID string) ([]Deployment, error) {
				return nil, errors.New("network error")
			},
		}

		_, err := ResolveDeployment(context.Background(), client, "app-123", "Production", testOut)
		require.Error(t, err)
	})
}

func TestPollStatus(t *testing.T) {
	t.Run("returns on done", func(t *testing.T) {
		callCount := 0
		client := &mockClient{
			getUpdateStatusFunc: func(updateID string) (*UpdateStatus, error) {
				callCount++
				if callCount < 3 {
					return &UpdateStatus{UpdateID: updateID, Status: StatusUploaded}, nil
				}
				return &UpdateStatus{UpdateID: updateID, Status: StatusProcessedValid}, nil
			},
		}

		status, err := pollStatus(context.Background(), client, "pkg", PollConfig{MaxAttempts: 5, Interval: 1 * time.Millisecond})
		require.NoError(t, err)
		assert.Equal(t, StatusProcessedValid, status.Status)
		assert.Equal(t, 3, callCount)
	})

	t.Run("returns error on failed", func(t *testing.T) {
		client := &mockClient{
			getUpdateStatusFunc: func(updateID string) (*UpdateStatus, error) {
				return &UpdateStatus{UpdateID: updateID, Status: StatusProcessedError, StatusReason: "bad format"}, nil
			},
		}

		_, err := pollStatus(context.Background(), client, "pkg", fastPollConfig)
		require.Error(t, err)
		assert.ErrorContains(t, err, "bad format")
	})

	t.Run("times out", func(t *testing.T) {
		client := &mockClient{
			getUpdateStatusFunc: func(updateID string) (*UpdateStatus, error) {
				return &UpdateStatus{UpdateID: updateID, Status: StatusUploaded}, nil
			},
		}

		_, err := pollStatus(context.Background(), client, "pkg", PollConfig{MaxAttempts: 2, Interval: 1 * time.Millisecond})
		require.Error(t, err)
		assert.ErrorContains(t, err, "timed out")
	})

	t.Run("respects context cancellation during sleep", func(t *testing.T) {
		// Behavioral addition over the ported CLI: pollStatus now selects on ctx.Done()
		// during its interval sleep instead of an unconditional time.Sleep, so a canceled
		// context interrupts a stuck poll loop promptly instead of waiting out every attempt.
		client := &mockClient{
			getUpdateStatusFunc: func(updateID string) (*UpdateStatus, error) {
				return &UpdateStatus{UpdateID: updateID, Status: StatusUploaded}, nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := pollStatus(ctx, client, "pkg", PollConfig{MaxAttempts: 5, Interval: 1 * time.Hour})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func createTestBundleDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bundleDir := filepath.Join(dir, "bundle")
	require.NoError(t, os.Mkdir(bundleDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "main.jsbundle"), []byte("bundle"), 0o644))
	return bundleDir
}
