// Adapted from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/codepush/testhelpers_test.go). mockClient is trimmed to only the four Client methods
// this port kept (ListDeployments, GetUploadURL, UploadFile, GetUpdateStatus); the CRUD/rollback/
// promote/patch mock hooks were dropped along with the interface methods they backed. testOut
// (output.NewTest) was replaced with a no-op Logger test double.
package codepush

import (
	"context"
	"time"
)

type mockClient struct {
	listDeploymentsFunc func(appID string) ([]Deployment, error)
	getUploadURLFunc    func(deploymentID, updateID string, req UploadURLRequest) (*UploadURLResponse, error)
	uploadFileFunc      func(req UploadFileRequest) error
	getUpdateStatusFunc func(updateID string) (*UpdateStatus, error)
}

func (m *mockClient) ListDeployments(_ context.Context, appID string) ([]Deployment, error) {
	if m.listDeploymentsFunc != nil {
		return m.listDeploymentsFunc(appID)
	}
	return nil, nil
}

func (m *mockClient) GetUploadURL(_ context.Context, deploymentID, updateID string, req UploadURLRequest) (*UploadURLResponse, error) {
	if m.getUploadURLFunc != nil {
		return m.getUploadURLFunc(deploymentID, updateID, req)
	}
	return &UploadURLResponse{URL: "https://example.com/upload", Method: "PUT"}, nil
}

func (m *mockClient) UploadFile(_ context.Context, req UploadFileRequest) error {
	if m.uploadFileFunc != nil {
		return m.uploadFileFunc(req)
	}
	return nil
}

func (m *mockClient) GetUpdateStatus(_ context.Context, updateID string) (*UpdateStatus, error) {
	if m.getUpdateStatusFunc != nil {
		return m.getUpdateStatusFunc(updateID)
	}
	return &UpdateStatus{UpdateID: updateID, Status: StatusProcessedValid}, nil
}

// testLogger is a no-op Logger test double, standing in for the CLI's
// output.NewTest(io.Discard) (this package uses the local Logger interface; see resolve.go).
type testLogger struct{}

func (l *testLogger) Infof(format string, args ...interface{})  {}
func (l *testLogger) Debugf(format string, args ...interface{}) {}

var testOut = &testLogger{}

var fastPollConfig = PollConfig{
	MaxAttempts: 3,
	Interval:    1 * time.Millisecond,
}
